package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/utils"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	config        *utils.Config
	stubCraneCtld protos.CraneCtldClient
	logger        *logrus.Logger
)

type serverJob struct {
	protos.UnimplementedJobServiceServer
}

type serverAccount struct {
	protos.UnimplementedAccountServiceServer
}

type serverUser struct {
	protos.UnimplementedUserServiceServer
}

type serverConfig struct {
	protos.UnimplementedConfigServiceServer
}

type serverVersion struct {
	protos.UnimplementedVersionServiceServer
}

func init() {
	config = utils.ParseConfig(utils.DefaultConfigPath)
}

// version
func (s *serverVersion) GetVersion(ctx context.Context, in *protos.GetVersionRequest) (*protos.GetVersionResponse, error) {
	var version string
	// 记录日志
	logger.Infof("Received request GetVersion: %v", in)
	file, _ := os.Open("Makefile")
	defer file.Close()
	// 创建一个 bufio 读取器
	reader := bufio.NewReader(file)
	// 逐行读取文件内容
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break // 文件读取完毕或出现错误
		}

		// 在这里对每一行进行解析
		// 这里只是简单地打印每一行的内容，你可以根据实际需求进行解析处理
		// fmt.Println("Line:", line)
		// 匹配字符串
		tagPresent := strings.Contains(line, "tag=")
		if tagPresent {
			version = line[len(line)-6:]
			break
		}
	}
	if version == "" {
		return &protos.GetVersionResponse{Major: 1, Minor: 3, Patch: 0}, nil
	}
	list := strings.Split(version, ".")
	major, _ := strconv.Atoi(list[0])
	minor, _ := strconv.Atoi(list[1])
	patch, _ := strconv.Atoi(list[2])
	return &protos.GetVersionResponse{Major: uint32(major), Minor: uint32(minor), Patch: uint32(patch)}, nil
}

func (s *serverConfig) GetAvailablePartitions(ctx context.Context, in *protos.GetAvailablePartitionsRequest) (*protos.GetAvailablePartitionsResponse, error) {
	// 先获取account的partition qos
	// 在获取user的partition qos
	// 查账户和用户之间有没有关联关系

	return &protos.GetAvailablePartitionsResponse{}, nil
}

func (s *serverConfig) GetClusterConfig(ctx context.Context, in *protos.GetClusterConfigRequest) (*protos.GetClusterConfigResponse, error) {
	var (
		partitions []*protos.Partition
	)
	logger.Infof("Received request GetClusterConfig: %v", in)

	// 获取系统Qos
	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")
	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos is not exists.")
	}

	for _, part := range config.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		partitionName := part.Name
		request := &protos.QueryPartitionInfoRequest{
			PartitionName: partitionName,
		}
		response, err := stubCraneCtld.QueryPartitionInfo(context.Background(), request)
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
		}
		partitionValue := response.GetPartitionInfo()[0]
		logger.Infof("%v", response.GetPartitionInfo())
		partitions = append(partitions, &protos.Partition{
			Name:  partitionValue.GetName(),
			MemMb: partitionValue.GetTotalMem() / (1024 * 1024),
			// Cores: uint32(partitionValue.GetTotalCpus()),
			Cores: uint32(partitionValue.GetTotalCpu()),
			Nodes: partitionValue.GetTotalNodes(),
			Qos:   qosListValue, // QOS是强行加进去的
		})
	}
	logger.Infof("%v", partitions)
	return &protos.GetClusterConfigResponse{Partitions: partitions, SchedulerName: "Crane"}, nil
}

func (s *serverUser) AddUserToAccount(ctx context.Context, in *protos.AddUserToAccountRequest) (*protos.AddUserToAccountResponse, error) {
	var (
		allowedPartitionQosList []*protos.UserInfo_AllowedPartitionQos
	)
	logger.Infof("Received request AddUserToAccount: %v", in)

	// 获取crane中QOS列表
	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")

	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos is not exists.")
	}

	// 获取计算分区 配置qos
	for _, partition := range config.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &protos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosListValue,
			DefaultQos:    qosListValue[0],
		})
	}
	uid, err := utils.GetUidByUserName(in.UserId)
	if err != nil {
		return nil, utils.RichError(codes.NotFound, "USER_NOT_FOUND", "The user is not exists.")
	}
	user := &protos.UserInfo{
		Uid:                     uint32(uid),
		Name:                    in.UserId,
		Account:                 in.AccountName,
		Blocked:                 false,
		AllowedPartitionQosList: allowedPartitionQosList,
		AdminLevel:              protos.UserInfo_None, // none
	}
	// 添加用户到账户下的请求体
	request := &protos.AddUserRequest{
		Uid:  0,
		User: user,
	}
	response, err := stubCraneCtld.AddUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", response.GetReason())
	}
	return &protos.AddUserToAccountResponse{}, nil
}

