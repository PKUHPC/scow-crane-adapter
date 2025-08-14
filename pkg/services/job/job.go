package job

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/utils"
)

type ServerJob struct {
	protos.UnimplementedJobServiceServer
	ModulePath string
}

func (s *ServerJob) CancelJob(ctx context.Context, in *protos.CancelJobRequest) (*protos.CancelJobResponse, error) {
	logrus.Infof("Received request CancelJob: %v", in)
	request := &craneProtos.CancelTaskRequest{
		OperatorUid:   0,
		FilterTaskIds: []uint32{uint32(in.JobId)},
		FilterState:   craneProtos.TaskStatus_Invalid,
	}
	_, err := utils.CraneCtld.CancelTask(context.Background(), request)
	if err != nil {
		logrus.Errorf("CancelJob failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", "Crane service call failed.")
	}
	logrus.Infof("CancelJob job: %v success", in.JobId)
	return &protos.CancelJobResponse{}, nil
}

func (s *ServerJob) QueryJobTimeLimit(ctx context.Context, in *protos.QueryJobTimeLimitRequest) (*protos.QueryJobTimeLimitResponse, error) {
	var (
		jobIdList []uint32
		seconds   uint64
	)
	logrus.Infof("Received request QueryJobTimeLimit: %v", in)

	jobIdList = append(jobIdList, in.JobId)
	request := &craneProtos.QueryTasksInfoRequest{
		FilterTaskIds:               jobIdList,
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
	var (
		jobIdList []uint32
		seconds   uint64
	)
	logrus.Infof("Received request ChangeJobTimeLimit: %v", in)
	// 查询请求体
	jobIdList = append(jobIdList, in.JobId)
	requestLimitTime := &craneProtos.QueryTasksInfoRequest{
		FilterTaskIds: jobIdList,
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
	logrus.Infof("Received request GetJobById: %v", in)
	request := &craneProtos.QueryTasksInfoRequest{
		FilterTaskIds:               []uint32{uint32(in.JobId)},
		OptionIncludeCompletedTasks: true,
	}
	response, err := utils.CraneCtld.QueryTasksInfo(context.Background(), request)
	if err != nil {
		logrus.Errorf("GetJobById failed: %v", err)
		return nil, utils.RichError(codes.Unavailable, "CRANE_CALL_FAILED", err.Error())
	}
	if !response.GetOk() {
		logrus.Errorf("GetJobById failed: %v", fmt.Errorf("CRANE_INTERNAL_ERROR"))
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", "Crane service internal error.")
	}
	if len(response.GetTaskInfoList()) == 0 {
		logrus.Errorf("GetJobById failed: %v", fmt.Errorf("JOB_NOT_FOUND"))
		return nil, utils.RichError(codes.NotFound, "JOB_NOT_FOUND", "The job not found in crane.")
	}
	// 获取作业信息
	TaskInfoList := response.GetTaskInfoList()[0]
	if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - TaskInfoList.GetStartTime().Seconds
	} else if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Pending {
		elapsedSeconds = 0
	}
	// 获取作业时长
	// elapsedSeconds = TaskInfoList.GetEndTime().Seconds - TaskInfoList.GetStartTime().Seconds
	if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Running {
		elapsedSeconds = time.Now().Unix() - TaskInfoList.GetStartTime().Seconds
	} else if TaskInfoList.GetStatus() == craneProtos.TaskStatus_Pending {
		elapsedSeconds = 0
	} else {
		elapsedSeconds = TaskInfoList.GetEndTime().Seconds - TaskInfoList.GetStartTime().Seconds
	}

	// 获取cpu核分配数
	// cpusAlloc := TaskInfoList.GetAllocCpus()
	cpusAlloc := TaskInfoList.GetReqResView().GetAllocatableRes().CpuCoreLimit
	cpusAllocInt32 := int32(cpusAlloc)
	// 获取节点列表
	nodeList := TaskInfoList.GetCranedList()

	if TaskInfoList.GetStatus().String() == "Completed" {
		state = "COMPLETED"
		reason = "ENDED"
	} else if TaskInfoList.GetStatus().String() == "Failed" {
		state = "FAILED"
		reason = "ENDED"
	} else if TaskInfoList.GetStatus().String() == "Cancelled" {
		state = "CANCELLED"
		reason = "ENDED"
	} else if TaskInfoList.GetStatus().String() == "Running" {
		state = "RUNNING"
		reason = "Running"
	} else if TaskInfoList.GetStatus().String() == "Pending" {
		state = "PENDING"
		reason = "Pending"
	}

	if len(in.Fields) == 0 {
		jobInfo := &protos.JobInfo{
			JobId:            TaskInfoList.GetTaskId(),
			Name:             TaskInfoList.GetName(),
			Account:          TaskInfoList.GetAccount(),
			User:             TaskInfoList.GetUsername(),
			Partition:        TaskInfoList.GetPartition(),
			NodeList:         &nodeList,
			StartTime:        TaskInfoList.GetStartTime(),
			EndTime:          TaskInfoList.GetEndTime(),
			TimeLimitMinutes: TaskInfoList.GetTimeLimit().Seconds / 60, // 转换成分钟数
			WorkingDirectory: TaskInfoList.GetCwd(),
			CpusAlloc:        &cpusAllocInt32,
			State:            state,
			ElapsedSeconds:   &elapsedSeconds,
			Reason:           &reason,
			Qos:              TaskInfoList.GetQos(),
			SubmitTime:       TaskInfoList.GetStartTime(),
		}
		return &protos.GetJobByIdResponse{Job: jobInfo}, nil
	}
	jobInfo := &protos.JobInfo{}
	for _, field := range in.Fields {
		switch field {
		case "job_id":
			jobInfo.JobId = TaskInfoList.GetTaskId()
		case "name":
			jobInfo.Name = TaskInfoList.GetName()
		case "account":
			jobInfo.Account = TaskInfoList.GetAccount()
		case "user":
			jobInfo.User = TaskInfoList.GetUsername()
		case "partition":
			jobInfo.Partition = TaskInfoList.GetPartition()
		case "node_list":
			jobInfo.NodeList = &nodeList
		case "start_time":
			jobInfo.StartTime = TaskInfoList.GetStartTime()
		case "end_time":
			jobInfo.EndTime = TaskInfoList.GetEndTime()
		case "time_limit_minutes":
			jobInfo.TimeLimitMinutes = TaskInfoList.GetTimeLimit().Seconds / 60
		case "working_directory":
			jobInfo.WorkingDirectory = TaskInfoList.GetCwd()
		case "cpus_alloc":
			jobInfo.CpusAlloc = &cpusAllocInt32
		case "state":
			jobInfo.State = state
		case "elapsed_seconds":
			jobInfo.ElapsedSeconds = &elapsedSeconds
		case "reason":
			jobInfo.Reason = &reason
		case "qos":
			jobInfo.Qos = TaskInfoList.GetQos()
		case "submit_time":
			jobInfo.SubmitTime = TaskInfoList.GetStartTime()
		}
	}
	logrus.Tracef("GetJobById job: %v", jobInfo)
	return &protos.GetJobByIdResponse{Job: jobInfo}, nil
}

func (s *ServerJob) GetJobs(ctx context.Context, in *protos.GetJobsRequest) (*protos.GetJobsResponse, error) {
	var (
		startTimeFilter int64
		endTimeFilter   int64
		statesList      []craneProtos.TaskStatus
		// accountList     []string
		request  *craneProtos.QueryTasksInfoRequest
		jobsInfo []*protos.JobInfo
		totalNum uint32
		// submitTimeTimestamp *timestamppb.Timestamp
	)
	logrus.Infof("Received request GetJobs: %v", in)

	if in.Filter != nil {
		if in.Filter.EndTime != nil {
			startTimeFilter = in.Filter.EndTime.StartTime.GetSeconds()
			endTimeFilter = in.Filter.EndTime.EndTime.GetSeconds()
			statesList = utils.GetCraneStatesList(in.Filter.States)
			if startTimeFilter == 0 && endTimeFilter != 0 {
				// endTimeProto := timestamppb.New(time.Unix(endTimeFilter, 0))
				endTimeInterval := &craneProtos.TimeInterval{
					UpperBound: timestamppb.New(time.Unix(endTimeFilter, 0)), // 设置结束时间的时间戳
				}

				// UpperBound 表示右区间
				// LowerBound 表示左区间

				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              in.Filter.Accounts,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else if startTimeFilter != 0 && endTimeFilter != 0 {
				endTimeInterval := &craneProtos.TimeInterval{
					LowerBound: timestamppb.New(time.Unix(startTimeFilter, 0)),
					UpperBound: timestamppb.New(time.Unix(endTimeFilter, 0)),
				}

				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              in.Filter.Accounts,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else if startTimeFilter != 0 && endTimeFilter == 0 {
				endTimeInterval := &craneProtos.TimeInterval{
					LowerBound: timestamppb.New(time.Unix(startTimeFilter, 0)),
				}
				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              in.Filter.Accounts,
					FilterEndTimeInterval:       endTimeInterval,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			} else {
				request = &craneProtos.QueryTasksInfoRequest{
					FilterTaskStates:            statesList,
					FilterUsers:                 in.Filter.Users,
					FilterAccounts:              in.Filter.Accounts,
					OptionIncludeCompletedTasks: true,
					NumLimit:                    99999999,
				}
			}
		} else if in.Filter.JobName != nil {
			request = &craneProtos.QueryTasksInfoRequest{
				FilterTaskStates:            statesList,
				FilterUsers:                 in.Filter.Users,
				FilterAccounts:              in.Filter.Accounts,
				FilterTaskNames:             []string{*in.Filter.JobName},
				OptionIncludeCompletedTasks: true,
				NumLimit:                    99999999,
			}
		} else {
			statesList = utils.GetCraneStatesList(in.Filter.States)
			request = &craneProtos.QueryTasksInfoRequest{
				FilterTaskStates:            statesList,
				FilterUsers:                 in.Filter.Users,
				FilterAccounts:              in.Filter.Accounts,
				OptionIncludeCompletedTasks: true,
				NumLimit:                    99999999,
			}
		}
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
		var elapsedSeconds int64
		var state string
		var reason = "no reason"
		var nodeNum int32
		var endTime *timestamppb.Timestamp
		if job.GetStatus() == craneProtos.TaskStatus_Running {
			elapsedSeconds = time.Now().Unix() - job.GetStartTime().Seconds
		} else if job.GetStatus() == craneProtos.TaskStatus_Pending {
			elapsedSeconds = 0
		} else {
			elapsedSeconds = job.GetEndTime().Seconds - job.GetStartTime().Seconds
		}
		cpusReq := job.GetReqResView().GetAllocatableRes().CpuCoreLimit
		jobMemReqMb := job.GetReqResView().GetAllocatableRes().MemoryLimitBytes
		memReqMb := int64(jobMemReqMb / (1024 * 1024))

		cpusAlloc := job.GetAllocatedResView().GetAllocatableRes().CpuCoreLimit
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
		} else {
			state = "IVALID"
			reason = "Ivalid"
		}
		nodeNum = int32(job.GetNodeNum())
		if len(in.Fields) == 0 {
			jobsInfo = append(jobsInfo, &protos.JobInfo{
				JobId:            job.GetTaskId(),
				Name:             job.GetName(),
				Account:          job.GetAccount(),
				User:             job.GetUsername(),
				Partition:        job.GetPartition(),
				StartTime:        job.GetStartTime(),
				EndTime:          endTime,
				NodesAlloc:       &nodeNum,
				TimeLimitMinutes: job.GetTimeLimit().Seconds / 60,
				WorkingDirectory: job.GetCwd(),
				State:            state,
				NodesReq:         int32(len(job.GetReqNodes())),
				NodeList:         &nodeList,
				CpusReq:          int32(cpusReq),
				CpusAlloc:        &cpusAllocInt32,
				ElapsedSeconds:   &elapsedSeconds,
				Qos:              job.GetQos(),
				Reason:           &reason,
				SubmitTime:       job.GetStartTime(), // 没有提交时间，用开始时间来代替
				GpusAlloc:        &gpusAlloc,
				MemReqMb:         memReqMb,
				MemAllocMb:       &memAllocMb,
			})
			logrus.Tracef("GetJobs: jobsInfo %v", jobsInfo)
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
					subJobInfo.StartTime = job.GetStartTime()
				case "end_time":
					subJobInfo.EndTime = endTime
				case "time_limit_minutes":
					subJobInfo.TimeLimitMinutes = job.GetTimeLimit().Seconds / 60
				case "working_directory":
					subJobInfo.WorkingDirectory = job.GetCwd()
				case "cpus_req":
					subJobInfo.CpusReq = int32(cpusReq)
				case "mem_req_mb":
					subJobInfo.MemReqMb = memReqMb
				case "nodes_req":
					subJobInfo.NodesReq = int32(len(job.GetReqNodes()))
				case "cpus_alloc":
					subJobInfo.CpusAlloc = &cpusAllocInt32
				case "state":
					subJobInfo.State = state
				case "elapsed_seconds":
					subJobInfo.ElapsedSeconds = &elapsedSeconds
				case "qos":
					subJobInfo.Qos = job.GetQos()
				case "submit_time":
					subJobInfo.SubmitTime = job.GetStartTime() // 没有提交时间
				case "reason":
					subJobInfo.Reason = &reason
				case "nodes_alloc":
					subJobInfo.NodesAlloc = &nodeNum
				case "gpus_alloc":
					subJobInfo.GpusAlloc = &gpusAlloc
				case "mem_alloc_mb":
					subJobInfo.MemAllocMb = &memAllocMb
				}
			}
			logrus.Tracef("GetJobs: jobsInfo %v", jobsInfo)
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

func (s *ServerJob) SubmitJob(ctx context.Context, in *protos.SubmitJobRequest) (*protos.SubmitJobResponse, error) {
	var (
		stdout          string
		homedir         string
		timeLimitString string
		scriptString    = "#!/bin/bash\n"
	)
	logrus.Tracef("Received request SubmitJob: %v", in)

	if in.Stdout != nil {
		stdout = *in.Stdout
	} else { // 可选参数没传的情况
		stdout = "job.%j.out"
	}

	// 拼凑成绝对路径的工作目录
	isAbsolute := filepath.IsAbs(in.WorkingDirectory)
	if !isAbsolute {
		homedirTemp, _ := utils.GetUserHomedir(in.UserId)
		homedir = homedirTemp + "/" + in.WorkingDirectory
	} else {
		homedir = in.WorkingDirectory
	}

	scriptString += "#CBATCH " + "-A " + in.Account + "\n"
	scriptString += "#CBATCH " + "-p " + in.Partition + "\n"
	if in.Qos != nil {
		scriptString += "#CBATCH " + "--qos " + *in.Qos + "\n"
	}
	scriptString += "#CBATCH " + "-J " + in.JobName + "\n"
	scriptString += "#CBATCH " + "-N " + strconv.Itoa(int(in.NodeCount)) + "\n"
	scriptString += "#CBATCH " + "--ntasks-per-node " + strconv.Itoa(1) + "\n"
	if in.GpuCount != 0 {
		deviceType, err := utils.GetPartitionDeviceType(in.Partition)
		if err != nil {
			logrus.Errorf("SubmitJob failed: %v", fmt.Errorf("CREATE_SCRIPT_FAILED"))
			return nil, utils.RichError(codes.Aborted, "CREATE_SCRIPT_FAILED", "Create submit script failed.")
		}
		scriptString += "#CBATCH " + "--gres " + deviceType + ":" + strconv.Itoa(int(in.GpuCount)) + "\n"
	}
	scriptString += "#CBATCH " + "-c " + strconv.Itoa(int(in.CoreCount)) + "\n"
	if in.TimeLimitMinutes != nil {
		// 要把时间换成字符串的形式
		if *in.TimeLimitMinutes < 60 {
			timeLimitString = fmt.Sprintf("00:%s:00", strconv.Itoa(int(*in.TimeLimitMinutes)))
		} else if *in.TimeLimitMinutes == 60 {
			timeLimitString = "1:00:00"
		} else {
			hours, minitues := *in.TimeLimitMinutes/60, *in.TimeLimitMinutes%60
			timeLimitString = fmt.Sprintf("%s:%s:00", strconv.Itoa(int(hours)), strconv.Itoa(int(minitues)))
		}
		scriptString += "#CBATCH " + "--time " + timeLimitString + "\n"
	}
	scriptString += "#CBATCH " + "--chdir " + homedir + "\n"
	if in.Stdout != nil {
		scriptString += "#CBATCH " + "--output " + stdout + "\n"
	}

	if in.MemoryMb != nil {
		scriptString += "#CBATCH " + "--mem " + strconv.Itoa(int(*in.MemoryMb/uint64(in.NodeCount))) + "M" + "\n"
	}
	if len(in.ExtraOptions) != 0 {
		for _, extraVale := range in.ExtraOptions {
			scriptString += "#CBATCH " + extraVale + "\n"
		}
	}
	scriptString += "#CBATCH " + "--export ALL" + "\n"
	scriptString += "#CBATCH " + "--get-user-env" + "\n"

	modulePathString := fmt.Sprintf("source %s", s.ModulePath)
	scriptString += "\n" + modulePathString + "\n"

	scriptString += in.Script

	// 将这个保存成一个脚本文件，通过脚本文件进行提交
	// 生成一个随机的文件名
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	filePath := "/tmp" + "/" + string(b) + ".sh" // 生成的脚本存放路径
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		logrus.Errorf("SubmitJob failed: %v", fmt.Errorf("CREATE_SCRIPT_FAILED"))
		return nil, utils.RichError(codes.Aborted, "CREATE_SCRIPT_FAILED", "Create submit script failed.")
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString(scriptString)
	writer.Flush()

	os.Chmod(filePath, 0777)

	submitResult, err := utils.LocalSubmitJob(filePath, in.UserId)
	os.Remove(filePath) // 删除掉提交脚本
	if err != nil {
		logrus.Errorf("SubmitJob failed: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", submitResult)
	}
	responseList := strings.Split(strings.TrimSpace(string(submitResult)), " ")
	jobIdString := responseList[len(responseList)-1]

	jobId1, _ := strconv.Atoi(jobIdString[:len(jobIdString)-1])

	return &protos.SubmitJobResponse{JobId: uint32(jobId1), GeneratedScript: scriptString}, nil
}

func (s *ServerJob) SubmitScriptAsJob(ctx context.Context, in *protos.SubmitScriptAsJobRequest) (*protos.SubmitScriptAsJobResponse, error) {
	logrus.Tracef("Received request SubmitScriptAsJob: %v", in)
	// 具体的提交逻辑
	updateScript := "#!/bin/bash\n"
	trimmedScript := strings.TrimLeft(in.Script, "\n")
	// 通过换行符 "\n" 分割字符串
	checkBool1 := strings.Contains(trimmedScript, "--chdir")
	checkBool2 := strings.Contains(trimmedScript, " -D ")
	if !checkBool1 && !checkBool2 {
		chdirString := fmt.Sprintf("#SBATCH --chdir=%s\n", *in.ScriptFileFullPath)
		updateScript = updateScript + chdirString
		for _, value := range strings.Split(trimmedScript, "\n")[1:] {
			updateScript = updateScript + value + "\n"
		}
		in.Script = updateScript
	}
	// 将这个保存成一个脚本文件，通过脚本文件进行提交
	// 生成一个随机的文件名
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	filePath := "/tmp" + "/" + string(b) + ".sh" // 生成的脚本存放路径
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		logrus.Errorf("SubmitScriptAsJob failed: %v", fmt.Errorf("CREATE_SCRIPT_FAILED"))
		return nil, utils.RichError(codes.Aborted, "CREATE_SCRIPT_FAILED", "Create submit script failed.")
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString(in.Script)
	writer.Flush()

	submitResult, err := utils.LocalSubmitJob(filePath, in.UserId)
	os.Remove(filePath) // 删除生成的提交脚本
	if err != nil {
		logrus.Errorf("SubmitScriptAsJob failed: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_INTERNAL_ERROR", submitResult)
	}
	responseList := strings.Split(strings.TrimSpace(string(submitResult)), " ")
	jobIdString := responseList[len(responseList)-1]

	jobId1, _ := strconv.Atoi(jobIdString[:len(jobIdString)-1])

	return &protos.SubmitScriptAsJobResponse{JobId: uint32(jobId1)}, nil
}
