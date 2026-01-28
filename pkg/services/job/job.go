package job

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/services/job/internal/builder"
	"scow-crane-adapter/pkg/utils"
)

const maxUint = 4294967295

type ServerJob struct {
	protos.UnimplementedJobServiceServer
	JM *utils.JobManager
}

func (s *ServerJob) CancelJob(ctx context.Context, in *protos.CancelJobRequest) (*protos.CancelJobResponse, error) {
	logrus.Infof("Received request CancelJob: %v", in)

	jobInfo, err := utils.GetJobById(in.JobId, in.UserId)
	if err != nil {
		message := fmt.Errorf("get job failed: %v", err)
		logrus.Errorf("[CancelJob] %v", message)
		return nil, utils.RichError(codes.Internal, "CRANE_FAILED", message.Error())
	}
	defer func() {
		err = utils.GlobalProxyManager.StopAndRemoveProxy(jobInfo.Name, jobInfo.ExecutionNode)
		if err != nil {
			logrus.Warnf("[CancelJob] delete job proxy file failed: %v", err)
		}
		err := s.JM.DeleteJobInfo(in.JobId)
		if err != nil {
			logrus.Warnf("[CancelJob] delete job info file failed: %v", err)
		}
	}()

	stepIds, err := utils.ParseStepIdList(strconv.Itoa(int(in.JobId)), ",")
	if err != nil {
		logrus.Errorf("[CancelJob] get job step ids failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", "Crane service call failed.")
	}
	request := &craneProtos.CancelTaskRequest{
		OperatorUid:    0,
		FilterIds:      stepIds,
		FilterUsername: in.UserId,
		FilterState:    craneProtos.TaskStatus_Invalid,
	}
	_, err = utils.CraneCtld.CancelTask(context.Background(), request)
	if err != nil {
		logrus.Errorf("[CancelJob] cancel job failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", "Crane service call failed.")
	}
	logrus.Infof("[CancelJob] cancel job: %v success", in.JobId)
	return &protos.CancelJobResponse{}, nil
}

