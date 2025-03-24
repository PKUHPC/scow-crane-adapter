package utils

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"reflect"
	"sort"
	"strconv"
	"strings"

	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"
)

type CraneConfig struct {
	ClusterName         string `yaml:"ClusterName"`
	ControlMachine      string `yaml:"ControlMachine"`
	CraneCtldListenPort string `yaml:"CraneCtldListenPort"`

	UseTls             bool        `yaml:"UseTls"`
	ServerCertFilePath string      `yaml:"ServerCertFilePath"`
	ServerKeyFilePath  string      `yaml:"ServerKeyFilePath"`
	CaCertFilePath     string      `yaml:"CaCertFilePath"`
	DomainSuffix       string      `yaml:"DomainSuffix"`
	Partitions         []Partition `yaml:"Partitions"`
}

type Partition struct {
	Name  string `yaml:"name"`
	Nodes string `yaml:"nodes"`
}

var (
	DefaultConfigPath = "/etc/crane/config.yaml"
)

// ParseConfig 解析crane配置文件
func ParseConfig(configFilePath string) *CraneConfig {
	confFile, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	config := &CraneConfig{}
	err = yaml.Unmarshal(confFile, config)
	if err != nil {
		log.Fatal(err)
	}
	return config
}

// GetUidByUserName 通过os/user包去获取用户的uid
func GetUidByUserName(userName string) (int, error) {
	u, err := user.Lookup(userName)
	if err != nil {
		fmt.Printf("Failed to lookup user: %s\n", err)
		return 0, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	return uid, nil
}

// RichError rich error model 封装
func RichError(code codes.Code, reason string, message string) error {
	errInfo := &errdetails.ErrorInfo{
		Reason: reason,
	}
	st := status.New(code, message)
	st, _ = st.WithDetails(errInfo)
	return st.Err()
}

// GetQos 获取系统中Qos列表
func GetQos() ([]string, error) {
	var Qoslist []string
	request := &craneProtos.QueryQosInfoRequest{
		Uid: uint32(os.Getuid()),
	}
	response, err := CraneCtld.QueryQosInfo(context.Background(), request)
	if err != nil {
		return []string{}, err
	}
	Qos := response.GetQosList()
	for _, value := range Qos {
		Qoslist = append(Qoslist, value.GetName())
	}
	return Qoslist, nil
}

func GetAllQos() ([]string, error) {
	qosList, err := GetQos()
	if err != nil {
		return []string{}, err
	}
	qosListValue := RemoveValue(qosList, "UNLIMITED")
	if len(qosListValue) == 0 {
		return nil, RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	return qosListValue, nil
}

func GetAllAccount() ([]*craneProtos.AccountInfo, error) {
	request := &craneProtos.QueryAccountInfoRequest{
		Uid: 0,
	}
	response, err := CraneCtld.QueryAccountInfo(context.Background(), request)
	if err != nil {
		return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetRichErrorList()[0].GetDescription())
	}
	return response.GetAccountList(), nil
}

// CheckAccount 检查账户名是否非法
func CheckAccount(name string) error {
	if len(name) > 30 {
		return fmt.Errorf("name is too long (up to 30)")
	}
	return nil
}

func GetAccountByName(accountName string) (*craneProtos.AccountInfo, error) {
	request := &craneProtos.QueryAccountInfoRequest{
		Uid:         0,
		AccountList: []string{accountName},
	}
	response, err := CraneCtld.QueryAccountInfo(context.Background(), request)
	if err != nil {
		return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", response.RichErrorList[0].GetDescription())
	}
	return response.GetAccountList()[0], nil
}

func GetAccountByUser(userName string) ([]string, error) {
	var accountList []string
	request := &craneProtos.QueryUserInfoRequest{
		Uid:      0,
		UserList: []string{userName},
	}
	response, err := CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetRichErrorList()[0].GetDescription())
	}

	for _, list := range response.GetUserList() {
		if strings.Contains(list.Account, "*") {
			account := list.Account[:len(list.Account)-1]
			accountList = append(accountList, account)
		} else {
			accountList = append(accountList, list.Account)
		}
	}
	return accountList, nil
}

