package builder

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	craneProtos "scow-crane-adapter/gen/crane"
	"scow-crane-adapter/pkg/services/job/internal/types"
	"scow-crane-adapter/pkg/utils"
)

// ContainerJobBuilder 容器作业构建器
type ContainerJobBuilder struct{}

// NewContainerJobBuilder 创建构建器
func NewContainerJobBuilder() *ContainerJobBuilder {
	return &ContainerJobBuilder{}
}

// Build 构建容器作业
func (b *ContainerJobBuilder) Build(adapter types.ContainerJobRequest) (*craneProtos.TaskToCtld, error) {
	// 1. 创建基础任务
	task := b.createBaseTask()

	// 2. 应用资源配置
	if err := b.applyResourceOptions(adapter, task); err != nil {
		return nil, fmt.Errorf("application resource configuration failed: %v", err)
	}

	// 3. 应用调度配置
	if err := b.applySchedulingOptions(adapter, task); err != nil {
		return nil, fmt.Errorf("application scheduling configuration failed: %v", err)
	}

	if err := b.applyEnvironmentOptions(adapter, task); err != nil {
		return nil, fmt.Errorf("failed to apply environment options: %v", err)
	}

	// 4. 构建容器元数据
	containerMeta, err := b.buildContainerMeta(adapter)
	if err != nil {
		return nil, fmt.Errorf("build container metadata failed: %v", err)
	}

	// 5. 构建 Pod 元数据
	podMeta, err := b.buildPodMeta(adapter, task)
	if err != nil {
		return nil, fmt.Errorf("build pod metadata failed: %v", err)
	}

	// 6. 设置作业名称
	b.setJobName(adapter, task)

	// 7. 设置扩展属性
	b.setExtendedProperties(adapter, task)

	task.ContainerMeta = containerMeta
	task.PodMeta = podMeta

	return task, nil
}

// createBaseTask 创建基础任务
func (b *ContainerJobBuilder) createBaseTask() *craneProtos.TaskToCtld {
	return &craneProtos.TaskToCtld{
		Type:      craneProtos.TaskType_Container,
		TimeLimit: utils.InvalidDuration(),
		ReqResources: &craneProtos.ResourceView{
			AllocatableRes: &craneProtos.AllocatableResource{
				CpuCoreLimit:       1,
				MemoryLimitBytes:   0,
				MemorySwLimitBytes: 0,
			},
		},
		NodeNum:       1,
		NtasksPerNode: 1,
		CpusPerTask:   1,
		GetUserEnv:    false,
		Env:           make(map[string]string),
	}
}

// applyResourceOptions 应用资源配置
func (b *ContainerJobBuilder) applyResourceOptions(adapter types.ContainerJobRequest, task *craneProtos.TaskToCtld) error {
	// 设置 CPU
	coreCount := int32(adapter.GetCoreCount())
	if coreCount == 0 {
		coreCount = 1
	}
	task.CpusPerTask = float64(coreCount)

	// 设置内存
	if memoryMb := adapter.GetMemoryMb(); memoryMb != nil {
		memoryBytes := *memoryMb * 1024 * 1024
		task.ReqResources.AllocatableRes.MemoryLimitBytes = memoryBytes
		task.ReqResources.AllocatableRes.MemorySwLimitBytes = memoryBytes
	}

	// 设置 GPU
	if gpuCount := adapter.GetGpuCount(); gpuCount > 0 {
		deviceType, err := utils.GetPartitionDeviceType(adapter.GetPartition())
		if err != nil {
			return fmt.Errorf("failed to get partition device type: %v", err)
		}

		gres := deviceType + ":" + strconv.Itoa(int(gpuCount))
		// Set GPU resources via DeviceMap
		task.ReqResources.DeviceMap = utils.ParseGres(gres)
	}

	return nil
}

func (b *ContainerJobBuilder) applyEnvironmentOptions(adapter types.ContainerJobRequest, task *craneProtos.TaskToCtld) error {
	task.Cwd = adapter.GetWorkingDirectory()

	userUid, userGid, err := utils.GetUIDGIDByName(adapter.GetUserId())
	if err != nil {
		return err
	}

	if uid, err := strconv.ParseUint(userUid, 10, 32); err == nil {
		task.Uid = uint32(uid)
	} else {
		task.Uid = uint32(os.Getuid())
	}

	if gid, err := strconv.ParseUint(userGid, 10, 32); err == nil {
		task.Gid = uint32(gid)
	} else {
		task.Gid = uint32(os.Getgid())
	}

	return nil
}

