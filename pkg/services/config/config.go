package config

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

type ServerConfig struct {
	protos.UnimplementedConfigServiceServer
}

func (s *ServerConfig) GetAvailablePartitions(ctx context.Context, in *protos.GetAvailablePartitionsRequest) (*protos.GetAvailablePartitionsResponse, error) {
	logrus.Infof("Received request GetAvailablePartitions: %v", in)
	var partitions []*protos.Partition

	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")
	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	for _, part := range utils.CConfig.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		partitionName := part.Name
		request := &craneProtos.QueryPartitionInfoRequest{
			PartitionName: partitionName,
		}
		response, err := utils.CraneCtld.QueryPartitionInfo(context.Background(), request)
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
		}
		partitionValue := response.GetPartitionInfo()[0]
		logrus.Infof("%v", response.GetPartitionInfo())
		// MemMb: partitionValue.GetTotalMem() / (1024 * 1024),
		partitions = append(partitions, &protos.Partition{
			Name:  partitionValue.GetName(),
			MemMb: partitionValue.GetResTotal().GetAllocatableRes().MemoryLimitBytes / (1024 * 1024),
			// Cores: uint32(partitionValue.GetTotalCpus()),
			Cores: uint32(partitionValue.GetResTotal().GetAllocatableRes().CpuCoreLimit),
			Nodes: partitionValue.GetTotalNodes(),
			Qos:   qosListValue,
		})
	}
	logrus.Infof("%v", partitions)
	return &protos.GetAvailablePartitionsResponse{Partitions: partitions}, nil
}

func (s *ServerConfig) GetClusterInfo(ctx context.Context, in *protos.GetClusterInfoRequest) (*protos.GetClusterInfoResponse, error) {
	var (
		partitions []*protos.PartitionInfo
	)
	logrus.Infof("Received request GetClusterInfo: %v", in)
	for _, part := range utils.CConfig.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		var runningNodes uint32
		var state protos.PartitionInfo_PartitionStatus
		partitionName := part.Name // 获取分区名
		// 请求体
		request := &craneProtos.QueryPartitionInfoRequest{
			PartitionName: partitionName,
		}

		response, err := utils.CraneCtld.QueryPartitionInfo(context.Background(), request)
		if err != nil {
			logrus.Errorf("Received request GetClusterInfo: %v", err)
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
		logrus.Infof("%v", response.GetPartitionInfo())
		resultRatio := float64(runningNodes) / float64(partitionValue.TotalNodes)
		percentage := int(resultRatio * 100) // 保留整数
		if partitionValue.State == craneProtos.PartitionState_PARTITION_UP {
			state = protos.PartitionInfo_AVAILABLE
		} else {
			state = protos.PartitionInfo_NOT_AVAILABLE
		}
		TotalCpu := partitionValue.GetResTotal().GetAllocatableRes().CpuCoreLimit
		AllocCpu := partitionValue.GetResAlloc().GetAllocatableRes().CpuCoreLimit
		AvailCpu := partitionValue.GetResAvail().GetAllocatableRes().CpuCoreLimit
		IdleCpu := TotalCpu - AllocCpu
		NotAvailableCpu := TotalCpu - AvailCpu - AllocCpu
		partitions = append(partitions, &protos.PartitionInfo{
			PartitionName:         partitionValue.GetName(),
			NodeCount:             partitionValue.TotalNodes,
			RunningNodeCount:      runningNodes,
			IdleNodeCount:         partitionValue.AliveNodes - runningNodes, // 减去正在运行的节点 partitionValue.TotalNodes - runningNodes,
			NotAvailableNodeCount: partitionValue.TotalNodes - partitionValue.AliveNodes,
			CpuCoreCount:          uint32(TotalCpu),
			RunningCpuCount:       uint32(AllocCpu),
			IdleCpuCount:          uint32(IdleCpu),
			NotAvailableCpuCount:  uint32(NotAvailableCpu),
			JobCount:              uint32(runningJobNum) + uint32(pendingJobNum),
			RunningJobCount:       uint32(runningJobNum),
			PendingJobCount:       uint32(pendingJobNum),
			UsageRatePercentage:   uint32(percentage),
			PartitionStatus:       state,
		})

	}
	return &protos.GetClusterInfoResponse{ClusterName: utils.CConfig.ClusterName, Partitions: partitions}, nil
}

func (s *ServerConfig) GetClusterConfig(ctx context.Context, in *protos.GetClusterConfigRequest) (*protos.GetClusterConfigResponse, error) {
	var partitions []*protos.Partition
	logrus.Infof("Received request GetClusterConfig: %v", in)

	// 获取系统Qos
	qosList, _ := utils.GetQos()
	qosListValue := utils.RemoveValue(qosList, "UNLIMITED")
	if len(qosListValue) == 0 {
		return nil, utils.RichError(codes.NotFound, "QOS_NOT_FOUND", "The qos not exists.")
	}

	for _, part := range utils.CConfig.Partitions { // 遍历每个计算分区、分别获取信息  分区从接口获取
		partitionName := part.Name
		request := &craneProtos.QueryPartitionInfoRequest{
			PartitionName: partitionName,
		}
		response, err := utils.CraneCtld.QueryPartitionInfo(context.Background(), request)
		if err != nil {
			return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
		}
		partitionValue := response.GetPartitionInfo()[0]
		logrus.Infof("%v", response.GetPartitionInfo())
		partitions = append(partitions, &protos.Partition{
			Name:  partitionValue.GetName(),
			MemMb: partitionValue.GetResTotal().GetAllocatableRes().MemorySwLimitBytes / (1024 * 1024),
			// Cores: uint32(partitionValue.GetTotalCpus()),
			Cores: uint32(partitionValue.GetResTotal().GetAllocatableRes().CpuCoreLimit),
			Nodes: partitionValue.GetTotalNodes(),
			Qos:   qosListValue, // QOS是强行加进去的
		})
	}
	logrus.Infof("%v", partitions)
	return &protos.GetClusterConfigResponse{Partitions: partitions, SchedulerName: "Crane"}, nil
}