func GetAllUser() ([]*craneProtos.UserInfo, error) {
	request := &craneProtos.QueryUserInfoRequest{
		Uid: 0,
	}
	response, err := CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetRichErrorList()[0].GetDescription())
	}
	return response.GetUserList(), nil
}

// GetAllAccountUserInfoMap 获取账户下每个用户的信息
func GetAllAccountUserInfoMap(allAccounts []*craneProtos.AccountInfo) (map[*craneProtos.AccountInfo][]*craneProtos.UserInfo, error) {
	accountUserInfo := make(map[*craneProtos.AccountInfo][]*craneProtos.UserInfo)

	for _, account := range allAccounts {
		request := &craneProtos.QueryUserInfoRequest{
			Uid:      0,
			UserList: account.Users,
		}
		response, err := CraneCtld.QueryUserInfo(context.Background(), request)
		if err != nil {
			return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
		}
		if !response.GetOk() {
			var message string
			for _, richError := range response.GetRichErrorList() {
				message += richError.GetDescription() + "\n"
			}
			return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", message)
		}

		for _, info := range response.GetUserList() {
			if _, ok := accountUserInfo[account]; !ok {
				accountUserInfo[account] = []*craneProtos.UserInfo{}
			}
			accountUserInfo[account] = append(accountUserInfo[account], info)
		}
	}

	return accountUserInfo, nil
}

func GetPartitionByName(partitionName string) (*craneProtos.PartitionInfo, error) {
	request := &craneProtos.QueryPartitionInfoRequest{
		PartitionName: partitionName,
	}
	response, err := CraneCtld.QueryPartitionInfo(context.Background(), request)
	if err != nil {
		return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
	}

	return response.GetPartitionInfo()[0], nil
}

func GetTaskByPartitionAndStatus(partitionList []string, statusList []craneProtos.TaskStatus) ([]*craneProtos.TaskInfo, error) {
	req := craneProtos.QueryTasksInfoRequest{
		FilterPartitions:            partitionList,
		FilterTaskStates:            statusList,
		OptionIncludeCompletedTasks: false,
	}

	response, err := CraneCtld.QueryTasksInfo(context.Background(), &req)
	if err != nil {
		return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}

	return response.GetTaskInfoList(), nil
}