// applySchedulingOptions 应用调度配置
func (b *ContainerJobBuilder) applySchedulingOptions(adapter types.ContainerJobRequest, task *craneProtos.TaskToCtld) error {
	// 设置分区
	if partition := adapter.GetPartition(); partition != "" {
		task.PartitionName = partition
	}

	// Time limit
	if timeLimit := adapter.GetTimeLimit(); timeLimit != nil {
		seconds := *timeLimit * 60
		task.TimeLimit.Seconds = int64(seconds)
	}

	// Account
	if account := adapter.GetAccount(); account != "" {
		task.Account = account
	}

	// QoS
	if qos := adapter.GetQos(); qos != nil && *qos != "" {
		task.Qos = *qos
	}

	// Node allocation - validate parameters
	if nodeCount := adapter.GetNodeCount(); nodeCount > 0 {
		task.NodeNum = nodeCount
	} else {
		task.NodeNum = 1
	}

	return nil
}

// buildContainerMeta 构建容器元数据
func (b *ContainerJobBuilder) buildContainerMeta(adapter types.ContainerJobRequest) (*craneProtos.ContainerTaskAdditionalMeta, error) {
	// 获取镜像
	image, err := adapter.GetImage()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain image: %v", err)
	}

	// 创建容器元数据
	containerMeta := &craneProtos.ContainerTaskAdditionalMeta{
		Image: &craneProtos.ContainerTaskAdditionalMeta_ImageInfo{
			Image:      image,
			PullPolicy: utils.PullIfNotPresent,
		},
		Env:    make(map[string]string),
		Mounts: make(map[string]string),
	}

	containerMeta.Command = b.setCommand(adapter)
	if adapter.GetJobType() == types.JobTypeDevHost {
		containerMeta.Args = b.setArgs(adapter)
	}

	// 添加环境变量
	for _, env := range adapter.GetEnvVariables() {
		containerMeta.Env[env.Name] = env.Value
	}

	// 添加系统环境变量
	containerMeta.Env["JOB_TYPE"] = adapter.GetJobType().String()
	containerMeta.Env["USER_ID"] = adapter.GetUserId()
	containerMeta.Env["JOB_NAME"] = adapter.GetJobName()

	// 处理挂载点
	if mounts, err := adapter.GetMounts(); err == nil {
		b.mergeMount(containerMeta.Mounts, mounts)
	}

	// 添加特殊路径
	b.addExtraMount(adapter, containerMeta.Mounts)
	b.addExtraEnv(adapter, containerMeta.Env)
	b.applyIOOptions(containerMeta)

	return containerMeta, nil
}

// setExtendedProperties 设置扩展属性
func (b *ContainerJobBuilder) setCommand(adapter types.ContainerJobRequest) string {
	command := "/opt/entry.sh"
	if adapter.GetJobType() == types.JobTypeDevHost {
		command = "/opt/crane/entry.sh"
	}
	return command
}

func (b *ContainerJobBuilder) setArgs(adapter types.ContainerJobRequest) []string {
	var args []string

	proxyBasePath := adapter.GetJupyterLabInfo().ProxyBasePath
	vscodeBinPath := adapter.GetVSCodeInfo().VscodeBinPath
	if proxyBasePath != "" && vscodeBinPath != "" {
		vscodePort := utils.VscodePort
		jupyterPort := utils.JupyterPort
		hostname, _ := os.Hostname()

		host := fmt.Sprintf("--host=%s", hostname)
		jupyterPortStr := fmt.Sprintf("--jupyter-port=%d", jupyterPort)
		jupyterSvcPort := fmt.Sprintf("--jupyter-svcport=%d", jupyterPort)
		jupyterProxy := fmt.Sprintf("--jupyter-proxy=%s", proxyBasePath)
		vscodePortStr := fmt.Sprintf("--vscode-port=%d", vscodePort)
		vscodeSvcPort := fmt.Sprintf("--vscode-svcport=%d", vscodePort)
		vscodeBin := fmt.Sprintf("--vscode-bin=%s", vscodeBinPath)

		args = append(args, "--mode=both", host, jupyterPortStr,
			jupyterSvcPort, jupyterProxy, vscodePortStr, vscodeSvcPort, vscodeBin)
	} else if vscodeBinPath != "" {
		strAppPort := adapter.GetContainerPort()
		hostname, _ := os.Hostname()

		host := fmt.Sprintf("--host=%s", hostname)
		vscodePortStr := fmt.Sprintf("--vscode-port=%d", strAppPort)
		vscodeSvcPort := fmt.Sprintf("--vscode-svcport=%d", strAppPort)
		vscodeBin := fmt.Sprintf("--vscode-bin=%s", vscodeBinPath)

		args = append(args, "--mode=vscode", host, vscodePortStr, vscodeSvcPort, vscodeBin)
	} else if proxyBasePath != "" {
		strAppPort := adapter.GetContainerPort()
		hostname, _ := os.Hostname()
		host := fmt.Sprintf("--host=%s", hostname)
		jupyterPortStr := fmt.Sprintf("--jupyter-port=%d", strAppPort)
		jupyterSvcPort := fmt.Sprintf("--jupyter-svcport=%d", strAppPort)
		jupyterProxy := fmt.Sprintf("--jupyter-proxy=%s", proxyBasePath)

		args = append(args, "--mode=jupyterlab", host, jupyterPortStr,
			jupyterSvcPort, jupyterProxy)
		//command += fmt.Sprintf(" --mode=jupyterlab --host=%s --jupyter-port=%d --jupyter-svcport=%d --jupyter-proxy=%s",
		//	hostname, strAppPort, strAppPort, proxyBasePath)
	}

	return args
}

