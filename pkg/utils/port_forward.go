package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	craneProtos "scow-crane-adapter/gen/crane"
)

var (
	globalPersistence  = NewFilePersistence(ProxyInfos)
	GlobalProxyManager = NewProxyManager(globalPersistence)
)

type SubmitJobProxyInfo struct {
	JobName      string            `json:"new_job_name"`
	JobId        uint32            `json:"job_id"`
	ForwardNodes []*JobForwardNode `json:"forward_info,omitempty"`
}

// JobForwardNode 作业端口转发的节点
type JobForwardNode struct {
	ExecutionNode string `json:"execution_node"`
}

// ProxyManager 代理管理器
type ProxyManager struct {
	mu          sync.Mutex
	proxyMap    map[string]*ProxyService
	persistence *FilePersistence // 持久化实例
	ticker      *time.Ticker     // 定时巡检的Ticker
	stopChan    chan struct{}    // 停止定时任务的信号通道
}

// NewProxyManager 创建代理管理器
func NewProxyManager(persistence *FilePersistence) *ProxyManager {
	return &ProxyManager{
		proxyMap:    make(map[string]*ProxyService),
		persistence: persistence,
		stopChan:    make(chan struct{}),
	}
}

type ProxyService struct {
	ctx        context.Context
	cancel     context.CancelFunc
	listener   net.Listener
	proxyPort  int
	targetAddr string
	server     *http.Server
	JobId      uint32
}

// ProxyMeta 代理元信息
type ProxyMeta struct {
	JobName    string `json:"job_name"`    // 作业名
	JobId      uint32 `json:"job_id"`      // 作业ID
	ProxyPort  int    `json:"proxy_port"`  // master上的代理端口
	TargetNode string `json:"target_node"` // 容器运行节点
	TargetAddr string `json:"target_addr"` // 容器目标地址（http://nodeIP:port）
}

// FilePersistence 文件持久化实现
type FilePersistence struct {
	filename string // 持久化文件路径
	mu       sync.Mutex
}

// NewFilePersistence 创建文件持久化实例
func NewFilePersistence(filename string) *FilePersistence {
	return &FilePersistence{
		filename: filename,
	}
}

// Delete 删除指定作业的代理元信息
func (f *FilePersistence) Delete(jobName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	metas, err := f.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load existing metadata: %w", err)
	}

	// 过滤掉要删除的作业
	newMetas := make([]*ProxyMeta, 0)
	for _, m := range metas {
		if m.JobName != jobName {
			newMetas = append(newMetas, m)
		}
	}

	// 重新写入文件
	data, err := json.MarshalIndent(newMetas, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}
	if err := ioutil.WriteFile(f.filename, data, 0644); err != nil {
		return fmt.Errorf("fail to write to file: %w", err)
	}

	logrus.Tracef("[job %s] Proxy metadata has been removed from persistent file", jobName)
	return nil
}

// LoadAll 加载所有代理元信息
func (f *FilePersistence) LoadAll() ([]*ProxyMeta, error) {
	data, err := ioutil.ReadFile(f.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ProxyMeta{}, nil // 文件不存在，返回空列表
		}
		return nil, fmt.Errorf("failed to read persistent file: %w", err)
	}

	var metas []*ProxyMeta
	if err := json.Unmarshal(data, &metas); err != nil {
		return nil, fmt.Errorf("failed to deserialize metadata: %w", err)
	}
	return metas, nil
}

// Save 保存代理元信息
func (f *FilePersistence) Save(meta *ProxyMeta) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 加载现有数据
	metas, err := f.LoadAll()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load existing metadata: %w", err)
	}

	// 去重（覆盖已有作业的元信息）
	newMetas := make([]*ProxyMeta, 0)
	found := false
	for _, m := range metas {
		if m.JobName != meta.JobName {
			newMetas = append(newMetas, m)
		} else {
			newMetas = append(newMetas, meta)
			found = true
		}
	}
	if !found {
		newMetas = append(newMetas, meta)
	}

	// 原子写入
	tempFile := f.filename + ".tmp"
	data, err := json.MarshalIndent(newMetas, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize metadata: %w", err)
	}
	tempDir := filepath.Dir(tempFile)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for temp file: %w", err)
	}
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}
	if err := os.Rename(tempFile, f.filename); err != nil {
		return fmt.Errorf("failed to replace persistent file: %w", err)
	}

	logrus.Tracef("[job %s] Proxy metadata has been persisted to a file: %s", meta.JobName, f.filename)
	return nil
}