func (s *ServerJob) QueryJobTimeLimit(ctx context.Context, in *protos.QueryJobTimeLimitRequest) (*protos.QueryJobTimeLimitResponse, error) {
	var seconds uint64

	logrus.Infof("Received request QueryJobTimeLimit: %v", in)
	filterIds := make(map[uint32]*craneProtos.JobStepIds)
	filterIds[in.JobId] = &craneProtos.JobStepIds{Steps: []uint32{1}}
	request := &craneProtos.QueryTasksInfoRequest{
		FilterIds:                   filterIds,
		OptionIncludeCompletedTasks: true, // 包含运行结束的作业
	}
	response, err := utils.CraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		logrus.Errorf("QueryJobTimeLimit failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	taskInfoList := response.GetTaskInfoList()
	if len(taskInfoList) == 0 {
		message := fmt.Sprintf("Task #%d was not found in crane.", in.JobId)
		logrus.Errorf("QueryJobTimeLimit failed: %v", message)
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", message)
	}
	if response.GetOk() {
		for _, taskInfo := range taskInfoList {
			timeLimit := taskInfo.GetTimeLimit()
			seconds = uint64(timeLimit.GetSeconds())
		}
		logrus.Tracef("QueryJobTimeLimit job: %v, TimeLimitMinutes: %v", in.JobId, seconds/60)
		return &protos.QueryJobTimeLimitResponse{TimeLimitMinutes: seconds / 60}, nil
	}
	return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Get job timelimit failed.")
}

func (s *ServerJob) ChangeJobTimeLimit(ctx context.Context, in *protos.ChangeJobTimeLimitRequest) (*protos.ChangeJobTimeLimitResponse, error) {
	var seconds uint64

	logrus.Infof("Received request ChangeJobTimeLimit: %v", in)
	// 查询请求体
	filterIds := make(map[uint32]*craneProtos.JobStepIds)
	filterIds[in.JobId] = &craneProtos.JobStepIds{Steps: []uint32{1}}
	requestLimitTime := &craneProtos.QueryTasksInfoRequest{
		FilterIds: filterIds,
	}

	responseLimitTime, err := utils.CraneCtld.QueryTasksInfo(context.Background(), requestLimitTime)
	if err != nil {
		logrus.Errorf("ChangeJobTimeLimit failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}

	taskInfoList := responseLimitTime.GetTaskInfoList()
	if len(taskInfoList) == 0 {
		message := fmt.Sprintf("Task #%d was not found in crane.", in.JobId)
		logrus.Errorf("ChangeJobTimeLimit failed: %v", message)
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", message)
	}
	if responseLimitTime.GetOk() {
		for _, taskInfo := range taskInfoList {
			timeLimit := taskInfo.GetTimeLimit()
			seconds = uint64(timeLimit.GetSeconds())
		}
	}

	// 如果小于0的话直接返回
	if in.DeltaMinutes*60+int64(seconds) <= 0 {
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", "Time limit should be greater than 0.")
	}
	// 修改时长限制的请求体
	request := &craneProtos.ModifyTaskRequest{
		TaskIds: []uint32{in.JobId},
		Value: &craneProtos.ModifyTaskRequest_TimeLimitSeconds{
			TimeLimitSeconds: in.DeltaMinutes*60 + int64(seconds),
		},
	}
	response, err := utils.CraneCtld.ModifyTask(context.Background(), request)
	if err != nil {
		logrus.Errorf("ChangeJobTimeLimit failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if len(response.GetNotModifiedTasks()) != 0 {
		logrus.Errorf("ChangeJobTimeLimit failed: %v", fmt.Errorf("JOB_NOT_FOUND"))
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", response.GetNotModifiedReasons()[0])
	}
	logrus.Tracef("ChangeJobTimeLimit success! job: %v, TimeLimitMinutes: %v", in.JobId, in.DeltaMinutes)
	return &protos.ChangeJobTimeLimitResponse{}, nil
}

func (s *ServerJob) GetJobById(ctx context.Context, in *protos.GetJobByIdRequest) (*protos.GetJobByIdResponse, error) {
	var (
		elapsedSeconds int64
		state          string
		reason         string
	)

	logrus.Infof("[GetJobById] Received request: %v", in)
	jobId := in.JobId
	taskInfo, err := utils.GetJobById(jobId, "")
	if err != nil {
		logrus.Errorf("[GetJobById] get job info err: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", err.Error())
	}

	if taskInfo.GetStatus() == craneProtos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - taskInfo.GetStartTime().Seconds
	} else if taskInfo.GetStatus() == craneProtos.TaskStatus_Pending {
		elapsedSeconds = 0
	}
	// 获取作业时长
	// elapsedSeconds = TaskInfoList.GetEndTime().Seconds - TaskInfoList.GetStartTime().Seconds
	if taskInfo.GetStatus() == craneProtos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - taskInfo.GetStartTime().Seconds
	} else if taskInfo.GetStatus() == craneProtos.TaskStatus_Pending {
		elapsedSeconds = 0
	} else {
		elapsedSeconds = taskInfo.GetEndTime().Seconds - taskInfo.GetStartTime().Seconds
	}

	// 获取cpu核分配数
	// cpusAlloc := TaskInfoList.GetAllocCpus()
	cpusAlloc := taskInfo.GetAllocatedResView().GetAllocatableRes().CpuCoreLimit
	cpusAllocInt32 := int32(cpusAlloc)
	// 获取节点列表
	nodeList := taskInfo.GetCranedList()

	if taskInfo.GetStatus().String() == "Completed" {
		state = "COMPLETED"
		reason = "ENDED"
	} else if taskInfo.GetStatus().String() == "Failed" {
		state = "FAILED"
		reason = "ENDED"
	} else if taskInfo.GetStatus().String() == "Cancelled" {
		state = "CANCELLED"
		reason = "ENDED"
	} else if taskInfo.GetStatus().String() == "Running" {
		state = "RUNNING"
		reason = "Running"
	} else if taskInfo.GetStatus().String() == "Pending" {
		state = "PENDING"
		reason = "Pending"
	}

	pods := utils.ConvertStepInfoToPodInfo(taskInfo.Partition, taskInfo.GetStepInfoList())

	if len(in.Fields) == 0 {
		jobInfo := &protos.JobInfo{
			JobId:            taskInfo.GetTaskId(),
			Name:             taskInfo.GetName(),
			Account:          taskInfo.GetAccount(),
			User:             taskInfo.GetUsername(),
			Partition:        taskInfo.GetPartition(),
			NodeList:         &nodeList,
			StartTime:        taskInfo.GetStartTime(),
			EndTime:          taskInfo.GetEndTime(),
			TimeLimitMinutes: taskInfo.GetTimeLimit().Seconds / 60, // 转换成分钟数
			WorkingDirectory: taskInfo.GetCwd(),
			CpusAlloc:        &cpusAllocInt32,
			State:            state,
			ElapsedSeconds:   &elapsedSeconds,
			Reason:           &reason,
			Qos:              taskInfo.GetQos(),
			SubmitTime:       taskInfo.GetStartTime(),
			Pods:             pods,
		}
		return &protos.GetJobByIdResponse{Job: jobInfo}, nil
	}
	jobInfo := &protos.JobInfo{}
	for _, field := range in.Fields {
		switch field {
		case "job_id":
			jobInfo.JobId = taskInfo.GetTaskId()
		case "name":
			jobInfo.Name = taskInfo.GetName()
		case "account":
			jobInfo.Account = taskInfo.GetAccount()
		case "user":
			jobInfo.User = taskInfo.GetUsername()
		case "partition":
			jobInfo.Partition = taskInfo.GetPartition()
		case "node_list":
			jobInfo.NodeList = &nodeList
		case "start_time":
			jobInfo.StartTime = taskInfo.GetStartTime()
		case "end_time":
			jobInfo.EndTime = taskInfo.GetEndTime()
		case "time_limit_minutes":
			jobInfo.TimeLimitMinutes = taskInfo.GetTimeLimit().Seconds / 60
		case "working_directory":
			jobInfo.WorkingDirectory = taskInfo.GetCwd()
		case "cpus_alloc":
			jobInfo.CpusAlloc = &cpusAllocInt32
		case "state":
			jobInfo.State = state
		case "elapsed_seconds":
			jobInfo.ElapsedSeconds = &elapsedSeconds
		case "reason":
			jobInfo.Reason = &reason
		case "qos":
			jobInfo.Qos = taskInfo.GetQos()
		case "pods":
			jobInfo.Pods = pods
		case "submit_time":
			jobInfo.SubmitTime = taskInfo.GetStartTime()
		}
	}
	logrus.Tracef("[GetJobById] job info: %v", jobInfo)
	return &protos.GetJobByIdResponse{Job: jobInfo}, nil
}

func (s *ServerJob) GetJobs(ctx context.Context, in *protos.GetJobsRequest) (*protos.GetJobsResponse, error) {
	var (
		request  *craneProtos.QueryTasksInfoRequest
		jobsInfo []*protos.JobInfo
		totalNum uint32
	)
	logrus.Infof("Received request GetJobs: %v", in)

	if in.Filter != nil {
		base := &craneProtos.QueryTasksInfoRequest{
			FilterTaskTypes:             []craneProtos.TaskType{craneProtos.TaskType_Container},
			FilterStates:                utils.GetCraneStatesList(in.Filter.States),
			FilterUsers:                 in.Filter.Users,
			FilterAccounts:              in.Filter.Accounts,
			OptionIncludeCompletedTasks: true,
			NumLimit:                    99999999,
		}

		var startTimeFilter, endTimeFilter int64
		interval := &craneProtos.TimeInterval{}

		if in.Filter.EndTime != nil {
			startTimeFilter = in.Filter.EndTime.StartTime.GetSeconds()
			endTimeFilter = in.Filter.EndTime.EndTime.GetSeconds()
		} else if in.Filter.SubmitTime != nil {
			startTimeFilter = in.Filter.SubmitTime.StartTime.GetSeconds()
			endTimeFilter = in.Filter.SubmitTime.EndTime.GetSeconds()
		}

		if startTimeFilter != 0 {
			interval.LowerBound = timestamppb.New(time.Unix(startTimeFilter, 0))
		}
		if endTimeFilter != 0 {
			interval.UpperBound = timestamppb.New(time.Unix(endTimeFilter, 0))
		}

		base.FilterEndTimeInterval = interval

		if in.Filter.JobName != nil {
			base.FilterTaskNames = []string{*in.Filter.JobName}
		}
		request = base
	} else {
		// 没有筛选条件的请求体
		request = &craneProtos.QueryTasksInfoRequest{
			OptionIncludeCompletedTasks: true,
			NumLimit:                    99999999,
		}
	}
	response, err := utils.CraneCtld.QueryTasksInfo(context.Background(), request)

	if err != nil {
		logrus.Errorf("GetJobs failed: %v", fmt.Errorf("CRANE_CALL_FAILED"))
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("GetJobs failed: %v", fmt.Errorf("CRANE_INTERNAL_ERROR"))
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}
	if len(response.GetTaskInfoList()) == 0 {
		logrus.Errorf("GetJobs failed: %v", fmt.Errorf("no Task found"))
		totalNum = uint32(len(response.GetTaskInfoList()))
		return &protos.GetJobsResponse{Jobs: jobsInfo, TotalCount: &totalNum}, nil
	}
	totalNum = uint32(len(response.GetTaskInfoList()))
	for _, job := range response.GetTaskInfoList() {
		var elapsedSeconds, timeLimitMinutes int64
		var state string
		var reason = "no reason"
		var nodeNum int32
		var endTime, startTime *timestamppb.Timestamp
		if job.GetStatus() == craneProtos.TaskStatus_Running {
			startTime = job.GetStartTime()
			elapsedSeconds = time.Now().Unix() - job.GetStartTime().Seconds
		} else if job.GetStatus() == craneProtos.TaskStatus_Pending {
			elapsedSeconds = 0
		} else {
			if job.GetNodeNum() != 0 {
				startTime = job.GetStartTime()
			}
			elapsedSeconds = job.GetEndTime().Seconds - job.GetStartTime().Seconds
		}
		cpusAlloc := job.GetAllocatedResView().AllocatableRes.CpuCoreLimit
		cpusAllocInt32 := int32(cpusAlloc)

		jobMemAllocMb := job.GetAllocatedResView().GetAllocatableRes().MemoryLimitBytes
		memAllocMb := int64(jobMemAllocMb / (1024 * 1024))

		jobGpusAlloc := job.GetAllocatedResView().GetDeviceMap()
		gpusAlloc := utils.GetGpuNumsFromJob(jobGpusAlloc)

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
			endTime = job.GetEndTime()
		} else {
			state = "IVALID"
			reason = "Ivalid"
		}
		nodeNum = int32(job.GetNodeNum())

		if job.GetTimeLimit() == nil || (job.GetTimeLimit().Seconds == 0 && job.GetTimeLimit().Nanos == 0) {
			timeLimitMinutes = maxUint
		} else {
			timeLimitMinutes = job.GetTimeLimit().Seconds / 60
			// 因为scow数据库中该值是uint类型的，当作业的TimeLimit大于该值时会插入该作业数据到数据库失败
			if timeLimitMinutes > maxUint {
				timeLimitMinutes = maxUint
			}
		}

		logrus.Tracef("GetJobs: job pod Info %v", job.GetPodMeta())
		logrus.Tracef("GetJobs: job step Info %v", job.GetStepInfoList())
		pods := utils.ConvertStepInfoToPodInfo(job.Partition, job.GetStepInfoList())
		if len(in.Fields) == 0 {
			subJobInfo := &protos.JobInfo{}
			subJobInfo = &protos.JobInfo{
				JobId:            job.GetTaskId(),
				Name:             job.GetName(),
				Account:          job.GetAccount(),
				User:             job.GetUsername(),
				Partition:        job.GetPartition(),
				StartTime:        startTime,
				EndTime:          endTime,
				NodesAlloc:       &nodeNum,
				TimeLimitMinutes: timeLimitMinutes,
				WorkingDirectory: job.GetCwd(),
				State:            state,
				NodeList:         &nodeList,
				CpusAlloc:        &cpusAllocInt32,
				ElapsedSeconds:   &elapsedSeconds,
				Qos:              job.GetQos(),
				Reason:           &reason,
				SubmitTime:       job.GetSubmitTime(),
				GpusAlloc:        &gpusAlloc,
				MemAllocMb:       &memAllocMb,
				Pods:             pods,
			}
			jobsInfo = append(jobsInfo, subJobInfo)
			logrus.Tracef("GetJobs: jobsInfo %v", subJobInfo)
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
					subJobInfo.StartTime = startTime
				case "end_time":
					subJobInfo.EndTime = endTime
				case "time_limit_minutes":
					subJobInfo.TimeLimitMinutes = timeLimitMinutes
				case "working_directory":
					subJobInfo.WorkingDirectory = job.GetCwd()
				case "cpus_req":
					subJobInfo.CpusReq = cpusAllocInt32
				case "cpus_alloc":
					subJobInfo.CpusAlloc = &cpusAllocInt32
				case "state":
					subJobInfo.State = state
				case "elapsed_seconds":
					subJobInfo.ElapsedSeconds = &elapsedSeconds
				case "qos":
					subJobInfo.Qos = job.GetQos()
				case "submit_time":
					subJobInfo.SubmitTime = job.GetSubmitTime()
				case "reason":
					subJobInfo.Reason = &reason
				case "nodes_req":
					subJobInfo.NodesReq = nodeNum
				case "nodes_alloc":
					subJobInfo.NodesAlloc = &nodeNum
				case "gpus_req":
					subJobInfo.GpusReq = gpusAlloc
				case "gpus_alloc":
					subJobInfo.GpusAlloc = &gpusAlloc
				case "mem_req_mb":
					subJobInfo.MemReqMb = memAllocMb
				case "pods":
					subJobInfo.Pods = pods
				case "mem_alloc_mb":
					subJobInfo.MemAllocMb = &memAllocMb
				}
			}
			logrus.Tracef("GetJobs: jobsInfo %v", subJobInfo)
			jobsInfo = append(jobsInfo, subJobInfo)
		}
	}
	if in.Sort != nil && len(jobsInfo) != 0 {
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
	logrus.Tracef("GetJobs jobs: %v", jobsInfo)
	return &protos.GetJobsResponse{Jobs: jobsInfo, TotalCount: &totalNum}, nil
}

