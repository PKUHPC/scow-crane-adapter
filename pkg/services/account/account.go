package account

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/utils"
)

type ServerAccount struct {
	protos.UnimplementedAccountServiceServer
	muBlock   sync.Mutex
	muUnBlock sync.Mutex
}

func (s *ServerAccount) ListAccounts(ctx context.Context, in *protos.ListAccountsRequest) (*protos.ListAccountsResponse, error) {
	accountList, err := utils.GetAccountByUser(in.UserId)
	if err != nil {
		logrus.Errorf("ListAccounts failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ListAccounts failed", err.Error())
	}

	logrus.Tracef("ListAccounts accounts: %v", accountList)
	return &protos.ListAccountsResponse{Accounts: accountList}, nil
}

func (s *ServerAccount) CreateAccount(ctx context.Context, in *protos.CreateAccountRequest) (*protos.CreateAccountResponse, error) {
	var (
		partitionList           []string
		allowedPartitionQosList []*craneProtos.UserInfo_AllowedPartitionQos
	)
	logrus.Infof("Received request CreateAccount: %v", in)

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("CreateAccount failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	// 获取计算分区信息
	for _, partition := range utils.CConfig.Partitions {
		partitionList = append(partitionList, partition.Name)
	}
	// 获取系统QOS
	qosList, err := utils.GetAllQos()
	if err != nil {
		logrus.Errorf("CreateAccount Error getting QoS: %v", err)
		return nil, utils.RichError(codes.Internal, "Error getting QoS", err.Error())
	}

	AccountInfo := &craneProtos.AccountInfo{
		Name:              in.AccountName,
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
	response, err := utils.CraneCtld.AddAccount(context.Background(), request)
	if err != nil {
		logrus.Errorf("CreateAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("CreateAccount err: %v", fmt.Errorf("CRANE_INTERNAL_ERROR"))
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", strconv.FormatInt(int64(response.GetCode()), 10))
	}
	logrus.Tracef("create account: %v success", in.AccountName)
	// 账户创建成功后，将用户添加至账户中
	for _, partition := range utils.CConfig.Partitions {
		allowedPartitionQosList = append(allowedPartitionQosList, &craneProtos.UserInfo_AllowedPartitionQos{
			PartitionName: partition.Name,
			QosList:       qosList,
			DefaultQos:    qosList[0],
		})
	}
	uid, err := utils.GetUidByUserName(in.OwnerUserId)
	if err != nil {
		logrus.Errorf("CreateAccount err: %v", err)
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
		logrus.Errorf("CreateAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !responseUser.GetOk() {
		logrus.Errorf("CreateAccount err: %v", fmt.Errorf("ACCOUNT_NOT_FOUND"))
		return nil, utils.RichError(codes.NotFound, "ACCOUNT_NOT_FOUND", strconv.FormatInt(int64(responseUser.GetCode()), 10))
	}
	logrus.Tracef("add user : %v to account: %v success", in.OwnerUserId, in.AccountName)

	logrus.Infof("CreateAccount create account: %v success", in.AccountName)
	return &protos.CreateAccountResponse{}, nil
}

func (s *ServerAccount) BlockAccount(ctx context.Context, in *protos.BlockAccountRequest) (*protos.BlockAccountResponse, error) {
	logrus.Infof("Received request BlockAccount: %v", in)
	s.muBlock.Lock()
	defer s.muBlock.Unlock()

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("BlockAccount failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	// 请求体 封锁账户
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      true,
		EntityType: craneProtos.EntityType_Account,
		EntityList: []string{in.AccountName},
		Uid:        0,
	}
	response, err := utils.CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("BlockAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("BlockAccount err: %v", fmt.Errorf("ACCOUNT_ALREADY_EXISTS"))
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.RichErrorList[0].GetDescription())
	} else {
		logrus.Infof("BlockAccount account: %v success", in.AccountName)
		return &protos.BlockAccountResponse{}, nil
	}
}

func (s *ServerAccount) UnblockAccount(ctx context.Context, in *protos.UnblockAccountRequest) (*protos.UnblockAccountResponse, error) {
	logrus.Infof("Received request UnblockAccount: %v", in)
	s.muUnBlock.Lock() // 加锁操作
	defer s.muUnBlock.Unlock()

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("UnblockAccount failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	//  解封账户请求体
	request := &craneProtos.BlockAccountOrUserRequest{
		Block:      false,
		EntityType: craneProtos.EntityType_Account,
		EntityList: []string{in.AccountName},
		Uid:        0,
	}
	response, err := utils.CraneCtld.BlockAccountOrUser(context.Background(), request)
	if err != nil {
		logrus.Errorf("UnblockAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("UnblockAccount err: %v", fmt.Errorf("ACCOUNT_ALREADY_EXISTS"))
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", response.RichErrorList[0].GetDescription())
	} else {
		logrus.Infof("UnblockAccount account: %v success", in.AccountName)
		return &protos.UnblockAccountResponse{}, nil
	}
}

func (s *ServerAccount) GetAllAccountsWithUsers(ctx context.Context, in *protos.GetAllAccountsWithUsersRequest) (*protos.GetAllAccountsWithUsersResponse, error) {
	var accounts []*protos.ClusterAccountInfo

	logrus.Infof("Received request GetAllAccountsWithUsers: %v", in)
	allAccount, err := utils.GetAllAccount()
	if err != nil {
		logrus.Errorf("GetAllAccountsWithUsers err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}
	// 获取所有账户信息
	for _, account := range allAccount {
		var userInfo []*protos.ClusterAccountInfo_UserInAccount
		requestUser := &craneProtos.QueryUserInfoRequest{
			Uid:     0,
			Account: account.GetName(),
		}
		// 获取单个账户下用户信息
		responseUser, _ := utils.CraneCtld.QueryUserInfo(context.Background(), requestUser)
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
	logrus.Tracef("GetAllAccountsWithUsers Accounts: %v", accounts)
	return &protos.GetAllAccountsWithUsersResponse{Accounts: accounts}, nil
}

func (s *ServerAccount) QueryAccountBlockStatus(ctx context.Context, in *protos.QueryAccountBlockStatusRequest) (*protos.QueryAccountBlockStatusResponse, error) {
	var accountStatusInPartition []*protos.AccountStatusInPartition
	logrus.Infof("Received request QueryAccountBlockStatus: %v", in)

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("QueryAccountBlockStatus failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	// 查询账户
	account, err := utils.GetAccountByName(in.AccountName)
	if err != nil {
		logrus.Errorf("QueryAccountBlockStatus err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}

	// 获取所有分区
	partitions := utils.GetAllPartitions()

	// 获取账户的blocked
	accountBlocked := account.GetBlocked()
	// 该账户在所有分区中的封锁
	if accountBlocked {
		for _, partition := range partitions {
			accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
				Blocked:   accountBlocked,
				Partition: partition,
			})
		}
		logrus.Tracef("QueryAccountBlockStatus Account Blocked Details: %v", accountStatusInPartition)
		return &protos.QueryAccountBlockStatusResponse{Blocked: accountBlocked, AccountBlockedDetails: accountStatusInPartition}, nil
	}

	// 获取账户的allowPartitions
	allowPartitions := account.GetAllowedPartitions()
	for _, partition := range partitions {
		if utils.Contains(allowPartitions, partition) {
			accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
				Blocked:   false,
				Partition: partition,
			})
			continue
		}
		accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
			Blocked:   true,
			Partition: partition,
		})
	}

	logrus.Tracef("GetAllAccountsWithUsers Accounts: %v", accountStatusInPartition)
	return &protos.QueryAccountBlockStatusResponse{Blocked: false, AccountBlockedDetails: accountStatusInPartition}, nil
}

func (s *ServerAccount) DeleteAccount(ctx context.Context, in *protos.DeleteAccountRequest) (*protos.DeleteAccountResponse, error) {
	logrus.Infof("Received request DeleteAccount: %v", in)

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("DeleteAccount failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	jobInfo, err := utils.GetTaskByAccountName([]string{in.AccountName})
	if err != nil {
		logrus.Errorf("DeleteAccount err: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_RUNCOMMAND_ERROR", err.Error())
	}
	logrus.Tracef("DeleteAccount job info: %v", jobInfo)

	runningJobs := len(jobInfo)
	if runningJobs != 0 {
		logrus.Errorf("DeleteAccount failed: %v", fmt.Errorf("exist running jobs"))
		return nil, err
	}

	// 创建删除账户请求体
	deleteAccountRequest := &craneProtos.DeleteAccountRequest{
		Uid:         uint32(os.Getuid()),
		AccountList: []string{in.AccountName},
	}
	response, err := utils.CraneCtld.DeleteAccount(context.Background(), deleteAccountRequest)
	if err != nil {
		logrus.Errorf("DeleteAccount err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("DeleteAccount failed: %v", fmt.Errorf("ASSOCIATION_NOT_EXISTS"))
		return nil, utils.RichError(codes.NotFound, "ASSOCIATION_NOT_EXISTS", response.RichErrorList[0].GetDescription())
	}
	logrus.Infof("DeleteAccount: %v success", in.AccountName)
	return &protos.DeleteAccountResponse{}, nil
}

// BlockAccountWithPartitions Crane账户封锁将根据分区来细粒度封锁
func (s *ServerAccount) BlockAccountWithPartitions(ctx context.Context, in *protos.BlockAccountWithPartitionsRequest) (*protos.BlockAccountWithPartitionsResponse, error) {
	logrus.Infof("Received request BlockAccountWithPartitions: %v", in)
	s.muBlock.Lock()
	defer s.muBlock.Unlock()

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("BlockAccountWithPartitions failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	// 查询账户
	account, err := utils.GetAccountByName(in.AccountName)
	if err != nil {
		logrus.Errorf("BlockAccountWithPartitions err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}

	// 获取账户的allowPartitions
	allowPartitions := account.GetAllowedPartitions()

	var needBlockPartitions []string
	// 请求的分区在账户的allowPartitions内需要进行封锁，若不在allowPartitions内，表示账户在该分区本来就是封锁状态，无需进行封锁了
	for _, partition := range in.BlockedPartitions {
		if utils.Contains(allowPartitions, partition) {
			needBlockPartitions = append(needBlockPartitions, partition)
		}
	}

	if len(needBlockPartitions) == 0 {
		logrus.Infof("BlockAccountWithPartitions：%v no partition need block", in.AccountName)
		return &protos.BlockAccountWithPartitionsResponse{}, nil
	}

	// 封锁账户请求体
	request := &craneProtos.ModifyAccountRequest{
		ModifyField: craneProtos.ModifyField_Partition,
		ValueList:   needBlockPartitions,
		Name:        in.AccountName,
		Type:        craneProtos.OperationType_Delete,
		Uid:         0,
	}

	response, err := utils.CraneCtld.ModifyAccount(context.Background(), request)
	if err != nil {
		logrus.Errorf("BlockAccountWithPartitions err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		var message string
		for _, richError := range response.GetRichErrorList() {
			message += richError.GetDescription() + "\n"
		}
		logrus.Errorf("BlockAccountWithPartitions failed: %v", message)
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", message)
	}

	logrus.Infof("BlockAccountWithPartitions account: %v success", in.AccountName)
	return &protos.BlockAccountWithPartitionsResponse{}, nil
}

func (s *ServerAccount) UnblockAccountWithPartitions(ctx context.Context, in *protos.UnblockAccountWithPartitionsRequest) (*protos.UnblockAccountWithPartitionsResponse, error) {
	logrus.Infof("Received request UnblockAccountWithPartitions: %v", in)
	s.muUnBlock.Lock() // 加锁操作
	defer s.muUnBlock.Unlock()

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("UnblockAccountWithPartitions failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	// 查询账户
	account, err := utils.GetAccountByName(in.AccountName)
	if err != nil {
		logrus.Errorf("UnblockAccountWithPartitions err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}

	// 获取账户的allowPartitions
	allowPartitions := account.GetAllowedPartitions()

	var needUnblockPartitions []string
	// 请求的分区不在账户的allowPartitions内，表示账户在该分区是封锁状态，需进行解封
	for _, partition := range in.UnblockedPartitions {
		if !utils.Contains(allowPartitions, partition) {
			needUnblockPartitions = append(needUnblockPartitions, partition)
		}
	}

	if len(needUnblockPartitions) == 0 {
		logrus.Infof("UnblockAccountWithPartitions：%v no partition need unblock", in.AccountName)
		return &protos.UnblockAccountWithPartitionsResponse{}, nil
	}

	// 封锁账户请求体
	request := &craneProtos.ModifyAccountRequest{
		ModifyField: craneProtos.ModifyField_Partition,
		ValueList:   needUnblockPartitions,
		Name:        in.AccountName,
		Type:        craneProtos.OperationType_Add,
		Uid:         0,
	}

	response, err := utils.CraneCtld.ModifyAccount(context.Background(), request)
	if err != nil {
		logrus.Errorf("UnblockAccountWithPartitions err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		var message string
		for _, richError := range response.GetRichErrorList() {
			message += richError.GetDescription() + "\n"
		}
		logrus.Errorf("UnblockAccountWithPartitions failed: %v", message)
		return nil, utils.RichError(codes.AlreadyExists, "ACCOUNT_ALREADY_EXISTS", message)
	}

	logrus.Infof("UnblockAccountWithPartitions account: %v success", in.AccountName)
	return &protos.UnblockAccountWithPartitionsResponse{}, nil
}

func (s *ServerAccount) QueryAccountBlockStatusWithPartitions(ctx context.Context, in *protos.QueryAccountBlockStatusWithPartitionsRequest) (*protos.QueryAccountBlockStatusWithPartitionsResponse, error) {
	var (
		accountStatusInPartition []*protos.AccountStatusInPartition
		queriedPartitions        []string
	)
	logrus.Infof("Received request QueryAccountBlockStatus: %v", in)

	// 检查账户名
	if err := utils.CheckAccount(in.AccountName); err != nil {
		logrus.Errorf("QueryAccountBlockStatusWithPartitions failed: %v", err)
		return nil, utils.RichError(codes.Internal, "ACCOUNT_ILLEGAL", err.Error())
	}

	// 查询账户
	account, err := utils.GetAccountByName(in.AccountName)
	if err != nil {
		logrus.Errorf("QueryAccountBlockStatusWithPartitions err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}

	// 获取计算分区信息
	if len(in.QueriedPartitions) == 0 {
		queriedPartitions = utils.GetAllPartitions()
	} else {
		queriedPartitions = in.QueriedPartitions
	}

	// 获取账户的blocked
	accountBlocked := account.GetBlocked()
	// 该账户在所有分区中封锁
	if accountBlocked {
		for _, partition := range queriedPartitions {
			accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
				Blocked:   accountBlocked,
				Partition: partition,
			})
		}
		logrus.Tracef("QueryAccountBlockStatusWithPartitions Account Blocked Details: %v", accountStatusInPartition)
		return &protos.QueryAccountBlockStatusWithPartitionsResponse{Blocked: accountBlocked, AccountBlockedDetails: accountStatusInPartition}, nil
	}

	// 获取账户的allowPartitions
	allowPartitions := account.GetAllowedPartitions()
	for _, partition := range queriedPartitions {
		if utils.Contains(allowPartitions, partition) {
			accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
				Blocked:   false,
				Partition: partition,
			})
			continue
		}
		accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
			Blocked:   true,
			Partition: partition,
		})
	}
	logrus.Tracef("QueryAccountBlockStatusWithPartitions Account Blocked Details: %v", accountStatusInPartition)
	return &protos.QueryAccountBlockStatusWithPartitionsResponse{Blocked: false, AccountBlockedDetails: accountStatusInPartition}, nil
}

func (s *ServerAccount) GetAllAccountsWithUsersAndBlockedDetails(ctx context.Context, in *protos.GetAllAccountsWithUsersAndBlockedDetailsRequest) (*protos.GetAllAccountsWithUsersAndBlockedDetailsResponse, error) {
	logrus.Infof("Received request GetAllAccountsWithUsersAndBlockedDetails: %v", in)

	var acctInfo []*protos.ClusterAccountInfoWithBlockedDetails
	// 1. 获取所有账户
	allAccount, err := utils.GetAllAccount()
	if err != nil {
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}
	// 2. 获取所有账户的用户信息
	accountUserInfoMap, err := utils.GetAllAccountUserInfoMap(allAccount)
	if err != nil {
		logrus.Errorf("GetAllAccountsWithUsersAndBlockedDetails err: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_INTERNAL_ERROR", err.Error())
	}

	// 3. 获取所有分区
	partitions := utils.GetAllPartitions()

	// 4. 获取和每个账户关联的用户的信息以及用户的block状态
	for account, users := range accountUserInfoMap {
		var (
			userInfo                 []*protos.ClusterAccountInfoWithBlockedDetails_UserInAccount
			accountStatusInPartition []*protos.AccountStatusInPartition
		)

		for _, user := range users {
			userInfo = append(userInfo, &protos.ClusterAccountInfoWithBlockedDetails_UserInAccount{
				UserId:   strconv.Itoa(int(user.GetUid())),
				UserName: user.GetName(),
				Blocked:  user.GetBlocked(),
			})

		}

		// 获取账户的blocked
		accountBlocked := account.GetBlocked()
		// 该账户在所有分区中的封锁
		if accountBlocked {
			for _, partition := range partitions {
				accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
					Blocked:   accountBlocked,
					Partition: partition,
				})
			}
			acctInfo = append(acctInfo, &protos.ClusterAccountInfoWithBlockedDetails{
				AccountName:           account.GetName(),
				Users:                 userInfo,
				Blocked:               true,
				AccountBlockedDetails: accountStatusInPartition,
			})
			continue
		}

		// 获取账户的allowPartitions
		allowPartitions := account.GetAllowedPartitions()
		for _, partition := range partitions {
			if utils.Contains(allowPartitions, partition) {
				accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
					Blocked:   false,
					Partition: partition,
				})
				continue
			}
			accountStatusInPartition = append(accountStatusInPartition, &protos.AccountStatusInPartition{
				Blocked:   true,
				Partition: partition,
			})
		}

		acctInfo = append(acctInfo, &protos.ClusterAccountInfoWithBlockedDetails{
			AccountName:           account.GetName(),
			Users:                 userInfo,
			Blocked:               false,
			AccountBlockedDetails: accountStatusInPartition,
		})
	}

	logrus.Tracef("GetAllAccountsWithUsersAndBlockedDetails response: %v", acctInfo)
	return &protos.GetAllAccountsWithUsersAndBlockedDetailsResponse{Accounts: acctInfo}, nil
}