// addSpecialPaths 添加特殊路径
func (b *ContainerJobBuilder) addExtraMount(adapter types.ContainerJobRequest, mount map[string]string) {
	mount[adapter.GetWorkingDirectory()] = adapter.GetWorkingDirectory()
	//mount[adapter.GetScript()] = "/opt/entry.sh"
	if adapter.GetJobType() == types.JobTypeDevHost {
		path := utils.SplitBeforeUser(adapter.GetWorkingDirectory(), adapter.GetUserId())
		mount[path+"crane/"] = "/opt/crane/"
	} else {
		mount[utils.GetDirPathWithSlash(adapter.GetScript())] = utils.GetDirPathWithSlash("/opt/entry.sh")
	}

	if algorithmPath, _ := adapter.GetAlgorithmPath(); algorithmPath != "" {
		algorithms, _ := utils.ParseMountModel(algorithmPath)
		for _, a := range algorithms {
			// readWriteMode := "ro"
			// if a.IsPublic {
			// 	readWriteMode = "rw"
			// }
			mount[a.Path] = a.Path // + ":" + readWriteMode
		}
	}
	if datasetPath, _ := adapter.GetDatasetPath(); datasetPath != "" {
		datasets, _ := utils.ParseMountModel(datasetPath)
		for _, d := range datasets {
			//readWriteMode := "ro"
			//if d.IsPublic {
			//	readWriteMode = "rw"
			//}
			mount[d.Path] = d.Path // + ":" + readWriteMode
		}
	}
	if modelPath, _ := adapter.GetModelPath(); modelPath != "" {
		models, _ := utils.ParseMountModel(modelPath)
		for _, m := range models {
			//readWriteMode := "ro"
			//if m.IsPublic {
			//	readWriteMode = "rw"
			//}
			mount[m.Path] = m.Path // + ":" + readWriteMode
		}
	}

	acceleratorType, _ := adapter.GetGpuType()
	if utils.AcceleratorIsAscend(acceleratorType) {
		b.mergeMount(mount, getAscendMount())
	}
}

// addSpecialPaths 添加特殊路径
func (b *ContainerJobBuilder) addExtraEnv(adapter types.ContainerJobRequest, env map[string]string) {
	env["WORK_DIR"] = adapter.GetWorkingDirectory()

	if algorithmPath, _ := adapter.GetAlgorithmPath(); algorithmPath != "" {
		algorithms, _ := utils.ParseMountModel(algorithmPath)
		paths := make([]string, len(algorithms))
		for i, mount := range algorithms {
			paths[i] = mount.Path
		}
		algorithmValue := strings.Join(paths, ":")
		key := utils.ContainerEnvPrefix + utils.AlgorithmPathEnv
		env[key] = algorithmValue
	}
	if datasetPath, _ := adapter.GetDatasetPath(); datasetPath != "" {
		dataset, _ := utils.ParseMountModel(datasetPath)
		paths := make([]string, len(dataset))
		for i, mount := range dataset {
			paths[i] = mount.Path
		}
		datasetValue := strings.Join(paths, ":")
		key := utils.ContainerEnvPrefix + utils.DataSetPathEnv
		env[key] = datasetValue
	}
	if modelPath, _ := adapter.GetModelPath(); modelPath != "" {
		model, _ := utils.ParseMountModel(modelPath)
		paths := make([]string, len(model))
		for i, mount := range model {
			paths[i] = mount.Path
		}
		modelValue := strings.Join(paths, ":")
		key := utils.ContainerEnvPrefix + utils.ModelPathEnv
		env[key] = modelValue
	}
}