// SubmitJob 命令 ccon run --userns=false -itd alpine:latest /bin/sh
func (s *ServerJob) SubmitJob(ctx context.Context, in *protos.SubmitJobRequest) (*protos.SubmitJobResponse, error) {
	logrus.Tracef("[SubmitJob] Received request: %v", in)

	err := s.checkJob(in.Account, in.UserId, in.WorkingDirectory)
	if err != nil {
		return nil, utils.RichError(codes.Internal, "SUBMIT_JOB_FAILED", err.Error())
	}
	//
	//partitionInfo, err := utils.GetPartitionByName(partitionName)
	//if err != nil {
	//	message := fmt.Sprintf("get partition %s info failed", partitionName)
	//	logrus.Errorf("[SubmitJob]: %v", message)
	//	return nil, utils.RichError(codes.Internal, "CREATE_FAILED", message)
	//}

	// 构建容器作业
	coordinator := builder.NewJobBuilderCoordinator()
	task, err := coordinator.BuildJob(in)
	if err != nil {
		logrus.Errorf("[SubmitJob] build job err: %v", err)
		return nil, utils.RichError(codes.Internal, "BUILD_JOB_FAILED", err.Error())
	}

	logrus.Tracef("[SubmitJob] task info: %v", task)
	// 提交到调度器
	jobID, err := s.submitToScheduler(task)
	if err != nil {
		logrus.Errorf("[SubmitJob] submit job err: %v", err)
		return nil, utils.RichError(codes.Internal, "SUBMIT_JOB_FAILED", err.Error())
	}

	logrus.Infof("[SubmitJob] submit job sucess: %v", jobID)

	//go func() {
	//	if in.ExtraOptions[0] == utils.APP {
	//		submitJobInfo := &utils.SubmitJobInfo{
	//			JobName: in.JobName,
	//			JobType: in.ExtraOptions[0],
	//		}
	//		jobInfo, err := getJobInfoWithRetry(jobID, 30, 2)
	//		if err != nil {
	//			logrus.Errorf("[SubmitJob] get job info err: %v", err)
	//		}
	//		forwardInfo, err := utils.BuildJobForwardInfo(jobInfo.PodMeta, jobInfo.StepInfoList)
	//		if err != nil {
	//			logrus.Warn("build job forward info failed: %v", err)
	//		}
	//		submitJobInfo.ForwardInfo = forwardInfo
	//
	//		err = utils.GlobalProxyManager.CreateAndStartProxy(submitJobInfo)
	//		if err != nil {
	//			logrus.Warn("Failed to create proxy for app job %v: %v", in.JobName, err)
	//		}
	//	}
	//}()

	var hostPorts, containerPorts []int32
	for _, port := range task.PodMeta.Ports {
		hostPorts = append(hostPorts, port.HostPort)
		containerPorts = append(containerPorts, port.ContainerPort)
	}
	submitJobInfo := &utils.SubmitJobInfo{
		JobName:        in.JobName,
		JobId:          jobID,
		JobType:        in.ExtraOptions[0],
		HostPorts:      hostPorts,
		ContainerPorts: containerPorts,
	}
	if err := s.JM.SaveJobInfo(submitJobInfo); err != nil {
		logrus.Warn("save job submit info failed: %v", err)
	}

	return &protos.SubmitJobResponse{
		JobId: jobID,
	}, nil
}

