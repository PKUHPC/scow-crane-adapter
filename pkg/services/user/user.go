package user

import (
	"context"
	"fmt"
	"strconv"

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
	var allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
	logrus.Infof("Received request AddUserToAccount: %v", in)

	// 获取crane中QOS列表
	qosList, err := utils.GetAllQos()
	if err != nil {
		logrus.Errorf("AddUserToAccount Error getting QoS: %v", err)
		return nil, utils.RichError(codes.Internal, "Error getting QoS", err.Error())
	}

	// 获取计算分区 配置qos
	for _, partition := range utils.CConfig.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &craneProtos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosList,
			DefaultQos:    qosList[0],
		})
	}
	uid, err := utils.GetUidByUserName(in.UserId)
	if err != nil {
		logrus.Errorf("AddUserToAccount err: %v", err)
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
		logrus.Errorf("AddUserToAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("AddUserToAccount err: %v", fmt.Errorf("ACCOUNT_NOT_FOUND"))
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", strconv.FormatInt(int64(response.GetReason()), 10))
	}
	logrus.Infof("AddUserToAccount success! user: %v, account: %v", in.UserId, in.AccountName)
	return &protos.AddUserToAccountResponse{}, nil
}

func (s *ServerUser) RemoveUserFromAccount(ctx context.Context, in *protos.RemoveUserFromAccountRequest) (*protos.RemoveUserFromAccountResponse, error) {
	logrus.Infof("Received request RemoveUserFromAccount: %v", in)
	request := &craneProtos.DeleteUserRequest{
		Uid:     0,
		Account: in.AccountName,
		Name:    in.UserId,
	}

	response, err := utils.CraneCtld.DeleteUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("RemoveUserFromAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("RemoveUserFromAccount err: %v", fmt.Errorf("ASSOCIATION_NOT_EXISTS"))
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", strconv.FormatInt(int64(response.GetReason()), 10))
	}
	logrus.Infof("RemoveUserFromAccount success! user: %v, account: %v", in.UserId, in.AccountName)
	return &protos.RemoveUserFromAccountResponse{}, nil
}

func (s *ServerUser) BlockUserInAccount(ctx context.Context, in *protos.BlockUserInAccountRequest) (*protos.BlockUserInAccountResponse, error) {
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
		logrus.Errorf("BlockUserInAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("BlockUserInAccount err: %v", fmt.Errorf("ASSOCIATION_NOT_EXISTS"))
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", strconv.FormatInt(int64(response.GetReason()), 10))
	}
	logrus.Infof("BlockUserInAccount success! user: %v, account: %v", in.UserId, in.AccountName)
	return &protos.BlockUserInAccountResponse{}, nil
}

func (s *ServerUser) UnblockUserInAccount(ctx context.Context, in *protos.UnblockUserInAccountRequest) (*protos.UnblockUserInAccountResponse, error) {
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
		logrus.Errorf("UnblockUserInAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("UnblockUserInAccount err: %v", fmt.Errorf("ASSOCIATION_NOT_EXISTS"))
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", strconv.FormatInt(int64(response.GetReason()), 10))
	}
	logrus.Infof("UnblockUserInAccount success! user: %v, account: %v", in.UserId, in.AccountName)
	return &protos.UnblockUserInAccountResponse{}, nil
}

func (s *ServerUser) QueryUserInAccountBlockStatus(ctx context.Context, in *protos.QueryUserInAccountBlockStatusRequest) (*protos.QueryUserInAccountBlockStatusResponse, error) {
	logrus.Infof("Received request QueryUserInAccountBlockStatus: %v", in)
	request := &craneProtos.QueryUserInfoRequest{
		Uid:     0,
		Name:    in.UserId,
		Account: in.AccountName,
	}
	response, err := utils.CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		logrus.Errorf("QueryUserInAccountBlockStatus err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("QueryUserInAccountBlockStatus err: %v", fmt.Errorf("CRANE_INTERNAL_ERROR"))
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", strconv.FormatInt(int64(response.GetReason()), 10))
	}

	blocked := response.GetUserList()[0].GetBlocked()
	logrus.Tracef("QueryUserInAccountBlockStatus Blocked: %v", blocked)
	return &protos.QueryUserInAccountBlockStatusResponse{Blocked: blocked}, nil
}
