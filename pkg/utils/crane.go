package utils

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"

	craneProtos "scow-crane-adapter/gen/crane"
)

type jobCount struct {
	JobCount        uint32
	RunningJobCount uint32
	PendingJobCount uint32
}

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

func AddUserToAccount(accountName, userName string) error {
	var allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos

	// 获取系统QOS
	qosList, err := GetAllQos()
	if err != nil {
		logrus.Errorf("Error getting QoS: %v", err)
		return fmt.Errorf("error getting QoS: %v", err)
	}

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

// SelectAccountExists 查询账户的存在情况，并返回错误
func SelectAccountExists(account string) (bool, error) {
	request := &craneProtos.QueryAccountInfoRequest{
		Uid:         0,
		AccountList: []string{account},
	}
	response, err := CraneCtld.QueryAccountInfo(context.Background(), request)
	if err != nil {
		return false, fmt.Errorf("qury account %s failed: %v", account, err)
	}
	if !response.GetOk() {
		return false, nil
	}

	return true, nil
}

func CreateAccount(accountName string) error {
	var partitionList []string
	// 获取计算分区信息
	for _, partition := range CConfig.Partitions {
		partitionList = append(partitionList, partition.Name)
	}
	// 获取系统QOS
	qosList, err := GetAllQos()
	if err != nil {
		return err
	}

	AccountInfo := &craneProtos.AccountInfo{
		Name:              accountName,
		Description:       "Create account in crane.",
		AllowedPartitions: partitionList,
		DefaultQos:        qosList[0],
		AllowedQosList:    qosList,
	}
	// 创建账户请求体
	request := &craneProtos.AddAccountRequest{
		Uid:     uint32(os.Getuid()),
		Account: AccountInfo,
	}
	response, err := CraneCtld.AddAccount(context.Background(), request)
	if err != nil {
		logrus.Errorf("CreateAccount err: %v", err)
		return err
	}
	if !response.GetOk() {
		return fmt.Errorf("create account error: %v", strconv.FormatInt(int64(response.GetCode()), 10))
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

// SelectUserExists 查询用户的存在情况，并返回错误
func SelectUserExists(userName string) (bool, error) {
	request := &craneProtos.QueryUserInfoRequest{
		Uid:      0,
		UserList: []string{userName},
	}

	response, err := CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		logrus.Errorf("Failed to show the user %v, error: %v", userName, err)
		return false, err
	}
	if !response.GetOk() {
		return false, nil
	}
	return true, nil
}

func DeleteUserFromAccount(userId, accountName string) error {
	request := &craneProtos.DeleteUserRequest{
		Uid:      0,
		Account:  accountName,
		UserList: []string{userId},
	}

	response, err := CraneCtld.DeleteUser(context.Background(), request)
	if err != nil {
		return err
	}
	if !response.GetOk() {
		return fmt.Errorf("failed to delete user %v in account %v", userId, accountName)
	}
	return nil
}

func DeleteUser(userId string) error {
	request := &craneProtos.DeleteUserRequest{
		Uid:      0,
		UserList: []string{userId},
	}

	response, err := CraneCtld.DeleteUser(context.Background(), request)
	if err != nil {
		return err
	}
	if !response.GetOk() {
		return fmt.Errorf("the user has been deleted")
	}
	return nil
}

func BlockUserInAccount(userId, accountName string) error {
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		EntityList: []string{userId},
		Account:    accountName,
	}
	response, err := CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("BlockUserInAccount err: %v", err)
		return err
	}
	if !response.GetOk() {
		logrus.Errorf("BlockUserInAccount err: %v", fmt.Errorf("ASSOCIATION_NOT_EXISTS"))
		return fmt.Errorf("failed block user %v in account %v", userId, accountName)
	}
	return nil
}

func UnblockUserInAccount(userId, accountName string) error {
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		Uid:        0,
		EntityType: craneProtos.EntityType_User,
		EntityList: []string{userId},
		Account:    accountName,
	}
	response, err := CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("UnblockUserInAccount err: %v", err)
		return err
	}
	if !response.GetOk() {
		logrus.Errorf("UnblockUserInAccount err: %v", fmt.Errorf("ASSOCIATION_NOT_EXISTS"))
		return fmt.Errorf("failed unblock user %v in account %v", userId, accountName)
	}
	return nil
}

func HasUnfinishedJobsByUserName(userName string) (bool, error) {
	request := &craneProtos.QueryTasksInfoRequest{
		FilterUsers:                 []string{userName},
		OptionIncludeCompletedTasks: false,
	}
	response, err := CraneCtld.QueryTasksInfo(context.Background(), request)

	if err != nil {
		return false, err
	}
	if !response.GetOk() {
		return false, nil
	}

	if len(response.GetTaskInfoList()) != 0 {
		return true, nil
	}

	return false, nil
}

func GetAccountAssociatedUser(accountName string, excludeUserList []string) ([]string, error) {
	var userList []string

	request := &craneProtos.QueryUserInfoRequest{
		Uid:     0,
		Account: accountName,
	}
	response, err := CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		return nil, err
	}
	if !response.GetOk() {
		return nil, fmt.Errorf("the account %v not have user", accountName)
	}
	for _, ui := range response.GetUserList() {
		if Contains(excludeUserList, ui.Name) {
			continue
		}
		userList = append(userList, ui.Name)
	}

	return userList, nil
}

func GetAccountUserBlockedInfo(accountName string) (map[string]bool, error) {
	request := &craneProtos.QueryUserInfoRequest{
		Uid:     0,
		Account: accountName,
	}
	response, err := CraneCtld.QueryUserInfo(context.Background(), request)
	if err != nil {
		return nil, err
	}
	if !response.GetOk() {
		return nil, fmt.Errorf("account %v have not user", accountName)
	}
	blockedInfo := make(map[string]bool)
	for _, ui := range response.GetUserList() {
		blockedInfo[ui.Name] = ui.Blocked
	}
	return blockedInfo, nil
}

func GetJobsStatusDistribution() (map[string]*jobCount, error) {
	partitionJobs := make(map[string]*jobCount)
	for _, part := range CConfig.Partitions {
		// 获取正在运行作业的个数
		runningJob, err := GetTaskByPartitionAndStatus([]string{part.Name}, []craneProtos.TaskStatus{craneProtos.TaskStatus_Running})
		if err != nil {
			return nil, fmt.Errorf("get running task failed: %v", err)
		}
		runningJobNum := len(runningJob)

		// 获取正在排队作业的个数
		pendingJob, err := GetTaskByPartitionAndStatus([]string{part.Name}, []craneProtos.TaskStatus{craneProtos.TaskStatus_Pending})
		if err != nil {
			return nil, fmt.Errorf("get pending task failed: %v", err)
		}
		pendingJobNum := len(pendingJob)

		totalJobNum := runningJobNum + pendingJobNum
		partitionJobs[part.Name] = &jobCount{
			JobCount:        uint32(totalJobNum),
			RunningJobCount: uint32(runningJobNum),
			PendingJobCount: uint32(pendingJobNum),
		}
	}
	return partitionJobs, nil
}