func GetTaskByAccountName(accountNames []string) ([]*craneProtos.TaskInfo, error) {
	req := craneProtos.QueryTasksInfoRequest{
		OptionIncludeCompletedTasks: true,
		FilterAccounts:              accountNames,
		NumLimit:                    99999999,
	}

	response, err := CraneCtld.QueryTasksInfo(context.Background(), &req)
	if err != nil {
		return nil, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	if !response.GetOk() {
		return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}

	return response.GetTaskInfoList(), nil
}

func GetNodeByPartitionAndStatus(partitionList []string, cranedStateList []craneProtos.CranedResourceState) (uint32, error) {
	var nodeCount uint32
	controlStateList := []craneProtos.CranedControlState{craneProtos.CranedControlState_CRANE_NONE, craneProtos.CranedControlState_CRANE_DRAIN}
	req := craneProtos.QueryClusterInfoRequest{
		FilterPartitions:           partitionList,
		FilterCranedResourceStates: cranedStateList,
		FilterCranedControlStates:  controlStateList,
	}

	response, err := CraneCtld.QueryClusterInfo(context.Background(), &req)
	if err != nil {
		return 0, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	for _, partitionCraned := range response.Partitions {
		for _, commonCranedStateList := range partitionCraned.CranedLists {
			nodeCount += commonCranedStateList.Count
		}
	}
	return nodeCount, nil
}

func GetNodeByPartition(partitionList []string) (uint32, uint32, uint32, uint32, error) {
	var idleNodeCount, allocNodeCount, mixNodeCount, downNodeCount uint32

	cranedStateList := []craneProtos.CranedResourceState{craneProtos.CranedResourceState_CRANE_IDLE, craneProtos.CranedResourceState_CRANE_ALLOC, craneProtos.CranedResourceState_CRANE_MIX, craneProtos.CranedResourceState_CRANE_DOWN}
	controlStateList := []craneProtos.CranedControlState{craneProtos.CranedControlState_CRANE_NONE, craneProtos.CranedControlState_CRANE_DRAIN}
	req := craneProtos.QueryClusterInfoRequest{
		FilterPartitions:           partitionList,
		FilterCranedResourceStates: cranedStateList,
		FilterCranedControlStates:  controlStateList,
	}

	response, err := CraneCtld.QueryClusterInfo(context.Background(), &req)
	if err != nil {
		return idleNodeCount, allocNodeCount, mixNodeCount, downNodeCount, RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	for _, partitionCraned := range response.Partitions {
		for _, commonCranedStateList := range partitionCraned.CranedLists {
			if commonCranedStateList.Count > 0 {
				switch commonCranedStateList.ResourceState {
				case craneProtos.CranedResourceState_CRANE_IDLE:
					idleNodeCount += commonCranedStateList.Count
				case craneProtos.CranedResourceState_CRANE_ALLOC:
					allocNodeCount += commonCranedStateList.Count
				case craneProtos.CranedResourceState_CRANE_MIX:
					mixNodeCount += commonCranedStateList.Count
				case craneProtos.CranedResourceState_CRANE_DOWN:
					downNodeCount += commonCranedStateList.Count
				}
			}
		}
	}

	return idleNodeCount, allocNodeCount, mixNodeCount, downNodeCount, nil
}

func GetCraneStatesList(stateList []string) []craneProtos.TaskStatus {
	var statesList []craneProtos.TaskStatus
	for _, value := range stateList {
		if value == "PENDING" || value == "PENDDING" {
			statesList = append(statesList, craneProtos.TaskStatus_Pending)
		} else if value == "RUNNING" {
			statesList = append(statesList, craneProtos.TaskStatus_Running)
		} else if value == "CANCELED" {
			statesList = append(statesList, craneProtos.TaskStatus_Cancelled)
		} else if value == "COMPLETED" {
			statesList = append(statesList, craneProtos.TaskStatus_Completed)
		} else if value == "FAILED" || value == "NODE_FAIL" {
			statesList = append(statesList, craneProtos.TaskStatus_Failed)
		} else if value == "TIMEOUT" {
			statesList = append(statesList, craneProtos.TaskStatus_ExceedTimeLimit)
		} else {
			statesList = append(statesList, craneProtos.TaskStatus_Invalid)
		}
	}
	return statesList
}

func GetUserHomedir(username string) (string, error) {
	// 获取指定用户名的用户信息
	u, err := user.Lookup(username)
	if err != nil {
		return "", err
	}

	// 获取家目录
	homeDir := u.HomeDir
	return homeDir, nil
}

func RemoveValue[T comparable](list []T, value T) []T {
	var result []T
	for _, item := range list {
		if item != value {
			result = append(result, item)
		}
	}
	return result
}

func sortByKey(list []*protos.JobInfo, fieldName string, sortOrder string) bool {
	if sortOrder == "ASC" {
		sort.Slice(list, func(i, j int) bool {
			fieldValueI := reflect.ValueOf(list[i]).Elem().FieldByName(fieldName)
			fieldValueJ := reflect.ValueOf(list[j]).Elem().FieldByName(fieldName)
			switch fieldValueI.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return fieldValueI.Int() < fieldValueJ.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return fieldValueI.Uint() > fieldValueJ.Uint()
			case reflect.Float32, reflect.Float64:
				return fieldValueI.Float() < fieldValueJ.Float()
			case reflect.String:
				return fieldValueI.String() < fieldValueJ.String()
			default:
				return false
			}
		})
	} else if sortOrder == "DESC" {
		sort.Slice(list, func(i, j int) bool {
			fieldValueI := reflect.ValueOf(list[i]).Elem().FieldByName(fieldName)
			fieldValueJ := reflect.ValueOf(list[j]).Elem().FieldByName(fieldName)
			switch fieldValueI.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return fieldValueI.Int() > fieldValueJ.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return fieldValueI.Uint() > fieldValueJ.Uint()
			case reflect.Float32, reflect.Float64:
				return fieldValueI.Float() > fieldValueJ.Float()
			case reflect.String:
				return fieldValueI.String() > fieldValueJ.String()
			default:
				return false
			}
		})
	}
	return true
}

func SortJobInfo(sortKey string, sortOrder string, jobInfo []*protos.JobInfo) []*protos.JobInfo {
	sortByKey(jobInfo, sortKey, sortOrder)
	return jobInfo
}

// LocalSubmitJob 本地提交cbatch作业函数
func LocalSubmitJob(scriptString string, username string) (string, error) {
	// 提交作业命令行
	cmdLine := fmt.Sprintf("su - %s -c 'cbatch %s'", username, scriptString)
	cmd := exec.Command("bash", "-c", cmdLine)

	// 创建一个 bytes.Buffer 用于捕获输出
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	// 执行命令
	err := cmd.Run()
	if err != nil {
		return output.String(), err
	}

	return output.String(), nil
}

// RunCommand 简单执行shell命令函数
func RunCommand(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)

	// 创建一个 bytes.Buffer 用于捕获输出
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	// 执行命令
	err := cmd.Run()

	if err != nil {
		return output.String(), err
	}

	return strings.TrimSpace(output.String()), nil
}