func NewProxyService(jobId uint32, targetAddr string) (*ProxyService, error) {
	_, err := url.Parse(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target address: %w", err)
	}

	port, err := findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to search for available ports: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ProxyService{
		ctx:        ctx,
		cancel:     cancel,
		proxyPort:  port,
		targetAddr: targetAddr,
		JobId:      jobId,
	}, nil
}

func (p *ProxyService) Start() error {
	targetURL, _ := url.Parse(p.targetAddr)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logrus.Errorf("[job %s] Proxy request failed: %v, destination address: %s", p.JobId, err, p.targetAddr)
		http.Error(w, "Proxy service temporarily unavailable", http.StatusBadGateway)
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Real-IP", req.RemoteAddr)
		req.Header.Set("X-Forwarded-Host", req.Host)
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", p.proxyPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("[job %s] Listening on port% d failed: %w", p.JobId, p.proxyPort, err)
	}
	p.listener = listener

	p.server = &http.Server{
		Handler:      proxy,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		logrus.Tracef("[job %s] Proxy service started successfully: crane-master:%d -> %s", p.JobId, p.proxyPort, p.targetAddr)
		if err := p.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logrus.Errorf("[job %s] Proxy service abnormal exit: %v", p.JobId, err)
		}
	}()

	return nil
}

func (p *ProxyService) Stop() error {
	logrus.Tracef("[job %s] Stop proxy service and release port%d", p.JobId, p.proxyPort)

	p.cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := p.server.Shutdown(shutdownCtx); err != nil {
		logrus.Errorf("[job %s] Elegant shutdown of HTTP service failed, forced shutdown: %v", p.JobId, err)
		if err := p.listener.Close(); err != nil {
			return fmt.Errorf("[job %s] Forcefully closing port% d failed: %w", p.JobId, p.proxyPort, err)
		}
		return nil
	}

	logrus.Tracef("[job %s] Proxy service has stopped, port% d has been released", p.JobId, p.proxyPort)
	return nil
}

// findAvailablePort 查找可用端口（支持指定固定端口恢复）
func findAvailablePort() (int, error) {
	for port := MinPort; port <= MaxPort; port++ {
		listenAddr := fmt.Sprintf("0.0.0.0:%d", port)
		listener, err := net.Listen("tcp", listenAddr)
		if err == nil {
			_ = listener.Close()
			return port, nil
		}
		logrus.Warnf("Port% d is already occupied, try the next one", port)
	}

	return 0, fmt.Errorf("no available ports found (%d-%d)", MinPort, MaxPort)
}

// isTargetReachable 检查目标容器是否可达（恢复前健康检查）
func isTargetReachable(targetAddr string) bool {
	parsedURL, err := url.Parse(targetAddr)
	if err != nil {
		logrus.Errorf("Failed to parse target address %s: %v", targetAddr, err)
		return false
	}

	// 构造TCP地址（host:port）
	addr := parsedURL.Host
	if parsedURL.Port() == "" {
		if parsedURL.Scheme == "http" {
			addr = parsedURL.Host + ":80"
		} else if parsedURL.Scheme == "https" {
			addr = parsedURL.Host + ":443"
		}
	}

	// 超时连接测试
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		logrus.Errorf("The target address %s is unreachable: %v", targetAddr, err)
		return false
	}
	defer conn.Close()

	return true
}