func (s *ServerJob) SubmitInferJob(ctx context.Context, in *protos.SubmitInferJobRequest) (*protos.SubmitInferJobResponse, error) {
	logrus.Tracef("[SubmitInferJob] Received request: %v", in)

	err := s.checkJob(in.Account, in.UserId, in.WorkingDirectory)
	if err != nil {
		return nil, utils.RichError(codes.Internal, "SUBMIT_INFERENCE_JOB_FAILED", err.Error())
	}

	// 构建容器作业
	coordinator := builder.NewJobBuilderCoordinator()
	task, err := coordinator.BuildInferenceJob(in)
	if err != nil {
		logrus.Errorf("[SubmitInferJob] build job err: %v", err)
		return nil, utils.RichError(codes.Internal, "BUILD_INFERENCE_JOB_FAILED", err.Error())
	}

	logrus.Tracef("[SubmitInferJob] task info: %v", task)
	// 提交到调度器
	jobID, err := s.submitToScheduler(task)
	if err != nil {
		logrus.Errorf("[SubmitInferJob] submit job err: %v", err)
		return nil, utils.RichError(codes.Internal, "SUBMIT_INFERENCE_JOB_FAILED", err.Error())
	}

	logrus.Infof("[SubmitInferJob] submit job sucess: %v", jobID)

	//go func() {
	//	submitJobInfo := &utils.SubmitJobInfo{
	//		JobName: in.JobName,
	//		JobType: utils.Inference,
	//	}
	//	jobInfo, err := utils.GetJobById(jobID)
	//	if err != nil {
	//		logrus.Errorf("[SubmitJob] get job info err: %v", err)
	//	}
	//	forwardInfo, err := utils.BuildJobForwardInfo(jobInfo.PodMeta, jobInfo.StepInfoList)
	//	if err != nil {
	//		logrus.Warn("build job forward info failed: %v", err)
	//	}
	//	submitJobInfo.ForwardInfo = forwardInfo
	//
	//	err = utils.GlobalProxyManager.CreateAndStartProxy(submitJobInfo)
	//	if err != nil {
	//		logrus.Warn("Failed to create proxy for app job %v: %v", in.JobName, err)
	//	}
	//}()

	var hostPorts, containerPorts []int32
	for _, port := range task.PodMeta.Ports {
		hostPorts = append(hostPorts, port.HostPort)
		containerPorts = append(containerPorts, port.ContainerPort)
	}
	submitJobInfo := &utils.SubmitJobInfo{
		JobName:        in.JobName,
		JobType:        utils.Inference,
		JobId:          jobID,
		HostPorts:      hostPorts,
		ContainerPorts: containerPorts,
	}
	if err := s.JM.SaveJobInfo(submitJobInfo); err != nil {
		logrus.Warn("save job submit info failed: %v", err)
	}

	return &protos.SubmitInferJobResponse{
		JobId: jobID,
	}, nil
}

