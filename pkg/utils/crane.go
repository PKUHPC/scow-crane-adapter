package utils

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"

	craneProtos "scow-crane-adapter/gen/crane"
)

func getUsersByAccountName(accountName string) ([]*craneProtos.UserInfo, error) {
	request := &craneProtos.QueryUserInfoRequest{
		Uid:     0,
		Account: accountName,
	}
	response, err := CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		logrus.Errorf("QueryUserInAccountBlockStatus err: %v", err)
		return nil, fmt.Errorf("query users failed: %v", err)
	}
	if !response.GetOk() {
		return nil, fmt.Errorf("query users failed: %v", response.GetRichErrorList()[0].GetDescription())
	}
	return response.UserList, nil
}

func AddUserToAccount(accountName, userName string, qosList []string) error {
	var allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
	// 账户创建成功后，将用户添加至账户中
	for _, partition := range CConfig.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &craneProtos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosList,
			DefaultQos:    qosList[0],
		})
	}
	uid, err := GetUidByUserName(userName)
	if err != nil {
		return fmt.Errorf("the user is not exists")
	}
	user := &craneProtos.UserInfo{
		Uid:                     uint32(uid),
		Name:                    userName,
		Account:                 accountName,
		Blocked:                 false,
		AllowedPartitionQosList: allowedPartitionQosList,
		AdminLevel:              craneProtos.UserInfo_None,
	}
	requestAddUser := &craneProtos.AddUserRequest{
		Uid:  0,
		User: user,
	}
	responseUser, err := CraneCtld.AddUser(context.Background(), requestAddUser)
	if err != nil {
		logrus.Errorf("CreateAccount err: %v", err)
		return err
	}
	if !responseUser.GetOk() {
		return fmt.Errorf("add user failed, code: %v ", strconv.FormatInt(int64(responseUser.GetCode()), 10))
	}
	return nil
}

func BlockAccount(accountName string) error {
	// 请求体 封锁账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		EntityType: craneProtos.EntityType_Account,
		EntityList: []string{accountName},
		Uid:        0,
	}
	response, err := CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("BlockAccount err: %v", err)
		return err
	}
	if !response.GetOk() {
		logrus.Errorf("BlockAccount err: %v", fmt.Errorf("ACCOUNT_ALREADY_EXISTS"))
		return err
	}

	return nil
}

func BlockAccountWithPartition(accountName string, partitions []string) error {
	// 封锁账户请求体
	request := &craneProtos.ModifyAccountRequest{
		ModifyField: craneProtos.ModifyField_Partition,
		ValueList:   partitions,
		Name:        accountName,
		Type:        craneProtos.OperationType_Delete,
		Uid:         0,
		Force:       true,
	}

	response, err := CraneCtld.ModifyAccount(context.Background(), request)
	if err != nil {
		logrus.Errorf("BlockAccountWithPartitions err: %v", err)
		return err
	}
	if !response.GetOk() {
		var message string
		for _, richError := range response.GetRichErrorList() {
			message += richError.GetDescription() + "\n"
		}
		logrus.Errorf("BlockAccountWithPartitions failed: %v", message)
		return fmt.Errorf("error: %v", message)
	}
	return nil
}

func UnblockAccount(accountName string) error {
	// 请求体 封锁账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		EntityType: craneProtos.EntityType_Account,
		EntityList: []string{accountName},
		Uid:        0,
	}
	response, err := CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("BlockAccount err: %v", err)
		return err
	}
	if !response.GetOk() {
		logrus.Errorf("BlockAccount err: %v", fmt.Errorf("ACCOUNT_ALREADY_EXISTS"))
		return err
	}

	return nil
}

func UnblockAccountWithPartition(accountName string, partitions []string) error {
	// 封锁账户请求体
	request := &craneProtos.ModifyAccountRequest{
		ModifyField: craneProtos.ModifyField_Partition,
		ValueList:   partitions,
		Name:        accountName,
		Type:        craneProtos.OperationType_Add,
		Uid:         0,
	}

	response, err := CraneCtld.ModifyAccount(context.Background(), request)
	if err != nil {
		logrus.Errorf("UnblockAccountWithPartitions err: %v", err)
		return err
	}
	if !response.GetOk() {
		var message string
		for _, richError := range response.GetRichErrorList() {
			message += richError.GetDescription() + "\n"
		}
		logrus.Errorf("UnblockAccountWithPartitions failed: %v", message)
		return fmt.Errorf("error: %v", message)
	}

	// 封锁的时候会将账户下面的用户的allow partition删掉，因此解封的时候需要加回来
	if err = modifyUserAllowedPartitions(accountName, partitions); err != nil {
		logrus.Errorf("UnblockAccountWithPartitions err: %v", err)
		return err
	}

	return nil
}

func modifyUserAllowedPartitions(accountName string, partitions []string) error {
	users, err := getUsersByAccountName(accountName)
	if err != nil {
		logrus.Errorf("BlockAccountWithPartitions err: %v", err)
	}

	for _, user := range users {
		logrus.Infof("modify account %v user %v partitions %v", accountName, user, partitions)
		request := &craneProtos.ModifyUserRequest{
			ModifyField: craneProtos.ModifyField_Partition,
			ValueList:   partitions,
			Name:        user.Name,
			Account:     accountName,
			Type:        craneProtos.OperationType_Overwrite,
			Uid:         0,
		}

		response, err := CraneCtld.ModifyUser(context.Background(), request)
		if err != nil {
			logrus.Errorf("modify user failed: %v", err)
			return err
		}
		if !response.GetOk() {
			var message string
			for _, richError := range response.GetRichErrorList() {
				message += richError.GetDescription() + "\n"
			}
			logrus.Errorf("modify user failed: %v", message)
			return fmt.Errorf("error: %v", message)
		}
	}

	logrus.Infof("modify account %v users partitions success", accountName)
	return nil
}
