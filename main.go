package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/utils"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	config        *utils.Config
	stubCraneCtld craneProtos.CraneCtldClient
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

type serverApp struct {
	protos.UnimplementedAppServiceServer
}

func init() {
	config = utils.ParseConfig(utils.DefaultConfigPath)
}

// app
func (s *serverApp) GetAppconnectionInfo(ctx context.Context, in *protos.GetAppConnectionInfoRequest) (*protos.GetAppConnectionInfoResponse, error) {
	return &protos.GetAppConnectionInfoResponse{}, nil
}

// version
func (s *serverVersion) GetVersion(ctx context.Context, in *protos.GetVersionRequest) (*protos.GetVersionResponse, error) {
	// 记录日志
	logger.Infof("Received request GetVersion: %v", in)
	return &protos.GetVersionResponse{Major: 1, Minor: 5, Patch: 0}, nil

}

func (s *serverConfig) GetAvailablePartitions(ctx context.Context, in *protos.GetAvailablePartitionsRequest) (*protos.GetAvailablePartitionsResponse, error) {
	// 先获取account的partition qos
	// 在获取user的partition qos
	// 查账户和用户之间有没有关联关系
	var (
		partitions []*protos.Partition
	)
	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")
	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	for _, part := range config.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		partitionName := part.Name
		request := &craneProtos.QueryPartitionInfoRequest{
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
	return &protos.GetAvailablePartitionsResponse{Partitions: partitions}, nil
}

// 先在这里实现getclusterinfo的逻辑
func (s *serverConfig) GetClusterInfo(ctx context.Context, in *protos.GetClusterInfoRequest) (*protos.GetClusterInfoResponse, error) {
	var (
		partitions []*protos.PartitionInfo
	)
	logger.Infof("Received request GetClusterInfo: %v", in)
	for _, part := range config.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		var runningNodes uint32
		var state protos.PartitionInfo_PartitionStatus
		partitionName := part.Name // 获取分区名
		// 请求体
		request := &craneProtos.QueryPartitionInfoRequest{
			PartitionName: partitionName,
		}

		response, err := stubCraneCtld.QueryPartitionInfo(context.Background(), request)
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
		}
		// 这里还要拿cqueue的值
		runningJobCmd := fmt.Sprintf("cqueue -p %s -t r --noheader | wc -l", part.Name)                             // 获取正在运行作业的个数
		pendingJobCmd := fmt.Sprintf("cqueue -p %s -t p --noheader | wc -l", part.Name)                             // 获取正在排队作业的个数
		runningNodeCmd := fmt.Sprintf("cinfo -p %s -t alloc,mix | awk 'NR>1 {sum+=$4} END {print sum}'", part.Name) // 获取正在运行节点数

		runningJobNumStr, err := utils.RunCommand(runningJobCmd) // 转化一下
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_RUNCOMMAND_ERROR", err.Error())
		}
		pendingJobNumStr, err := utils.RunCommand(pendingJobCmd) // 转化一下
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_RUNCOMMAND_ERROR", err.Error())
		}
		runningNodeStr, err := utils.RunCommand(runningNodeCmd)
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_RUNCOMMAND_ERROR", err.Error())
		}
		runningJobNum, _ := strconv.Atoi(runningJobNumStr)
		pendingJobNum, _ := strconv.Atoi(pendingJobNumStr)
		if runningNodeStr == "INFO[0000] No matching partitions were found for the given filter." {
			runningNodes = 0
		} else {
			// 不为0的情况
			tempNodes, _ := strconv.Atoi(runningNodeStr)
			runningNodes = uint32(tempNodes)
		}

		partitionValue := response.GetPartitionInfo()[0]
		logger.Infof("%v", response.GetPartitionInfo())
		resultRatio := float64(runningNodes) / float64(partitionValue.TotalNodes)
		percentage := int(resultRatio * 100) // 保留整数
		if partitionValue.State == craneProtos.PartitionState_PARTITION_UP {
			state = protos.PartitionInfo_AVAILABLE
		} else {
			state = protos.PartitionInfo_NOT_AVAILABLE
		}
		partitions = append(partitions, &protos.PartitionInfo{
			PartitionName:         partitionValue.GetName(),
			NodeCount:             partitionValue.TotalNodes,
			RunningNodeCount:      runningNodes,
			IdleNodeCount:         partitionValue.AliveNodes - runningNodes, // 减去正在运行的节点 partitionValue.TotalNodes - runningNodes,
			NotAvailableNodeCount: partitionValue.TotalNodes - partitionValue.AliveNodes,
			CpuCoreCount:          uint32(partitionValue.TotalCpu),
			RunningCpuCount:       uint32(partitionValue.AllocCpu),
			IdleCpuCount:          uint32(partitionValue.TotalCpu) - uint32(partitionValue.AllocCpu),
			NotAvailableCpuCount:  uint32(partitionValue.TotalCpu) - uint32(partitionValue.AvailCpu) - uint32(partitionValue.AllocCpu),
			JobCount:              uint32(runningJobNum) + uint32(pendingJobNum),
			RunningJobCount:       uint32(runningJobNum),
			PendingJobCount:       uint32(pendingJobNum),
			UsageRatePercentage:   uint32(percentage),
			PartitionStatus:       state,
		})

	}
	return &protos.GetClusterInfoResponse{ClusterName: config.ClusterName, Partitions: partitions}, nil
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
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	for _, part := range config.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		partitionName := part.Name
		request := &craneProtos.QueryPartitionInfoRequest{
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
		allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
	)
	logger.Infof("Received request AddUserToAccount: %v", in)

	// 获取crane中QOS列表
	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")

	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	// 获取计算分区 配置qos
	for _, partition := range config.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &craneProtos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosListValue,
			DefaultQos:    qosListValue[0],
		})
	}
	uid, err := utils.GetUidByUserName(in.UserId)
	if err != nil {
		return nil, utils.RichError(codes.NotFound, "USER_NOT_FOUND", "The user is not exists.")
	}
	user := &craneProtos.UserInfo{
		Uid:                     uint32(uid),
		Name:                    in.UserId,
		Account:                 in.AccountName,
		Blocked:                 false,
		AllowedPartitionQosList: allowedPartitionQosList,
		AdminLevel:              craneProtos.UserInfo_None, // none
	}
	// 添加用户到账户下的请求体
	request := &craneProtos.AddUserRequest{
		Uid:  0,
		User: user,
	}
	response, err := stubCraneCtld.AddUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", response.GetReason())
	}
	return &protos.AddUserToAccountResponse{}, nil
}