func (s *serverUser) RemoveUserFromAccount(ctx context.Context, in *protos.RemoveUserFromAccountRequest) (*protos.RemoveUserFromAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request RemoveUserFromAccount: %v", in)
	request := &protos.DeleteEntityRequest{
		Uid:        0, // 操作者
		EntityType: protos.EntityType_User,
		Account:    in.AccountName,
		Name:       in.UserId,
	}

	response, err := stubCraneCtld.DeleteEntity(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.RemoveUserFromAccountResponse{}, nil
}

func (s *serverUser) BlockUserInAccount(ctx context.Context, in *protos.BlockUserInAccountRequest) (*protos.BlockUserInAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request BlockUserInAccount: %v", in)
	request := &protos.BlockAccountOrUserRequest{
		Block:      true,
		Uid:        0, // 操作者
		EntityType: protos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.BlockUserInAccountResponse{}, nil
}

func (s *serverUser) UnblockUserInAccount(ctx context.Context, in *protos.UnblockUserInAccountRequest) (*protos.UnblockUserInAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request UnblockUserInAccount: %v", in)
	request := &protos.BlockAccountOrUserRequest{
		Block:      false,
		Uid:        0,
		EntityType: protos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.UnblockUserInAccountResponse{}, nil
}

func (s *serverUser) QueryUserInAccountBlockStatus(ctx context.Context, in *protos.QueryUserInAccountBlockStatusRequest) (*protos.QueryUserInAccountBlockStatusResponse, error) {
	var (
		blocked bool
	)
	// 记录日志
	logger.Infof("Received request QueryUserInAccountBlockStatus: %v", in)
	request := &protos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: protos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	for _, v := range response.GetUserList() {
		blocked = v.GetBlocked()
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	return &protos.QueryUserInAccountBlockStatusResponse{Blocked: blocked}, nil
}

func (s *serverAccount) ListAccounts(ctx context.Context, in *protos.ListAccountsRequest) (*protos.ListAccountsResponse, error) {
	var (
		accountList []string
	)
	// 记录日志
	logger.Infof("Received request ListAccounts: %v", in)
	// 请求体
	request := &protos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: protos.EntityType_User,
		Name:       in.UserId,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}

	// 获取账户列表信息
	for _, list := range response.GetUserList() {
		if strings.Contains(list.Account, "*") {
			account := list.Account[:len(list.Account)-1]
			accountList = append(accountList, account)
		} else {
			accountList = append(accountList, list.Account)
		}
	}
	return &protos.ListAccountsResponse{Accounts: accountList}, nil
}

func (s *serverAccount) CreateAccount(ctx context.Context, in *protos.CreateAccountRequest) (*protos.CreateAccountResponse, error) {
	var (
		partitionList           []string
		qosList                 []string
		allowedPartitionQosList []*protos.UserInfo_AllowedPartitionQos
	)
	// 记录日志
	logger.Infof("Received request CreateAccount: %v", in)
	// 获取计算分区信息
	for _, partition := range config.Partitions {
		partitionList = append(partitionList, partition.Name)
	}
	// 获取系统QOS
	qosList, _ = utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")
	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos is not exists.")
	}

	AccountInfo := &protos.AccountInfo{
		Name:              in.AccountName,
		Description:       "Create account in crane.",
		AllowedPartitions: partitionList,
		DefaultQos:        qosListValue[0],
		AllowedQosList:    qosListValue,
	}
	// 创建账户请求体
	request := &protos.AddAccountRequest{
		Uid:     uint32(os.Getuid()),
		Account: AccountInfo,
	}
	response, err := stubCraneCtld.AddAccount(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	// 账户创建成功后，将用户添加至账户中
	for _, partition := range config.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &protos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosListValue,
			DefaultQos:    qosListValue[0],
		})
	}
	uid, err := utils.GetUidByUserName(in.OwnerUserId)
	if err != nil {
		return nil, utils.RichError(codes.NotFound, "USER_NOT_FOUND", "The user is not exists.")
	}
	user := &protos.UserInfo{
		Uid:                     uint32(uid),
		Name:                    in.OwnerUserId,
		Account:                 in.AccountName,
		Blocked:                 false,
		AllowedPartitionQosList: allowedPartitionQosList,
		AdminLevel:              protos.UserInfo_Operator,
	}
	requestAddUser := &protos.AddUserRequest{
		Uid:  0,
		User: user,
	}
	responseUser, err := stubCraneCtld.AddUser(context.Background(), requestAddUser)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if responseUser.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", responseUser.GetReason())
	}
	return &protos.CreateAccountResponse{}, nil
}