// RecoverProxies 程序启动时恢复所有代理服务（不依赖IsRunning，仅以作业/容器实际状态为准）
func (m *ProxyManager) RecoverProxies() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logrus.Tracef("Start restoring the proxy service before restarting...")

	// 1. 加载所有持久化的代理元信息
	metas, err := m.persistence.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load persistent metadata: %w", err)
	}
	if len(metas) == 0 {
		logrus.Tracef("No persistent proxy metadata, no need for recovery")
		return nil
	}

	// 2. 逐个校验并恢复代理
	successCount := 0
	failCount := 0
	cleanCount := 0
	for _, meta := range metas {
		// 重新创建代理实例
		proxy, err := NewProxyService(meta.JobId, meta.TargetAddr)
		if err != nil {
			logrus.Errorf("[job %s] Failed to create proxy instance: %v", meta.JobName, err)
			failCount++
			continue
		}

		// 强制复用原有端口，保证Web服务调用地址不变
		proxy.proxyPort = meta.ProxyPort

		// 4. 启动代理服务
		if err := proxy.Start(); err != nil {
			logrus.Errorf("[job %s] Failed to start proxy service: %v", meta.JobName, err)
			failCount++
			continue
		}

		// 5. 将恢复的代理加入管理器
		node, port, _ := ParseTargetAddr(proxy.targetAddr)

		name := strconv.Itoa(int(proxy.JobId)) + "-" + node + "-" + strconv.Itoa(port)
		m.proxyMap[name] = proxy
		successCount++
		logrus.Tracef("[job %s] Proxy service restored successfully, port: %d", meta.JobName, meta.ProxyPort)
	}

	logrus.Tracef("Proxy service recovery completed: %d successful: %d failed: %d invalid cleanup, total %d",
		successCount, failCount, cleanCount, len(metas))
	return nil
}

// CreateAndStartProxy 创建并启动代理
func (m *ProxyManager) CreateAndStartProxy(proxyInfo *SubmitJobProxyInfo, hostPorts []int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	jobName, jobId := proxyInfo.JobName, proxyInfo.JobId

	// containerPorts := jobInfo.ContainerPorts
	logrus.Infof("Creating proxy service for job %s-%d", jobName, jobId)
	for _, forwardInfo := range proxyInfo.ForwardNodes {
		for _, port := range hostPorts {
			name := strconv.Itoa(int(jobId)) + "-" + forwardInfo.ExecutionNode + "-" + strconv.Itoa(int(port))
			if _, exists := m.proxyMap[name]; exists {
				logrus.Infof("job %s agent already exists", name)
				continue
			}
			// 构造容器目标地址
			targetAddr := fmt.Sprintf("http://%s:%d", forwardInfo.ExecutionNode, port)

			// 创建代理实例
			proxy, err := NewProxyService(jobId, targetAddr)
			if err != nil {
				return fmt.Errorf("failed to create proxy: %v", err)
			}

			// 启动代理服务
			if err := proxy.Start(); err != nil {
				return fmt.Errorf("failed to start agent: %v", err)
			}

			logrus.Infof("Create proxy service for job %s success", name)
			// 将代理加入管理器
			m.proxyMap[name] = proxy

			// 持久化代理元信息
			meta := &ProxyMeta{
				JobName:    jobName,
				JobId:      jobId,
				ProxyPort:  proxy.proxyPort,
				TargetNode: forwardInfo.ExecutionNode,
				TargetAddr: targetAddr,
			}
			if err := m.persistence.Save(meta); err != nil {
				logrus.Errorf("[job %d] Proxy metadata persistence failed: %v", jobId, err)
			}
		}

	}

	return nil
}

// StopAndRemoveProxy 停止并移除代理
func (m *ProxyManager) StopAndRemoveProxy(jobName string, nodes []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, node := range nodes {
		// 检查作业是否有代理
		name := jobName + "-" + node
		proxy, exists := m.proxyMap[name]
		if !exists {
			return fmt.Errorf("job %s agent does not exist", jobName)
		}

		// 停止代理服务
		if err := proxy.Stop(); err != nil {
			return fmt.Errorf("stop proxy %v failed: %v", proxy, err)
		}
	}

	// 从持久化存储删除
	if err := m.persistence.Delete(jobName); err != nil {
		logrus.Errorf("[job %s] failed to delete persistent metadata: %v", jobName, err)
	}

	// 从管理器移除
	delete(m.proxyMap, jobName)

	return nil
}