// buildPodMeta 构建 Pod 元数据
func (b *ContainerJobBuilder) buildPodMeta(adapter types.ContainerJobRequest, task *craneProtos.TaskToCtld) (*craneProtos.PodTaskAdditionalMeta, error) {
	//var networkMode craneProtos.PodTaskAdditionalMeta_NamespaceMode
	//switch utils.NetworkMode {
	//case "host": // NODE
	//	networkMode = craneProtos.PodTaskAdditionalMeta_NODE
	//case "default": // POD
	//	networkMode = craneProtos.PodTaskAdditionalMeta_POD
	//default: // TARGET / CONTAINER is not support.
	//	return nil, fmt.Errorf("invalid network specification '%s': only 'host' and 'default' are supported", utils.NetworkMode)
	//}

	podMeta := &craneProtos.PodTaskAdditionalMeta{
		Name: adapter.GetJobName(),
		Namespace: &craneProtos.PodTaskAdditionalMeta_NamespaceOption{
			Network: craneProtos.PodTaskAdditionalMeta_POD,
		},
		Userns: utils.UserNs,
	}

	logrus.Infof("podMeta Userns %v", podMeta.Userns)
	//if podMeta.Userns {
	//	userId := adapter.GetUserId()
	//	uid, gid, err := utils.GetUIDGIDByName(userId)
	//	userSpec := uid + ":" + gid
	//	if err != nil {
	//		return nil, fmt.Errorf("invalid user id '%s': %v", userId, err)
	//	}
	//	if err := parseUserSpec(userSpec, podMeta); err != nil {
	//		return nil, fmt.Errorf("invalid user specification '%s': %v", userSpec, err)
	//	}
	//} else if !podMeta.Userns {
	//	podMeta.RunAsUser = task.Uid
	//	podMeta.RunAsGroup = task.Gid
	//}
	if !podMeta.Userns {
		podMeta.RunAsUser = task.Uid
		podMeta.RunAsGroup = task.Gid
	}

	ports := adapter.GetContainerPort()
	if len(ports) != 0 {
		if adapter.GetJobType() == types.JobTypeDevHost {
			parseDevHostPortMapping(ports, &podMeta.Ports)
		} else {
			parsePortMapping(ports, &podMeta.Ports)
		}
	}

	return podMeta, nil
}

// setJobName 设置作业名称
func (b *ContainerJobBuilder) setJobName(adapter types.ContainerJobRequest, task *craneProtos.TaskToCtld) {
	if jobName := adapter.GetJobName(); jobName != "" {
		task.Name = jobName
	} else {
		task.Name = b.generateDefaultJobName(adapter)
	}
}

// setExtendedProperties 设置扩展属性
func (b *ContainerJobBuilder) setExtendedProperties(adapter types.ContainerJobRequest, task *craneProtos.TaskToCtld) {
	err := utils.CheckAndAddExecPermission(adapter.GetWorkingDirectory())
	if err != nil {
		logrus.Errorf("file %v add exec permission error: %v", filepath.Join(adapter.GetWorkingDirectory(), "entry.sh"), err)
	}
}

// generateDefaultJobName 生成默认作业名
func (b *ContainerJobBuilder) generateDefaultJobName(adapter types.ContainerJobRequest) string {
	jobType := adapter.GetJobType()
	username := adapter.GetUserId()
	timestamp := time.Now().Unix()

	switch jobType {
	case types.JobTypeTraining:
		return fmt.Sprintf("train-%s-%d", username, timestamp)
	case types.JobTypeInference:
		return fmt.Sprintf("infer-%s-%d", username, timestamp)
	case types.JobTypeDevHost:
		return fmt.Sprintf("devhost-%s-%d", username, timestamp)
	case types.JobTypeApp:
		return fmt.Sprintf("app-%s-%d", username, timestamp)
	default:
		return fmt.Sprintf("job-%s-%d", username, timestamp)
	}
}