// GetSlurmClusterConfig 使用slurm命令获取partition的信息
func GetSlurmClusterConfig(block bool, qosList []string) ([]*protos.Partition, error) {
	var partitions []*protos.Partition

	if block {
		return partitions, nil
	}

	for _, part := range CConfig.Partitions {
		partitionName := part.Name
		request := &craneProtos.QueryPartitionInfoRequest{
			PartitionName: partitionName,
		}
		response, err := CraneCtld.QueryPartitionInfo(context.Background(), request)
		if err != nil {
			return nil, RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
		}
		partitionValue := response.GetPartitionInfo()[0]
		totalGpusTypeMap := partitionValue.GetResTotal().GetDeviceMap()
		// device_map:{name_type_map:{key:"npu"  value:{type_count_map:{key:"910B3"  value:8}}}}
		gpuCount := GetGpuNumsFromPartition(totalGpusTypeMap)
		partitions = append(partitions, &protos.Partition{
			Name:  partitionValue.GetName(),
			MemMb: partitionValue.GetResTotal().GetAllocatableRes().GetMemoryLimitBytes() / (1024 * 1024),
			Cores: uint32(partitionValue.GetResTotal().GetAllocatableRes().GetCpuCoreLimit()),
			Gpus:  gpuCount,
			Nodes: partitionValue.GetAliveNodes(),
			Qos:   qosList,
		})
	}

	return partitions, nil
}

func GetPartitionDeviceType(partitionName string) (string, error) {
	var deviceType = ""
	request := &craneProtos.QueryPartitionInfoRequest{
		PartitionName: partitionName,
	}
	response, err := CraneCtld.QueryPartitionInfo(context.Background(), request)
	if err != nil {
		return "", RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
	}
	partitionValue := response.GetPartitionInfo()[0]
	deviceMap := partitionValue.GetResTotal().GetDeviceMap()

	for key, _ := range deviceMap.GetNameTypeMap() {
		deviceType = key
	}

	return deviceType, nil
}

// Contains reports whether v is present in s.
func Contains[S ~[]E, E comparable](s S, v E) bool {
	return Index(s, v) >= 0
}