// StartPeriodicClean 启动定时清理任务（每30分钟执行一次）
// interval: 清理间隔（如30*time.Minute），便于测试可传更小值（如10*time.Second）
func (m *ProxyManager) StartPeriodicClean(interval time.Duration) {
	// 初始化Ticker
	m.ticker = time.NewTicker(interval)
	logrus.Tracef("The scheduled cleaning task has started, interval: %v", interval)

	// 异步执行定时任务
	go func() {
		for {
			select {
			case <-m.ticker.C:
				// 到达间隔，执行清理
				if err := m.CleanInvalidProxies(); err != nil {
					logrus.Errorf("Timed cleaning of invalid agents failed: %v", err)
				}
			case <-m.stopChan:
				// 收到停止信号，关闭Ticker并退出
				m.ticker.Stop()
				logrus.Tracef("The scheduled cleaning task has stopped")
				return
			}
		}
	}()
}

// StopPeriodicClean 停止定时清理任务
func (m *ProxyManager) StopPeriodicClean() {
	close(m.stopChan)
}

// CleanInvalidProxies 核心逻辑：清理无效代理
func (m *ProxyManager) CleanInvalidProxies() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logrus.Infof("Start executing scheduled cleaning of invalid proxy tasks...")

	// 加载所有持久化的代理元信息
	metas, err := m.persistence.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load persistent metadata: %w", err)
	}
	if len(metas) == 0 {
		logrus.Infof("No persistent proxy metadata, no need to clean up")
		return nil
	}

	// 逐个检查作业状态，清理无效代理
	cleanCount := 0
	skipCount := 0
	for _, meta := range metas {
		jobId := meta.JobId
		jobName := meta.JobName

		// 查询作业真实状态
		jobInfo, err := GetJobById(jobId, "")
		if err != nil {
			logrus.Errorf("[job %s] Status query failed, skipping cleanup: %v", jobName, err)
			skipCount++
			continue
		}

		// 判断作业是否为有效状态（Running/Pending）
		jobStatus := jobInfo.Status
		if jobStatus == craneProtos.TaskStatus_Pending || jobStatus == craneProtos.TaskStatus_Running {
			logrus.Tracef("[job %s] Status is %v, keep proxy", jobName, jobStatus)
			skipCount++
			continue
		}

		// 无效状态：停止代理
		if proxy, exists := m.proxyMap[jobName]; exists {
			if err := proxy.Stop(); err != nil {
				logrus.Errorf("[job %s] Stop proxy failed: %v", jobName, err)
			} else {
				logrus.Tracef("[job %s] Invalid proxy stopped", jobName)
			}
			// 从内存中移除
			delete(m.proxyMap, jobName)
		}

		// 从持久化文件删除
		if err := m.persistence.Delete(jobName); err != nil {
			logrus.Errorf("[job %s] Failed to delete persistent metadata: %v", jobName, err)
		} else {
			logrus.Tracef("[job %s] Persistent metadata has been deleted (job status:%v)", jobName, jobStatus)
		}

		cleanCount++
	}

	// 输出清理统计
	logrus.Tracef("cleaning completed: %d invalid agents were cleared, %d valid agents were retained, and a total of %d agents were checked",
		cleanCount, skipCount, len(metas))
	return nil
}