func (s *serverUser) RemoveUserFromAccount(ctx context.Context, in *protos.RemoveUserFromAccountRequest) (*protos.RemoveUserFromAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request RemoveUserFromAccount: %v", in)
	request := &craneProtos.DeleteEntityRequest{
		Uid:        0, // 操作者
		EntityType: craneProtos.EntityType_User,
		Account:    in.AccountName,
		Name:       in.UserId,
	}

	response, err := stubCraneCtld.DeleteEntity(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.RemoveUserFromAccountResponse{}, nil
}

func (s *serverUser) BlockUserInAccount(ctx context.Context, in *protos.BlockUserInAccountRequest) (*protos.BlockUserInAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request BlockUserInAccount: %v", in)
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		Uid:        0, // 操作者
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.BlockUserInAccountResponse{}, nil
}

func (s *serverUser) UnblockUserInAccount(ctx context.Context, in *protos.UnblockUserInAccountRequest) (*protos.UnblockUserInAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request UnblockUserInAccount: %v", in)
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
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
	request := &craneProtos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
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
	if !response.GetOk() {
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
	request := &craneProtos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
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
		allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
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

	AccountInfo := &craneProtos.AccountInfo{
		Name:              in.AccountName,
		Description:       "Create account in crane.",
		AllowedPartitions: partitionList,
		DefaultQos:        qosListValue[0],
		AllowedQosList:    qosListValue,
	}
	// 创建账户请求体
	request := &craneProtos.AddAccountRequest{
		Uid:     uint32(os.Getuid()),
		Account: AccountInfo,
	}
	response, err := stubCraneCtld.AddAccount(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	// 账户创建成功后，将用户添加至账户中
	for _, partition := range config.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &craneProtos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosListValue,
			DefaultQos:    qosListValue[0],
		})
	}
	uid, err := utils.GetUidByUserName(in.OwnerUserId)
	if err != nil {
		return nil, utils.RichError(codes.NotFound, "USER_NOT_FOUND", "The user is not exists.")
	}
	user := &craneProtos.UserInfo{
		Uid:                     uint32(uid),
		Name:                    in.OwnerUserId,
		Account:                 in.AccountName,
		Blocked:                 false,
		AllowedPartitionQosList: allowedPartitionQosList,
		AdminLevel:              craneProtos.UserInfo_Operator,
	}
	requestAddUser := &craneProtos.AddUserRequest{
		Uid:  0,
		User: user,
	}
	responseUser, err := stubCraneCtld.AddUser(context.Background(), requestAddUser)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !responseUser.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", responseUser.GetReason())
	}
	return &protos.CreateAccountResponse{}, nil
}

