package user

import (
	"context"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/utils"
)

type ServerUser struct {
	protos.UnimplementedUserServiceServer
}

func (s *ServerUser) AddUserToAccount(ctx context.Context, in *protos.AddUserToAccountRequest) (*protos.AddUserToAccountResponse, error) {
	var (
		allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
	)
	logrus.Infof("Received request AddUserToAccount: %v", in)

	// 获取crane中QOS列表
	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")

	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	// 获取计算分区 配置qos
	for _, partition := range utils.CConfig.Partitions {
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
	response, err := utils.CraneCtld.AddUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", response.GetReason())
	}
	return &protos.AddUserToAccountResponse{}, nil
}

func (s *ServerUser) RemoveUserFromAccount(ctx context.Context, in *protos.RemoveUserFromAccountRequest) (*protos.RemoveUserFromAccountResponse, error) {
	// 记录日志
	logrus.Infof("Received request RemoveUserFromAccount: %v", in)
	request := &craneProtos.DeleteEntityRequest{
		Uid:        0, // 操作者
		EntityType: craneProtos.EntityType_User,
		Account:    in.AccountName,
		Name:       in.UserId,
	}

	response, err := utils.CraneCtld.DeleteEntity(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.RemoveUserFromAccountResponse{}, nil
}

func (s *ServerUser) BlockUserInAccount(ctx context.Context, in *protos.BlockUserInAccountRequest) (*protos.BlockUserInAccountResponse, error) {
	// 记录日志
	logrus.Infof("Received request BlockUserInAccount: %v", in)
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		Uid:        0, // 操作者
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := utils.CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.BlockUserInAccountResponse{}, nil
}

func (s *ServerUser) UnblockUserInAccount(ctx context.Context, in *protos.UnblockUserInAccountRequest) (*protos.UnblockUserInAccountResponse, error) {
	// 记录日志
	logrus.Infof("Received request UnblockUserInAccount: %v", in)
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := utils.CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.GetReason())
	}
	return &protos.UnblockUserInAccountResponse{}, nil
}

func (s *ServerUser) QueryUserInAccountBlockStatus(ctx context.Context, in *protos.QueryUserInAccountBlockStatusRequest) (*protos.QueryUserInAccountBlockStatusResponse, error) {
	var (
		blocked bool
	)
	// 记录日志
	logrus.Infof("Received request QueryUserInAccountBlockStatus: %v", in)
	request := &craneProtos.QueryEntityInfoRequest{
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		Name:       in.UserId,
		Account:    in.AccountName,
	}
	response, err := utils.CraneCtld.QueryEntityInfo(context.Background(), request)
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