func (s *serverAccount) BlockAccount(ctx context.Context, in *protos.BlockAccountRequest) (*protos.BlockAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request BlockAccount: %v", in)
	// 请求体 封锁账户
	request := &protos.BlockAccountOrUserRequest{
		Block:      true,
		EntityType: protos.EntityType_Account,
		Name:       in.AccountName,
		Uid:        0,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.GetReason())
	} else {
		return &protos.BlockAccountResponse{}, nil
	}
}

func (s *serverAccount) UnblockAccount(ctx context.Context, in *protos.UnblockAccountRequest) (*protos.UnblockAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request UnblockAccount: %v", in)
	// 请求体 解封账户
	request := &protos.BlockAccountOrUserRequest{
		Block:      false,
		EntityType: protos.EntityType_Account,
		Name:       in.AccountName,
		Uid:        0,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.GetReason())
	} else {
		return &protos.UnblockAccountResponse{}, nil
	}
}

func (s *serverAccount) GetAllAccountsWithUsers(ctx context.Context, in *protos.GetAllAccountsWithUsersRequest) (*protos.GetAllAccountsWithUsersResponse, error) {
	var (
		accounts []*protos.ClusterAccountInfo
	)
	// 记录日志
	logger.Infof("Received request GetAllAccountsWithUsers: %v", in)
	request := &protos.QueryEntityInfoRequest{
		Uid: 0,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	// 获取所有账户信息
	for _, account := range response.GetAccountList() {
		var userInfo []*protos.ClusterAccountInfo_UserInAccount
		requestUser := &protos.QueryEntityInfoRequest{
			Uid:        0,
			Account:    account.GetName(),
			EntityType: protos.EntityType_User,
		}
		// 获取单个账户下用户信息
		responseUser, _ := stubCraneCtld.QueryEntityInfo(context.Background(), requestUser)
		for _, user := range responseUser.GetUserList() {
			userInfo = append(userInfo, &protos.ClusterAccountInfo_UserInAccount{
				UserId:   user.GetName(),
				UserName: user.GetName(),
				Blocked:  user.GetBlocked(),
			})
		}
		accounts = append(accounts, &protos.ClusterAccountInfo{
			AccountName: account.GetName(),
			Blocked:     account.GetBlocked(),
			Users:       userInfo,
		})
	}
	return &protos.GetAllAccountsWithUsersResponse{Accounts: accounts}, nil
}

func (s *serverAccount) QueryAccountBlockStatus(ctx context.Context, in *protos.QueryAccountBlockStatusRequest) (*protos.QueryAccountBlockStatusResponse, error) {
	var (
		blocked bool
	)
	logger.Infof("Received request QueryAccountBlockStatus: %v", in)
	// 请求体
	request := &protos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: protos.EntityType_Account,
		Name:       in.AccountName,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	if response.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", response.GetReason())
	}

	// 获取单个用户的封锁状态
	for _, v := range response.GetAccountList() {
		blocked = v.GetBlocked()
	}

	return &protos.QueryAccountBlockStatusResponse{Blocked: blocked}, nil
}

func (s *serverJob) CancelJob(ctx context.Context, in *protos.CancelJobRequest) (*protos.CancelJobResponse, error) {
	logger.Infof("Received request CancelJob: %v", in)
	request := &protos.CancelTaskRequest{
		OperatorUid:   0,
		FilterTaskIds: []uint32{uint32(in.JobId)},
		FilterState:   protos.TaskStatus_Invalid,
	}
	_, err := stubCraneCtld.CancelTask(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", "Crane service call failed.")
	}
	return &protos.CancelJobResponse{}, nil
}

