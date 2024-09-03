package account

import (
	"context"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/utils"
)

type ServerAccount struct {
	protos.UnimplementedAccountServiceServer
}

func (s *ServerAccount) ListAccounts(ctx context.Context, in *protos.ListAccountsRequest) (*protos.ListAccountsResponse, error) {
	var (
		accountList []string
	)
	// 记录日志
	logrus.Infof("Received request ListAccounts: %v", in)

	// 请求体
	request := &craneProtos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
	}
	response, err := utils.CraneCtld.QueryEntityInfo(context.Background(), request)
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

func (s *ServerAccount) CreateAccount(ctx context.Context, in *protos.CreateAccountRequest) (*protos.CreateAccountResponse, error) {
	var (
		partitionList           []string
		qosList                 []string
		allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
	)
	// 记录日志
	logrus.Infof("Received request CreateAccount: %v", in)
	// 获取计算分区信息
	for _, partition := range utils.CConfig.Partitions {
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
	response, err := utils.CraneCtld.AddAccount(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", response.GetReason())
	}
	// 账户创建成功后，将用户添加至账户中
	for _, partition := range utils.CConfig.Partitions {
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
		AdminLevel:              craneProtos.UserInfo_None,
	}
	requestAddUser := &craneProtos.AddUserRequest{
		Uid:  0,
		User: user,
	}
	responseUser, err := utils.CraneCtld.AddUser(context.Background(), requestAddUser)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !responseUser.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", responseUser.GetReason())
	}
	return &protos.CreateAccountResponse{}, nil
}

func (s *ServerAccount) BlockAccount(ctx context.Context, in *protos.BlockAccountRequest) (*protos.BlockAccountResponse, error) {
	// 记录日志
	logrus.Infof("Received request BlockAccount: %v", in)
	// 请求体 封锁账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		EntityType: craneProtos.EntityType_Account,
		Name:       in.AccountName,
		Uid:        0,
	}
	response, err := utils.CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.GetReason())
	} else {
		return &protos.BlockAccountResponse{}, nil
	}
}

func (s *ServerAccount) UnblockAccount(ctx context.Context, in *protos.UnblockAccountRequest) (*protos.UnblockAccountResponse, error) {
	// 记录日志
	logrus.Infof("Received request UnblockAccount: %v", in)
	// 请求体 解封账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		EntityType: craneProtos.EntityType_Account,
		Name:       in.AccountName,
		Uid:        0,
	}
	response, err := utils.CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.GetReason())
	} else {
		return &protos.UnblockAccountResponse{}, nil
	}
}

func (s *ServerAccount) GetAllAccountsWithUsers(ctx context.Context, in *protos.GetAllAccountsWithUsersRequest) (*protos.GetAllAccountsWithUsersResponse, error) {
	var (
		accounts []*protos.ClusterAccountInfo
	)
	// 记录日志
	logrus.Infof("Received request GetAllAccountsWithUsers: %v", in)
	request := &craneProtos.QueryEntityInfoRequest{
		Uid: 0,
	}
	response, err := utils.CraneCtld.QueryEntityInfo(context.Background(), request)
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
		responseUser, _ := utils.CraneCtld.QueryEntityInfo(context.Background(), requestUser)
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

func (s *ServerAccount) QueryAccountBlockStatus(ctx context.Context, in *protos.QueryAccountBlockStatusRequest) (*protos.QueryAccountBlockStatusResponse, error) {
	var (
		blocked bool
	)
	logrus.Infof("Received request QueryAccountBlockStatus: %v", in)
	// 请求体
	request := &craneProtos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: craneProtos.EntityType_Account,
		Name:       in.AccountName,
	}
	response, err := utils.CraneCtld.QueryEntityInfo(context.Background(), request)
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