func (s *ServerJob) CreateDevHost(ctx context.Context, in *protos.CreateDevHostRequest) (*protos.CreateDevHostResponse, error) {
	logrus.Tracef("[CreateDevHost] Received request: %v", in)

	err := s.checkJob(in.Account, in.UserId, in.WorkingDirectory)
	if err != nil {
		return nil, utils.RichError(codes.Internal, "CREATE_DEV_HOST_FAILED", err.Error())
	}
	// 构建容器作业
	coordinator := builder.NewJobBuilderCoordinator()
	task, err := coordinator.BuildDevHostJob(in)
	if err != nil {
		logrus.Errorf("[CreateDevHost] build job err: %v", err)
		return nil, utils.RichError(codes.Internal, "BUILD_DEV_HOST_FAILED", err.Error())
	}

	logrus.Tracef("[CreateDevHost] task info: %v", task)
	// 提交到调度器
	jobID, err := s.submitToScheduler(task)
	if err != nil {
		logrus.Errorf("[CreateDevHost] submit job err: %v", err)
		return nil, utils.RichError(codes.Internal, "CREATE_DEV_HOST_FAILED", err.Error())
	}

	logrus.Infof("[CreateDevHost] submit job sucess: %v", jobID)

	var hostPorts, containerPorts []int32
	for _, port := range task.PodMeta.Ports {
		hostPorts = append(hostPorts, port.HostPort)
		containerPorts = append(containerPorts, port.ContainerPort)
	}
	submitJobInfo := &utils.SubmitJobInfo{
		JobName:        in.JobName,
		JobType:        utils.DevHost,
		JobId:          jobID,
		HostPorts:      hostPorts,
		ContainerPorts: containerPorts,
	}

	if err := s.JM.SaveJobInfo(submitJobInfo); err != nil {
		logrus.Warn("save job submit info failed: %v", err)
	}

	//go func() {
	//	submitJobInfo := &utils.SubmitJobInfo{
	//		JobName: in.JobName,
	//		JobType: utils.DevHost,
	//	}
	//	jobInfo, err := getJobInfoWithRetry(jobID, 30, 2)
	//	if err != nil {
	//		logrus.Errorf("[SubmitJob] get job info err: %v", err)
	//	}
	//	forwardInfo, err := utils.BuildJobForwardInfo(jobInfo.PodMeta, jobInfo.StepInfoList)
	//	if err != nil {
	//		logrus.Warn("build job forward info failed: %v", err)
	//	}
	//	submitJobInfo.ForwardInfo = forwardInfo
	//
	//	err = utils.GlobalProxyManager.CreateAndStartProxy(submitJobInfo)
	//	if err != nil {
	//		logrus.Warn("Failed to create proxy for app job %v: %v", in.JobName, err)
	//	}
	//}()

	return &protos.CreateDevHostResponse{
		JobId: jobID,
	}, nil
}