func (s *serverJob) QueryJobTimeLimit(ctx context.Context, in *protos.QueryJobTimeLimitRequest) (*protos.QueryJobTimeLimitResponse, error) {
	var (
		jobIdList []uint32
		seconds   uint64
	)
	logger.Infof("Received request QueryJobTimeLimit: %v", in)

	jobIdList = append(jobIdList, in.JobId)
	request := &protos.QueryTasksInfoRequest{
		FilterTaskIds:               jobIdList,
		OptionIncludeCompletedTasks: true, // 包含运行结束的作业
	}
	response, err := stubCraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	taskInfoList := response.GetTaskInfoList()
	if len(taskInfoList) == 0 {
		message := fmt.Sprintf("Task #%d was not found in crane.", in.JobId)
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", message)
	}
	if response.GetOk() == true {
		for _, taskInfo := range taskInfoList {
			timeLimit := taskInfo.GetTimeLimit()
			seconds = uint64(timeLimit.GetSeconds())
		}
		return &protos.QueryJobTimeLimitResponse{TimeLimitMinutes: seconds / 60}, nil
	}
	return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
}

func (s *serverJob) ChangeJobTimeLimit(ctx context.Context, in *protos.ChangeJobTimeLimitRequest) (*protos.ChangeJobTimeLimitResponse, error) {
	var (
		jobIdList []uint32
		seconds   uint64
	)
	logger.Infof("Received request ChangeJobTimeLimit: %v", in)
	// 查询请求体
	jobIdList = append(jobIdList, in.JobId)
	requestLimitTime := &protos.QueryTasksInfoRequest{
		FilterTaskIds: jobIdList,
	}

	responseLimitTime, err := stubCraneCtld.QueryTasksInfo(context.Background(), requestLimitTime)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	taskInfoList := responseLimitTime.GetTaskInfoList()
	if len(taskInfoList) == 0 {
		message := fmt.Sprintf("Task #%d was not found in crane.", in.JobId)
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", message)
	}
	if responseLimitTime.GetOk() == true {
		for _, taskInfo := range taskInfoList {
			timeLimit := taskInfo.GetTimeLimit()
			seconds = uint64(timeLimit.GetSeconds())
		}
	}

	// 这个地方需要做校验，如果小于0的话直接返回
	if in.DeltaMinutes*60+int64(seconds) <= 0 {
		// 直接返回
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", "Time limit should be greater than 0.")
	}
	// 修改时长限制的请求体
	request := &protos.ModifyTaskRequest{
		TaskId: in.JobId,
		Value: &protos.ModifyTaskRequest_TimeLimitSeconds{
			TimeLimitSeconds: in.DeltaMinutes*60 + int64(seconds),
		},
	}
	response, err := stubCraneCtld.ModifyTask(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", response.GetReason())
	}
	return &protos.ChangeJobTimeLimitResponse{}, nil
}