// Index returns the index of the first occurrence of v in s,
// or -1 if not present.
func Index[S ~[]E, E comparable](s S, v E) int {
	for i := range s {
		if v == s[i] {
			return i
		}
	}
	return -1
}

func ExtractNodeInfo(info *craneProtos.CranedInfo) *protos.NodeInfo {
	var nodeState protos.NodeInfo_NodeState

	nodeName := info.GetHostname()
	partitions := info.GetPartitionNames()
	state := info.GetResourceState()
	switch state {
	case craneProtos.CranedResourceState_CRANE_IDLE:
		nodeState = protos.NodeInfo_IDLE
	case craneProtos.CranedResourceState_CRANE_MIX, craneProtos.CranedResourceState_CRANE_ALLOC:
		nodeState = protos.NodeInfo_RUNNING
	case craneProtos.CranedResourceState_CRANE_DOWN:
		nodeState = protos.NodeInfo_NOT_AVAILABLE
	default: // 其他不知道的状态默认为不可用的状态
		nodeState = protos.NodeInfo_NOT_AVAILABLE
	}
	totalMem := info.GetResTotal().GetAllocatableResInNode().GetMemoryLimitBytes()
	allocMem := info.GetResAlloc().GetAllocatableResInNode().GetMemoryLimitBytes()
	totalCpuCores := info.GetResTotal().GetAllocatableResInNode().GetCpuCoreLimit()
	allocCpuCores := info.GetResAlloc().GetAllocatableResInNode().GetCpuCoreLimit()
	totalGpusTypeMap := info.GetResTotal().GetDedicatedResInNode()
	totalGpus := getGpuNums(totalGpusTypeMap)
	allocGpusTypeMap := info.GetResAlloc().GetDedicatedResInNode()
	allocGpus := getGpuNums(allocGpusTypeMap)
	IdleGpuCountTypeMap := info.GetResAvail().GetDedicatedResInNode()
	idleGpus := getGpuNums(IdleGpuCountTypeMap)

	return &protos.NodeInfo{
		NodeName:          nodeName,
		Partitions:        partitions,
		State:             nodeState,
		CpuCoreCount:      uint32(totalCpuCores),
		AllocCpuCoreCount: uint32(allocCpuCores),
		IdleCpuCoreCount:  uint32(totalCpuCores) - uint32(allocCpuCores),
		TotalMemMb:        uint32(totalMem),
		AllocMemMb:        uint32(allocMem),
		IdleMemMb:         uint32(totalMem) - uint32(allocMem),
		GpuCount:          totalGpus,
		AllocGpuCount:     allocGpus,
		IdleGpuCount:      idleGpus,
	}
}

func getGpuNums(data *craneProtos.DedicatedResourceInNode) uint32 {
	if data == nil {
		return 0
	}

	var typeCount int
	for _, typeCountMap := range data.GetNameTypeMap() {
		for _, slots := range typeCountMap.GetTypeSlotsMap() {
			slotsSize := len(slots.Slots)
			if slotsSize != 0 {
				typeCount += slotsSize
			}
		}
	}

	return uint32(typeCount)
}

// GetGpuNumsFromPartition 获取加速卡的数量 device_map:{name_type_map:{key:"npu"  value:{type_count_map:{key:"910B3"  value:8}}}}
func GetGpuNumsFromPartition(data *craneProtos.DeviceMap) uint32 {
	if data == nil {
		return 0
	}

	var gpuCount int
	for _, typeCountMap := range data.GetNameTypeMap() { //name_type_map:{key:"npu"  value:{type_count_map:{key:"910B3"  value:8}}}
		for _, count := range typeCountMap.GetTypeCountMap() {
			gpuCount += int(count)
		}

	}

	return uint32(gpuCount)
}

func GetAllPartitions() []string {
	if CConfig.Partitions == nil {
		return nil
	}

	var partitions []string
	for _, partition := range CConfig.Partitions {
		partitions = append(partitions, partition.Name)
	}
	return partitions
}