func (s *ServerJob) StreamJobShell(stream protos.JobService_StreamJobShellServer) error {
	connectReq, err := s.waitForConnect(stream)
	if err != nil {
		logrus.Errorf("[StreamJobShell] wait for connect failed: %v", err)
		return utils.RichError(codes.Internal, "STREAM_JOB_SHELL_FAILED", err.Error())
	}
	logrus.Tracef("[StreamJobShell] Received request StreamJobShell connectInfo: %v", connectReq)
	jobID, stepID, nodeName, err := s.getJobIdStepIdNodeName(connectReq)
	if err != nil {
		logrus.Errorf("[StreamJobShell] get job id step id failed: %v", err)
		return utils.RichError(codes.Internal, "STREAM_JOB_SHELL_FAILED", err.Error())
	}

	taskInfo, err := utils.GetJobById(jobID, "")
	if err != nil {
		logrus.Errorf("[StreamJobShell] get job info err: %v", err)
		return utils.RichError(codes.Internal, "STREAM_JOB_SHELL_FAILED", err.Error())
	}
	// Check job step state
	if taskInfo.Status != craneProtos.TaskStatus_Running {
		message := fmt.Errorf("task %v state is: %s", jobID, taskInfo.Status.String())
		logrus.Errorf("[StreamJobShell] %v", message)
		return utils.RichError(codes.Internal, "STREAM_JOB_SHELL_FAILED", message.Error())
	}
	// 创建容器执行流
	streamURL, err := s.createContainerExecStream(jobID, stepID, taskInfo.Uid, nodeName)
	if err != nil {
		logrus.Errorf("[StreamJobShell] create container exec stream failed: %v", err)
		return fmt.Errorf("create container exec stream failed: %v", err)
	}
	logrus.Tracef("[StreamJobShell] streamURL: %v", streamURL)
	executor, err := s.createContainerExecutor(streamURL)
	if err != nil {
		logrus.Errorf("[StreamJobShell] create container executor failed: %v", err)
		return utils.RichError(codes.Internal, "CREATE_CONTAINER_EXECUTOR_FAILED", err.Error())
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// 初始化流管道和终端队列
	stdinR, stdinW := io.Pipe()   // 客户端输入 -> 容器 stdin
	stdoutR, stdoutW := io.Pipe() // 容器 stdout -> 客户端输出
	sizeQueue := NewTerminalSizeQueue()
	defer sizeQueue.Stop()

	// 启动容器流 Goroutine
	streamErrChan := make(chan error, 1)
	go s.runContainerStream(ctx, executor, stdinR, stdoutW, sizeQueue, streamErrChan)

	// 启动 Goroutine：转发容器输出到 gRPC 客户端
	stdoutDone := make(chan struct{})
	go s.forwardContainerOutput(stdoutR, stream, stdoutDone)

	// 处理客户端后续请求（数据输入/调整终端/断开）
	clientDone := make(chan struct{})
	go s.handleClientRequests(stream, stdinW, sizeQueue, clientDone)

	// 等待所有 Goroutine 完成
	streamErr := <-streamErrChan
	<-stdoutDone
	<-clientDone

	// 处理容器退出信息
	return s.handleStreamExit(stream, streamErr)
}

func (s *ServerJob) submitToScheduler(task *craneProtos.TaskToCtld) (uint32, error) {
	if err := utils.ValidateContainerJob(task); err != nil {
		return 0, fmt.Errorf("validation container job failed: %v", err)
	}

	reply, err := utils.SubmitContainerJob(task)
	if err != nil {
		return 0, fmt.Errorf(" submit container job failed: %v", err)
	}

	jobId, _, err := utils.GetContainerIDAndStepId(reply)
	if err != nil {
		return 0, fmt.Errorf(" get container job id failed: %v", err)
	}

	return jobId, nil
}

func (s *ServerJob) checkJob(accountName, userName, workdir string) error {
	// 先查询账户
	account, err := utils.GetAccountByName(accountName)
	if err != nil {
		message := fmt.Errorf("get account %v failed: %v", accountName, err)
		logrus.Errorf("[SubmitJob] %v", message)
		return message
	}
	if account.Blocked {
		message := fmt.Errorf("account %v is blocked", accountName)
		logrus.Errorf("[SubmitJob] %v", message)
		return message
	}

	user, err := utils.GetUserByName(userName)
	if err != nil {
		message := fmt.Errorf("get user %v failed: %v", userName, err)
		logrus.Errorf("[SubmitJob] %v", message)
		return message
	}
	if user.Blocked {
		message := fmt.Errorf("user %s is blocked", userName)
		logrus.Errorf("[SubmitJob]: %v", message)
		return message
	}

	// 工作目录由scow传过来一个绝对路径
	if !filepath.IsAbs(workdir) {
		message := fmt.Errorf("workdir %s is not absolute path", workdir)
		logrus.Errorf("[SubmitJob]: %v", message)
		return message
	}

	return nil
}