func (s *serverJob) GetJobById(ctx context.Context, in *protos.GetJobByIdRequest) (*protos.GetJobByIdResponse, error) {
	var (
		elapsedSeconds int64
		state          string
		reason         string
	)
	logger.Infof("Received request GetJobById: %v", in)
	// 请求体
	request := &protos.QueryTasksInfoRequest{
		FilterTaskIds:               []uint32{uint32(in.JobId)},
		OptionIncludeCompletedTasks: true,
	}
	response, err := stubCraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}
	if len(response.GetTaskInfoList()) == 0 {
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", "The job not found in crane.")
	}
	// 获取作业信息
	TaskInfoList := response.GetTaskInfoList()[0]
	if TaskInfoList.GetStatus() == protos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - TaskInfoList.GetStartTime().Seconds
	} else if TaskInfoList.GetStatus() == protos.TaskStatus_Pending {
		elapsedSeconds = 0
	}
	// 获取作业时长
	// elapsedSeconds = TaskInfoList.GetEndTime().Seconds - TaskInfoList.GetStartTime().Seconds
	if TaskInfoList.GetStatus() == protos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - TaskInfoList.GetStartTime().Seconds
	} else if TaskInfoList.GetStatus() == protos.TaskStatus_Pending {
		elapsedSeconds = 0
	} else {
		elapsedSeconds = TaskInfoList.GetEndTime().Seconds - TaskInfoList.GetStartTime().Seconds
	}

	// 获取cpu核分配数
	// cpusAlloc := TaskInfoList.GetAllocCpus()
	cpusAlloc := TaskInfoList.GetAllocCpu()
	cpusAllocInt32 := int32(cpusAlloc)
	// 获取节点列表
	nodeList := TaskInfoList.GetCranedList()

	if TaskInfoList.GetStatus().String() == "Completed" {
		state = "COMPLETED"
		reason = "ENDED"
	} else if TaskInfoList.GetStatus().String() == "Failed" {
		state = "FAILED"
		reason = "ENDED"
	} else if TaskInfoList.GetStatus().String() == "Cancelled" {
		state = "CANCELLED"
		reason = "ENDED"
	} else if TaskInfoList.GetStatus().String() == "Running" {
		state = "RUNNING"
		reason = "Running"
	} else if TaskInfoList.GetStatus().String() == "Pending" {
		state = "PENDING"
		reason = "Pending"
	}

	if len(in.Fields) == 0 {
		jobInfo := &protos.JobInfo{
			JobId:            TaskInfoList.GetTaskId(),
			Name:             TaskInfoList.GetName(),
			Account:          TaskInfoList.GetAccount(),
			User:             TaskInfoList.GetUsername(),
			Partition:        TaskInfoList.GetPartition(),
			NodeList:         &nodeList,
			StartTime:        TaskInfoList.GetStartTime(),
			EndTime:          TaskInfoList.GetEndTime(),
			TimeLimitMinutes: TaskInfoList.GetTimeLimit().Seconds / 60, // 转换成分钟数
			WorkingDirectory: TaskInfoList.GetCwd(),
			CpusAlloc:        &cpusAllocInt32,
			State:            state,
			ElapsedSeconds:   &elapsedSeconds,
			Reason:           &reason,
			Qos:              TaskInfoList.GetQos(),
			SubmitTime:       TaskInfoList.GetStartTime(),
		}
		return &protos.GetJobByIdResponse{Job: jobInfo}, nil
	}
	jobInfo := &protos.JobInfo{}
	for _, field := range in.Fields {
		switch field {
		case "job_id":
			jobInfo.JobId = TaskInfoList.GetTaskId()
		case "name":
			jobInfo.Name = TaskInfoList.GetName()
		case "account":
			jobInfo.Account = TaskInfoList.GetAccount()
		case "user":
			jobInfo.User = TaskInfoList.GetUsername()
		case "partition":
			jobInfo.Partition = TaskInfoList.GetPartition()
		case "node_list":
			jobInfo.NodeList = &nodeList
		case "start_time":
			jobInfo.StartTime = TaskInfoList.GetStartTime()
		case "end_time":
			jobInfo.EndTime = TaskInfoList.GetEndTime()
		case "time_limit_minutes":
			jobInfo.TimeLimitMinutes = TaskInfoList.GetTimeLimit().Seconds / 60
		case "working_directory":
			jobInfo.WorkingDirectory = TaskInfoList.GetCwd()
		case "cpus_alloc":
			jobInfo.CpusAlloc = &cpusAllocInt32
		case "state":
			jobInfo.State = state
		case "elapsed_seconds":
			jobInfo.ElapsedSeconds = &elapsedSeconds
		case "reason":
			jobInfo.Reason = &reason
		case "qos":
			jobInfo.Qos = TaskInfoList.GetQos()
		case "submit_time":
			jobInfo.SubmitTime = TaskInfoList.GetStartTime()
		}
	}
	return &protos.GetJobByIdResponse{Job: jobInfo}, nil
}