func (s *serverAccount) BlockAccount(ctx context.Context, in *protos.BlockAccountRequest) (*protos.BlockAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request BlockAccount: %v", in)
	// 请求体 封锁账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		EntityType: craneProtos.EntityType_Account,
		Name:       in.AccountName,
		Uid:        0,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.GetReason())
	} else {
		return &protos.BlockAccountResponse{}, nil
	}
}

func (s *serverAccount) UnblockAccount(ctx context.Context, in *protos.UnblockAccountRequest) (*protos.UnblockAccountResponse, error) {
	// 记录日志
	logger.Infof("Received request UnblockAccount: %v", in)
	// 请求体 解封账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		EntityType: craneProtos.EntityType_Account,
		Name:       in.AccountName,
		Uid:        0,
	}
	response, err := stubCraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
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
	request := &craneProtos.QueryEntityInfoRequest{
		Uid: 0,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	// 获取所有账户信息
	for _, account := range response.GetAccountList() {
		var userInfo []*protos.ClusterAccountInfo_UserInAccount
		requestUser := &craneProtos.QueryEntityInfoRequest{
			Uid:        0,
			Account:    account.GetName(),
			EntityType: craneProtos.EntityType_User,
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
	request := &craneProtos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: craneProtos.EntityType_Account,
		Name:       in.AccountName,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	if !response.GetOk() {
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
	request := &craneProtos.CancelTaskRequest{
		OperatorUid:   0,
		FilterTaskIds: []uint32{uint32(in.JobId)},
		FilterState:   craneProtos.TaskStatus_Invalid,
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
	request := &craneProtos.QueryTasksInfoRequest{
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
	if response.GetOk() {
		for _, taskInfo := range taskInfoList {
			timeLimit := taskInfo.GetTimeLimit()
			seconds = uint64(timeLimit.GetSeconds())
		}
		return &protos.QueryJobTimeLimitResponse{TimeLimitMinutes: seconds / 60}, nil
	}
	return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Get job timelimit failed.")
}

func (s *serverJob) ChangeJobTimeLimit(ctx context.Context, in *protos.ChangeJobTimeLimitRequest) (*protos.ChangeJobTimeLimitResponse, error) {
	var (
		jobIdList []uint32
		seconds   uint64
	)
	logger.Infof("Received request ChangeJobTimeLimit: %v", in)
	// 查询请求体
	jobIdList = append(jobIdList, in.JobId)
	requestLimitTime := &craneProtos.QueryTasksInfoRequest{
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
	if responseLimitTime.GetOk() {
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
	request := &craneProtos.ModifyTaskRequest{
		TaskId: in.JobId,
		Value: &craneProtos.ModifyTaskRequest_TimeLimitSeconds{
			TimeLimitSeconds: in.DeltaMinutes*60 + int64(seconds),
		},
	}
	response, err := stubCraneCtld.ModifyTask(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
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
	request := &craneProtos.QueryTasksInfoRequest{
		FilterTaskIds:               []uint32{uint32(in.JobId)},
		OptionIncludeCompletedTasks: true,
	}
	response, err := stubCraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}
	if len(response.GetTaskInfoList()) == 0 {
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", "The job not found in crane.")
	}
	// 获取作业信息
	TaskInfoList := response.GetTaskInfoList()[0]
	if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - TaskInfoList.GetStartTime().Seconds
	} else if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Pending {
		elapsedSeconds = 0
	}
	// 获取作业时长
	// elapsedSeconds = TaskInfoList.GetEndTime().Seconds - TaskInfoList.GetStartTime().Seconds
	if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - TaskInfoList.GetStartTime().Seconds
	} else if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Pending {
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
		statesList      []craneProtos.TaskStatus
		accountList     []string
		request         *craneProtos.QueryTasksInfoRequest
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
				endTimeInterval := &craneProtos.TimeInterval{
					UpperBound: timestamppb.New(time.Unix(endTimeFilter, 0)), // 设置结束时间的时间戳
				}

				// UpperBound 表示右区间
				// LowerBound 表示左区间

				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else if startTimeFilter != 0 && endTimeFilter != 0 {
				endTimeInterval := &craneProtos.TimeInterval{
					LowerBound: timestamppb.New(time.Unix(startTimeFilter, 0)),
					UpperBound: timestamppb.New(time.Unix(endTimeFilter, 0)),
				}

				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else if startTimeFilter != 0 && endTimeFilter == 0 {
				// startTimeProto := timestamppb.New(time.Unix(startTimeFilter, 0))
				endTimeInterval := &craneProtos.TimeInterval{
					LowerBound: timestamppb.New(time.Unix(startTimeFilter, 0)),
				}
				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else {
				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              accountList,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			}
		} else {
			statesList = utils.GetCraneStatesList(in.Filter.States)
			request = &craneProtos.QueryTasksInfoRequest{
				FilterTaskStates:            statesList,
				FilterUsers:                 in.Filter.Users,
				FilterAccounts:              accountList,
				OptionIncludeCompletedTasks: true,
				NumLimit:                    99999999,
			}
		}
	} else {
		// 没有筛选条件的请求体
		request = &craneProtos.QueryTasksInfoRequest{
			OptionIncludeCompletedTasks: true,
			NumLimit:                    99999999,
		}
	}
	response, err := stubCraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
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
		if job.GetStatus() == craneProtos.TaskStatus_Running {
			elapsedSeconds = time.Now().Unix() - job.GetStartTime().Seconds
		} else if job.GetStatus() == craneProtos.TaskStatus_Pending {
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
		// craneOptions string
		stdout string
		// memory       uint64
		homedir         string
		timeLimitString string
		scriptString    = "#!/bin/bash\n"
	)

	logger.Infof("Received request SubmitJob: %v", in)

	if in.Stdout != nil {
		stdout = *in.Stdout
	} else { // 可选参数没传的情况
		stdout = "job.%j.out"
	}

	// if in.MemoryMb != nil {
	// 	memory = *in.MemoryMb / uint64(in.NodeCount)
	// } else {
	// 	memory = 800
	// }

	// 拼凑成绝对路径的工作目录

	isAbsolute := filepath.IsAbs(in.WorkingDirectory)
	if !isAbsolute {
		homedirTemp, _ := utils.GetUserHomedir(in.UserId)
		homedir = homedirTemp + "/" + in.WorkingDirectory
	} else {
		homedir = in.WorkingDirectory
	}

	scriptString += "#CBATCH " + "-A " + in.Account + "\n"
	scriptString += "#CBATCH " + "-p " + in.Partition + "\n"
	if in.Qos != nil {
		scriptString += "#CBATCH " + "--qos " + *in.Qos + "\n"
	}
	scriptString += "#CBATCH " + "-J " + in.JobName + "\n"
	scriptString += "#CBATCH " + "-N " + strconv.Itoa(int(in.NodeCount)) + "\n"
	scriptString += "#CBATCH " + "--ntasks-per-node " + strconv.Itoa(1) + "\n"
	scriptString += "#CBATCH " + "-c " + strconv.Itoa(int(in.CoreCount)) + "\n"
	if in.TimeLimitMinutes != nil {
		// 要把时间换成字符串的形式
		if *in.TimeLimitMinutes < 60 {
			timeLimitString = fmt.Sprintf("00:%s:00", strconv.Itoa(int(*in.TimeLimitMinutes)))
		} else if *in.TimeLimitMinutes == 60 {
			timeLimitString = "1:00:00"
		} else {
			hours, minitues := *in.TimeLimitMinutes/60, *in.TimeLimitMinutes%60
			timeLimitString = fmt.Sprintf("%s:%s:00", strconv.Itoa(int(hours)), strconv.Itoa(int(minitues)))
		}
		scriptString += "#CBATCH " + "--time " + timeLimitString + "\n"
	}
	scriptString += "#CBATCH " + "--chdir " + homedir + "\n"
	if in.Stdout != nil {
		scriptString += "#CBATCH " + "--output " + stdout + "\n"
	}

	if in.MemoryMb != nil {
		scriptString += "#CBATCH " + "--mem " + strconv.Itoa(int(*in.MemoryMb)) + "M" + "\n"
	}
	if len(in.ExtraOptions) != 0 {
		for _, extraVale := range in.ExtraOptions {
			scriptString += "#CBATCH " + extraVale + "\n"
		}
	}
	scriptString += "#CBATCH " + "--export ALL" + "\n"
	scriptString += "#CBATCH " + "--get-user-env" + "\n"
	scriptString += in.Script

	// 将这个保存成一个脚本文件，通过脚本文件进行提交
	// 生成一个随机的文件名
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	filePath := "/tmp" + "/" + string(b) + ".sh" // 生成的脚本存放路径
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		return nil, utils.RichError(codes.Aborted, "CREATE_SCRIPT_FAILED", "Create submit script failed.")
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString(scriptString)
	writer.Flush()

	submitResult, err := utils.LocalSubmitJob(filePath, in.UserId)
	os.Remove(filePath) // 删除掉提交脚本
	if err != nil {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", submitResult)
	}
	responseList := strings.Split(strings.TrimSpace(string(submitResult)), " ")
	jobIdString := responseList[len(responseList)-1]

	jobId1, _ := strconv.Atoi(jobIdString[:len(jobIdString)-1])

	return &protos.SubmitJobResponse{JobId: uint32(jobId1), GeneratedScript: scriptString}, nil
}

func SubmitScriptAsJob(ctx context.Context, in *protos.SubmitScriptAsJobRequest) (*protos.SubmitScriptAsJobResponse, error) {
	// 获取传过来的文件内容
	logger.Infof("Received request SubmitScriptAsJob: %v", in)
	// 具体的提交逻辑
	updateScript := "#!/bin/bash\n"
	trimmedScript := strings.TrimLeft(in.Script, "\n") // 去除最前面的空行
	// 通过换行符 "\n" 分割字符串
	checkBool1 := strings.Contains(trimmedScript, "--chdir")
	checkBool2 := strings.Contains(trimmedScript, " -D ")
	if !checkBool1 && !checkBool2 {
		chdirString := fmt.Sprintf("#SBATCH --chdir=%s\n", *in.ScriptFileFullPath) // 这个地方需要更新protos文件后再处理
		updateScript = updateScript + chdirString
		for _, value := range strings.Split(trimmedScript, "\n")[1:] {
			updateScript = updateScript + value + "\n"
		}
		in.Script = updateScript
	}
	// 将这个保存成一个脚本文件，通过脚本文件进行提交
	// 生成一个随机的文件名
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	filePath := "/tmp" + "/" + string(b) + ".sh" // 生成的脚本存放路径
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		return nil, utils.RichError(codes.Aborted, "CREATE_SCRIPT_FAILED", "Create submit script failed.")
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString(in.Script)
	writer.Flush()

	submitResult, err := utils.LocalSubmitJob(filePath, in.UserId)
	os.Remove(filePath) // 删除生成的提交脚本
	if err != nil {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", submitResult)
	}
	responseList := strings.Split(strings.TrimSpace(string(submitResult)), " ")
	jobIdString := responseList[len(responseList)-1]

	jobId1, _ := strconv.Atoi(jobIdString[:len(jobIdString)-1])

	return &protos.SubmitScriptAsJobResponse{JobId: uint32(jobId1)}, nil
}

func main() {
	// 创建日志实例
	logger = logrus.New()
	// 设置日志输出格式为JSON
	logger.SetFormatter(&logrus.JSONFormatter{})
	// 设置日志级别为Info
	logger.SetLevel(logrus.InfoLevel)
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
	stubCraneCtld = craneProtos.NewCraneCtldClient(conn)

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
	protos.RegisterAppServiceServer(s, &serverApp{})
	// 启动服务
	err = s.Serve(lis)
	if err != nil {
		fmt.Printf("failed to serve: %v", err)
		return
	}
}