// ParseTargetAddr 从 targetAddr（http://xxx:port）中解析出节点地址和端口
// 参数：
//
//	targetAddr - 格式如 "http://192.168.1.100:8080" 或 "http://node01:9090"
//
// 返回：
//
//	executionNode - 节点IP/域名（如 "192.168.1.100"、"node01"）
//	port - 端口号（如 8080）
//	err - 解析错误（格式非法、端口非数字等）
func ParseTargetAddr(targetAddr string) (executionNode string, port int, err error) {
	// 1. 空值校验
	if strings.TrimSpace(targetAddr) == "" {
		return "", 0, fmt.Errorf("targetAddr is empty")
	}

	// 2. 解析URL（处理http/https协议）
	parsedURL, err := url.Parse(targetAddr)
	if err != nil {
		return "", 0, fmt.Errorf("parse url failed: %w (targetAddr: %s)", err, targetAddr)
	}

	// 3. 提取主机+端口（Host字段格式："host:port"）
	hostPort := parsedURL.Host
	if hostPort == "" {
		return "", 0, fmt.Errorf("invalid targetAddr: no host/port found (targetAddr: %s)", targetAddr)
	}

	// 4. 拆分主机和端口
	// 处理特殊情况：主机包含冒号（如IPv6地址 [::1]:8080）
	host, portStr, err := splitHostPort(hostPort)
	if err != nil {
		return "", 0, fmt.Errorf("split host:port failed: %w (hostPort: %s)", err, hostPort)
	}

	// 5. 端口转换为数字
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("port is not a number: %s (hostPort: %s)", portStr, hostPort)
	}

	// 6. 端口合法性校验
	if port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port: %d (must 1<=port<=65535)", port)
	}

	return host, port, nil
}

// splitHostPort 安全拆分host:port（兼容IPv6）
// 替代标准库 net.SplitHostPort，避免IPv6地址解析失败
func splitHostPort(hostPort string) (host, port string, err error) {
	// 先尝试标准库拆分（处理IPv4/域名）
	host, port, err = net.SplitHostPort(hostPort)
	if err == nil {
		return host, port, nil
	}

	// 兼容IPv6（格式如 [::1]:8080）
	if strings.HasPrefix(hostPort, "[") && strings.Contains(hostPort, "]:") {
		parts := strings.SplitN(hostPort, "]:", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid IPv6 format: %s", hostPort)
		}
		host = strings.TrimPrefix(parts[0], "[")
		port = parts[1]
		return host, port, nil
	}

	// 无端口的情况（非法）
	return "", "", fmt.Errorf("no port found in %s", hostPort)
}

// LoadJobProxyMetaFromFile 从 /adapter/jobs/proxy.json 读取 JSON 并还原为 ProxyMeta
func LoadJobProxyMetaFromFile(jobId uint32, nodeName string) (*ProxyMeta, error) {
	if jobId == 0 {
		return nil, fmt.Errorf("job id is empty")
	}

	path := filepath.Join(ProxyInfos)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}

	var metas []*ProxyMeta
	if err := json.Unmarshal(data, &metas); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	for _, meta := range metas {
		if meta.JobId == jobId && meta.TargetNode == nodeName {
			return meta, nil
		}
	}

	return nil, nil
}

func BuildJobForwardInfo(podMeta *craneProtos.PodTaskAdditionalMeta, stepList []*craneProtos.StepInfo) ([]*JobForwardNode, error) {
	var jfi []*JobForwardNode
	// 校验PodMeta的端口有效性
	if podMeta.Ports[0].ContainerPort <= 0 {
		return jfi, fmt.Errorf("container port in podMeta is invalid (value must be greater than 0)")
	}
	if podMeta.Ports[0].HostPort <= 0 {
		return jfi, fmt.Errorf("host port in podMeta is invalid (value must be greater than 0)")
	}

	var jobId uint32
	for _, step := range stepList {
		if step.StepType != craneProtos.StepType_PRIMARY {
			continue // 跳过DAEMON类型
		}

		// 所有PRIMARY step的job_id应一致，这里取第一个有效值即可
		if jobId == 0 {
			jobId = step.JobId
		} else if step.JobId != jobId {
			return jfi, fmt.Errorf("the job_id of step is inconsistent (existing: %d, new value: %d)", jobId, step.JobId)
		}

		// 校验并收集execution_node
		if len(step.ExecutionNode) == 0 {
			return jfi, fmt.Errorf("the execution_node of PRIMARY step is empty")
		}
		for _, node := range step.ExecutionNode {
			jfi = append(jfi, &JobForwardNode{
				ExecutionNode: node,
			})
		}
	}

	// 返回JobForwardInfo
	return jfi, nil
}