func (s *serverJob) GetJobs(ctx context.Context, in *protos.GetJobsRequest) (*protos.GetJobsResponse, error) {
	var (
		startTimeFilter int64
		endTimeFilter   int64
		statesList      []protos.TaskStatus
		accountList     []string
		request         *protos.QueryTasksInfoRequest
		jobsInfo        []*protos.JobInfo
		totalNum        uint32
		// submitTimeTimestamp *timestamppb.Timestamp
	)
	logger.Infof("Received request GetJobs: %v", in)

	if in.Filter != nil {
		if in.Filter.EndTime != nil {
			startTimeFilter = in.Filter.EndTime.StartTime.GetSeconds()
			endTimeFilter = in.Filter.EndTime.EndTime.GetSeconds()
			statesList = utils.GetCraneStatesList(in.Filter.States)
			if startTimeFilter == 0 && endTimeFilter != 0 {
				// endTimeProto := timestamppb.New(time.Unix(endTimeFilter, 0))
				// 新增endTimeInterval 代码
				endTimeInterval := &protos.TimeInterval{
					UpperBound: timestamppb.New(time.Unix(endTimeFilter, 0)), // 设置结束时间的时间戳
				}

				// UpperBound 表示右区间
				// LowerBound 表示左区间

				request = &protos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else if startTimeFilter != 0 && endTimeFilter != 0 {
				endTimeInterval := &protos.TimeInterval{
					LowerBound: timestamppb.New(time.Unix(startTimeFilter, 0)),
					UpperBound: timestamppb.New(time.Unix(endTimeFilter, 0)),
				}

				request = &protos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else if startTimeFilter != 0 && endTimeFilter == 0 {
				// startTimeProto := timestamppb.New(time.Unix(startTimeFilter, 0))
				endTimeInterval := &protos.TimeInterval{
					LowerBound: timestamppb.New(time.Unix(startTimeFilter, 0)),
				}
				request = &protos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else {
				request = &protos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			}
		} else {
			statesList = utils.GetCraneStatesList(in.Filter.States)
			request = &protos.QueryTasksInfoRequest{
				FilterTaskStates:            statesList,
				FilterUsers:                 in.Filter.Users,
				FilterAccounts:              accountList,
				OptionIncludeCompletedTasks: true,
				NumLimit:                    99999999,
			}
		}
	} else {
		// 没有筛选条件的请求体
		request = &protos.QueryTasksInfoRequest{
			OptionIncludeCompletedTasks: true,
			NumLimit:                    99999999,
		}
	}
	response, err := stubCraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if response.GetOk() == false {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}
	if len(response.GetTaskInfoList()) == 0 {
		totalNum = uint32(len(response.GetTaskInfoList()))
		return &protos.GetJobsResponse{Jobs: jobsInfo, TotalCount: &totalNum}, nil
	}
	totalNum = uint32(len(response.GetTaskInfoList()))
	for _, job := range response.GetTaskInfoList() {
		var elapsedSeconds int64
		var state string
		var reason string = "no reason"
		var nodeNum int32
		var endTime *timestamppb.Timestamp
		var gpusAlloc int32 = 0
		var memAllocMb int64 = 0
		// var endtime *timestamppb.Timestamp
		if job.GetStatus() == protos.TaskStatus_Running {
			elapsedSeconds = time.Now().Unix() - job.GetStartTime().Seconds
		} else if job.GetStatus() == protos.TaskStatus_Pending {
			elapsedSeconds = 0
		} else {
			elapsedSeconds = job.GetEndTime().Seconds - job.GetStartTime().Seconds
		}
		// cpusAlloc := job.GetAllocCpus()
		cpusAlloc := job.GetAllocCpu()
		cpusAllocInt32 := int32(cpusAlloc)
		nodeList := job.GetCranedList()

		if job.GetStatus().String() == "Completed" {
			state = "COMPLETED"
			reason = "ENDED"
			endTime = job.GetEndTime()
		} else if job.GetStatus().String() == "Failed" {
			state = "FAILED"
			reason = "ENDED"
			endTime = job.GetEndTime()
		} else if job.GetStatus().String() == "Cancelled" {
			state = "CANCELLED"
			reason = "ENDED"
			endTime = job.GetEndTime()
		} else if job.GetStatus().String() == "Running" {
			state = "RUNNING"
			reason = "Running"
		} else if job.GetStatus().String() == "Pending" {
			state = "PENDING"
			reason = "Pending"
		} else if job.GetStatus().String() == "ExceedTimeLimit" {
			state = "TIMEOUT"
			reason = "Timeout"
		} else {
			state = "IVALID"
			reason = "Ivalid"
		}
		nodeNum = int32(job.GetNodeNum())
		if len(in.Fields) == 0 {
			jobsInfo = append(jobsInfo, &protos.JobInfo{
				JobId:            job.GetTaskId(),
				Name:             job.GetName(),
				Account:          job.GetAccount(),
				User:             job.GetUsername(),
				Partition:        job.GetPartition(),
				StartTime:        job.GetStartTime(),
				EndTime:          endTime,
				NodesAlloc:       &nodeNum,
				TimeLimitMinutes: job.GetTimeLimit().Seconds / 60,
				WorkingDirectory: job.GetCwd(),
				State:            state,
				NodeList:         &nodeList,
				CpusAlloc:        &cpusAllocInt32,
				ElapsedSeconds:   &elapsedSeconds,
				Qos:              job.GetQos(),
				Reason:           &reason,
				SubmitTime:       job.GetStartTime(), // 没有提交时间，用开始时间来代替
				GpusAlloc:        &gpusAlloc,
				MemAllocMb:       &memAllocMb,
			})
		} else {
			subJobInfo := &protos.JobInfo{}
			for _, field := range in.Fields {
				switch field {
				case "job_id":
					subJobInfo.JobId = job.GetTaskId()
				case "name":
					subJobInfo.Name = job.GetName()
				case "account":
					subJobInfo.Account = job.GetAccount()
				case "user":
					subJobInfo.User = job.GetUsername()
				case "partition":
					subJobInfo.Partition = job.GetPartition()
				case "node_list":
					subJobInfo.NodeList = &nodeList
				case "start_time":
					subJobInfo.StartTime = job.GetStartTime()
				case "end_time":
					subJobInfo.EndTime = endTime
				case "time_limit_minutes":
					subJobInfo.TimeLimitMinutes = job.GetTimeLimit().Seconds / 60
				case "working_directory":
					subJobInfo.WorkingDirectory = job.GetCwd()
				case "cpus_alloc":
					subJobInfo.CpusAlloc = &cpusAllocInt32
				case "state":
					subJobInfo.State = state
				case "elapsed_seconds":
					subJobInfo.ElapsedSeconds = &elapsedSeconds
				case "qos":
					subJobInfo.Qos = job.GetQos()
				case "submit_time":
					subJobInfo.SubmitTime = job.GetStartTime() // 没有提交时间
				case "reason":
					subJobInfo.Reason = &reason
				case "nodes_alloc":
					subJobInfo.NodesAlloc = &nodeNum
				case "gpus_alloc":
					subJobInfo.GpusAlloc = &gpusAlloc
				case "mem_alloc_mb":
					subJobInfo.MemAllocMb = &memAllocMb
				}
			}
			jobsInfo = append(jobsInfo, subJobInfo)
		}
	}
	// 这里进行排序
	if in.Sort != nil && len(jobsInfo) != 0 {
		// 这里把slurm适配器的代码拿过来就可以了
		var sortKey string
		if in.Sort.GetField() == "" {
			sortKey = "JobId" // 默认jobid进行排序
		} else {
			sortKey = in.Sort.GetField()
			// 字段转换成首字母大写的字符串
			words := strings.Split(sortKey, "_")
			for i := 0; i < len(words); i++ {
				words[i] = strings.Title(words[i])
			}
			sortKey = strings.Join(words, "")
		}
		sortOrder := in.Sort.GetOrder().String()
		sortJobinfo := utils.SortJobInfo(sortKey, sortOrder, jobsInfo)
		return &protos.GetJobsResponse{Jobs: sortJobinfo, TotalCount: &totalNum}, nil
	}
	logger.Infof("%v", jobsInfo)
	return &protos.GetJobsResponse{Jobs: jobsInfo, TotalCount: &totalNum}, nil
}

func (s *serverJob) SubmitJob(ctx context.Context, in *protos.SubmitJobRequest) (*protos.SubmitJobResponse, error) {
	var (
		craneOptions string
		stdout       string
		memory       uint64
		homedir      string
	)
	logger.Infof("Received request SubmitJob: %v", in)

	if in.Stdout != nil {
		stdout = *in.Stdout
	} else { // 可选参数没传的情况
		stdout = "job.%j.out"
	}

	if in.MemoryMb != nil {
		memory = *in.MemoryMb / uint64(in.NodeCount)
	} else {
		memory = 800
	}

	uid, err := utils.GetUidByUserName(in.UserId) // 获取用户的uid
	if err != nil {
		return nil, utils.RichError(codes.NotFound, "USER_NOT_FOUND", "The user is not exists.")
	}
	// 拼凑成绝对路径的工作目录

	isAbsolute := filepath.IsAbs(in.WorkingDirectory)
	if !isAbsolute {
		homedirTemp, _ := utils.GetUserHomedir(in.UserId)
		homedir = homedirTemp + "/" + in.WorkingDirectory
	} else {
		homedir = in.WorkingDirectory
	}

	// 请求体
	request := &protos.SubmitBatchTaskRequest{
		Task: &protos.TaskToCtld{
			TimeLimit:     durationpb.New(time.Duration(*in.TimeLimitMinutes*uint32(60)) * time.Second),
			PartitionName: in.Partition,
			Resources: &protos.Resources{
				AllocatableResource: &protos.AllocatableResource{
					CpuCoreLimit:       float64(in.CoreCount) * 1,
					MemoryLimitBytes:   memory * 1024 * 1024,
					MemorySwLimitBytes: memory * 1024 * 1024,
				},
			},
			Type:            protos.TaskType_Batch,
			Uid:             uint32(uid),
			Account:         in.Account,
			Name:            in.JobName,
			NodeNum:         in.NodeCount,
			NtasksPerNode:   1,
			CpusPerTask:     float64(in.CoreCount),
			RequeueIfFailed: false,
			Payload: &protos.TaskToCtld_BatchMeta{
				BatchMeta: &protos.BatchTaskAdditionalMeta{
					ShScript:          "#!/bin/bash\n" + craneOptions + in.Script,
					OutputFilePattern: stdout,
				},
			},
			Cwd: homedir, // 工作目录不存在的情况下不会生成输出文件
			Qos: *in.Qos,
		},
	}
	response, err := stubCraneCtld.SubmitBatchTask(context.Background(), request)
	if err != nil {
		logger.Infof("%v", err.Error())
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
	}
	if response.GetOk() == false {
		logger.Infof("%v", response.GetReason())
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	return &protos.SubmitJobResponse{JobId: response.GetTaskId(), GeneratedScript: "#!/bin/bash\n" + craneOptions + in.Script}, nil
}

func main() {
	// 创建日志实例
	logger = logrus.New()
	// 设置日志输出格式为JSON
	logger.SetFormatter(&logrus.JSONFormatter{})
	// 设置日志级别为Info
	logger.SetLevel(logrus.InfoLevel)
	// // 设置日志输出到控制台和文件中
	// file, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer file.Close()
	// 创建一个 lumberjack.Logger，用于日志轮转配置
	logFile := &lumberjack.Logger{
		Filename:   "server.log", // 日志文件路径
		MaxSize:    10,           // 日志文件的最大大小（以MB为单位）
		MaxBackups: 3,            // 保留的旧日志文件数量
		MaxAge:     28,           // 保留的旧日志文件的最大天数
		LocalTime:  true,         // 使用本地时间戳
		Compress:   true,         // 是否压缩旧日志文件
	}
	logger.SetOutput(io.MultiWriter(os.Stdout, logFile))
	defer logFile.Close()

	// CraneCtld 客户端
	serverAddr := fmt.Sprintf("%s:%s", config.ControlMachine, config.CraneCtldListenPort)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Cannot connect to CraneCtld: " + err.Error())
	}
	defer conn.Close()
	stubCraneCtld = protos.NewCraneCtldClient(conn)

	// 监听本地8972端口
	lis, err := net.Listen("tcp", ":8972")
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
		return
	}
	s := grpc.NewServer()                            // 创建gRPC服务器
	protos.RegisterJobServiceServer(s, &serverJob{}) // 注册服务
	protos.RegisterAccountServiceServer(s, &serverAccount{})
	protos.RegisterConfigServiceServer(s, &serverConfig{})
	protos.RegisterUserServiceServer(s, &serverUser{})
	protos.RegisterVersionServiceServer(s, &serverVersion{})
	// 启动服务
	err = s.Serve(lis)
	if err != nil {
		fmt.Printf("failed to serve: %v", err)
		return
	}
}
