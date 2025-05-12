package sync_account_user

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"scow-crane-adapter/pkg/utils"

	"google.golang.org/grpc/codes"

	pb "scow-crane-adapter/gen/go"
)

func createAccount(syncData *pb.SyncAccountInfo) (*pb.SyncAccountUserInfoResponse_SyncOperationResult, error) {
	var result *pb.SyncAccountUserInfoResponse_SyncOperationResult
	accountName := syncData.AccountName
	// 如果账户为空，直接返回
	if accountName == "" {
		message := fmt.Sprintf("account %v is nil", accountName)
		logrus.Errorf("[SyncAccountUser] %v", message)
		return CreateAccountFailedOperation(accountName, message), fmt.Errorf("account %v is nil", accountName)
	}

	// 检查账号名是否在slurm中
	account, err := utils.GetAccountByName(accountName)
	if err != nil {
		logrus.Errorf("CreateAccount failed: %v", err)
		return nil, utils.RichError(codes.Internal, "SQL_QUERY_FAILED", err.Error())
	}

	if account == nil {
		var partitionList []string
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

		if err = utils.CreateAccount(syncData.AccountName, partitionList, qosList); err != nil {
			message := fmt.Sprintf("create account %v failed: %v", syncData.AccountName, err)
			logrus.Errorf("[SyncAccountUser] %v", message)
			return CreateAccountFailedOperation(syncData.AccountName, message), err
		}
		message := fmt.Sprintf("create account %v success", syncData.AccountName)
		logrus.Infof("[SyncAccountUser] %v", message)
		result = CreateAccountSuccessOperation(syncData.AccountName)
	}

	return result, nil
}
