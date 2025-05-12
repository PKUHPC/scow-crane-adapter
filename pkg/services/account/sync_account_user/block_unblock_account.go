package sync_account_user

import (
	"fmt"
	"github.com/sirupsen/logrus"
	pb "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/utils"
)

// 同步账户的封锁情况
func syncAccountBlockStatus(syncData *pb.SyncAccountInfo) *pb.SyncAccountUserInfoResponse_SyncOperationResult {
	var result *pb.SyncAccountUserInfoResponse_SyncOperationResult

	// 同步账户的封锁
	if syncData.BlockedInCluster {
		result = BlockAccount(syncData)
	} else {
		// 同步账户的解封
		result = UnBlockAccount(syncData)
	}

	return result
}

func BlockAccount(syncData *pb.SyncAccountInfo) *pb.SyncAccountUserInfoResponse_SyncOperationResult {
	if syncData.WhitelistId != nil {
		message := fmt.Sprintf("The account is in the whitelist and does not need to be blocked")
		logrus.Infof("[SyncAccountUser], %v", message)
		return BlockAccountFailedOperation(syncData.AccountName, message)
	}

	if err := utils.BlockAccount(syncData.AccountName); err != nil {
		message := fmt.Sprintf("block account: %v failed", syncData.AccountName)
		logrus.Errorf("[SyncAccountUser], %v", message)
		return BlockAccountFailedOperation(syncData.AccountName, message)
	}

	message := fmt.Sprintf("block account: %v success", syncData.AccountName)
	logrus.Infof("[SyncAccountUser], %v", message)
	return BlockAccountSuccessOperation(syncData.AccountName)
}

func UnBlockAccount(syncData *pb.SyncAccountInfo) *pb.SyncAccountUserInfoResponse_SyncOperationResult {
	var message string

	// 获取unblockedPartitions分区，该分区需要解封, blockPartitions分区，该分区需要封锁
	unblockPartition, blockPartitions := getBlockAndUnblockPartition(syncData)

	// 解封分区 需要封锁的分区不在unblockedPartitions中，自然就封锁了
	if err := utils.UnblockAccountInPartitions(syncData.AccountName, unblockPartition); err != nil {
		message = fmt.Sprintf("unblock account %v in partitions %v failed: %v", syncData.AccountName, unblockPartition, err)
		logrus.Errorf("[SyncAccountUser] %v", message)
		return UnblockAccountFailedOperation(syncData.AccountName, message)
	}

	// 需要封锁的分区
	if err := utils.BlockAccountInPartitions(syncData.AccountName, blockPartitions); err != nil {
		message = fmt.Sprintf("block account %v in partitions %v failed: %v", syncData.AccountName, blockPartitions, err)
		logrus.Errorf("[SyncAccountUser] %v", message)
		return UnblockAccountFailedOperation(syncData.AccountName, message)
	}

	message = fmt.Sprintf("unblock account: %v success", syncData.AccountName)
	logrus.Infof("[SyncAccountUser], %v", message)
	return UnblockAccountSuccessOperation(syncData.AccountName)
}

func getBlockAndUnblockPartition(syncData *pb.SyncAccountInfo) ([]string, []string) {
	var (
		blockPartitions   []string
		unblockPartitions []string
	)

	partitions := utils.GetAllPartitions()
	if syncData.GetUseAllPartitions() {
		unblockPartitions = partitions
		blockPartitions = []string{}
	} else {
		unblockPartitions = syncData.GetUnblockedPartitions().(*pb.SyncAccountInfo_AssignedPartitions_).AssignedPartitions.Partitions
		blockPartitions = utils.SliceSubtract(partitions, unblockPartitions)
	}

	return unblockPartitions, blockPartitions
}