// generateDefaultJobName 生成默认作业名
func (b *ContainerJobBuilder) mergeMount(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func getAscendMount() map[string]string {
	return map[string]string{
		"/usr/local/Ascend/driver":  "/usr/local/Ascend/driver",
		"/usr/local/Ascend/add-ons": "/usr/local/Ascend/add-ons",
		"/usr/local/bin/npu-smi":    "/usr/local/bin/npu-smi",
		"/etc/localtime":            "/etc/localtime",
	}
}

// parseUserSpec parses user specification in format "uid" or "uid:gid"
func parseUserSpec(userSpec string, podMeta *craneProtos.PodTaskAdditionalMeta) error {
	parts := strings.SplitN(userSpec, ":", 2)

	// Parse user (UID only)
	user := parts[0]
	if uid, err := strconv.ParseUint(user, 10, 32); err == nil {
		podMeta.RunAsUser = uint32(uid)
	} else {
		// We currently do not intend to support user name resolution
		return fmt.Errorf("user name resolution not supported, please provide a numeric UID")
	}

	// Parse group if provided
	if len(parts) == 2 {
		group := parts[1]
		if gid, err := strconv.ParseUint(group, 10, 32); err == nil {
			podMeta.RunAsGroup = uint32(gid)
		} else {
			return fmt.Errorf("group name resolution not supported, please provide a numeric GID")
		}
	}

	return nil
}

func parseDevHostPortMapping(ports []uint32, portList *[]*craneProtos.PodTaskAdditionalMeta_PortMapping) {
	for _, port := range ports {
		mapping := &craneProtos.PodTaskAdditionalMeta_PortMapping{
			Protocol: craneProtos.PodTaskAdditionalMeta_PortMapping_TCP,
		}

		mapping.HostPort = int32(port)
		mapping.ContainerPort = int32(port)

		*portList = append(*portList, mapping)
	}
}

func parsePortMapping(ports []uint32, portList *[]*craneProtos.PodTaskAdditionalMeta_PortMapping) {
	for _, port := range ports {
		mapping := &craneProtos.PodTaskAdditionalMeta_PortMapping{
			Protocol: craneProtos.PodTaskAdditionalMeta_PortMapping_TCP,
		}

		hostPort, _ := generateUnusedRandomPort()
		mapping.HostPort = int32(hostPort)
		mapping.ContainerPort = int32(port)

		*portList = append(*portList, mapping)
	}
}

func (b *ContainerJobBuilder) applyIOOptions(containerMeta *craneProtos.ContainerTaskAdditionalMeta) {
	containerMeta.Detached = true
	// TTY allocation: directly use -t flag
	containerMeta.Tty = true

	// Stdin attachment: directly use -i flag
	containerMeta.Stdin = true
}

// getUsedPortsFromFile 读取jobs.json，提取所有已使用的端口（去重）
func getUsedPortsFromFile() (map[int]bool, error) {
	// 1. 检查文件是否存在
	if _, err := os.Stat(utils.JobsInfos); os.IsNotExist(err) {
		return map[int]bool{}, nil // 文件不存在，返回空的已使用端口列表
	} else if err != nil {
		return nil, fmt.Errorf("check file %s failed: %w", utils.JobsInfos, err)
	}

	// 2. 读取文件内容
	data, err := os.ReadFile(utils.JobsInfos)
	if err != nil {
		return nil, fmt.Errorf("read file %s failed: %w", utils.JobsInfos, err)
	}

	// 3. 解析JSON为作业列表
	var jobList []*utils.SubmitJobInfo
	if err := json.Unmarshal(data, &jobList); err != nil {
		return nil, fmt.Errorf("unmarshal json failed: %w", err)
	}

	// 4. 提取所有端口（HostPorts），用map去重
	usedPorts := make(map[int]bool)
	for _, job := range jobList {
		// 提取HostPorts
		for _, port := range job.HostPorts {
			usedPorts[int(port)] = true
		}
	}

	return usedPorts, nil
}

// generateUnusedRandomPort 生成未使用的随机端口
// 参数：
//
//	usedPorts - 已使用的端口map
//	minPort/maxPort - 端口范围
//	maxRetry - 最大重试次数
//
// 返回：未使用的端口 / 错误
func generateUnusedRandomPort() (int, error) {
	usedPorts, err := getUsedPortsFromFile()
	if err != nil {
		return 0, fmt.Errorf("get used ports failed: %w", err)
	}
	maxRetry := 10 // 重试10次

	// 2. 生成随机端口并校验是否未使用
	for retry := 0; retry < maxRetry; retry++ {
		// 生成加密安全的随机端口
		port := generateRandomPort(utils.MinPort, utils.MaxPort)

		// 校验端口是否未被使用
		if !usedPorts[port] {
			return port, nil
		}
		logrus.Tracef("retry %d: port %d is used, continue", retry, port)
	}

	// 重试耗尽仍未找到
	return 0, fmt.Errorf("no unused port found in range [%d, %d] after %d retries (used ports count: %d)",
		utils.MinPort, utils.MaxPort, maxRetry, len(usedPorts))
}

// min - 端口最小值（建议≥30000，避免系统端口）
// max - 端口最大值（≤65535）
func generateRandomPort(min, max int) int {
	rangeLen := big.NewInt(int64(max - min))
	randomNum, err := rand.Int(rand.Reader, rangeLen)
	if err != nil {
		return utils.MinPort
	}
	return min + int(randomNum.Int64())
}
