package app

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"

	craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"scow-crane-adapter/pkg/utils"
)

type ServerApp struct {
	protos.UnimplementedAppServiceServer
	JM *utils.JobManager
}

func (s *ServerApp) GetAppConnectionInfo(ctx context.Context, in *protos.GetAppConnectionInfoRequest) (*protos.GetAppConnectionInfoResponse, error) {
	jobID := in.JobId
	taskInfo, err := utils.GetJobById(jobID, "")
	if err != nil {
		logrus.Errorf("[GetAppConnectionInfo] get job info err: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_FAILED", err.Error())
	}
	jobName := taskInfo.Name
	// Check job step state
	if taskInfo.Status != craneProtos.TaskStatus_Running {
		message := fmt.Errorf("task %v state is: %s", in.JobId, taskInfo.Status.String())
		logrus.Errorf("[GetAppConnectionInfo] %v", message)
		return nil, utils.RichError(codes.Internal, "CRANE_FAILED", message.Error())
	}

	logrus.Infof("task %v info is: %s", in.JobId, taskInfo)
	// 获取保存的作业信息
	jobInfo, err := s.JM.QueryJobInfo(jobID)
	if err != nil {
		logrus.Errorf("load job submit info failed: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_FAILED", err.Error())
	}

	submitJobProxyInfo := &utils.SubmitJobProxyInfo{
		JobName: jobName,
		JobId:   jobID,
	}

	forwardNodes, err := utils.BuildJobForwardInfo(taskInfo.PodMeta, taskInfo.StepInfoList)
	if err != nil {
		logrus.Errorf("build job forward info failed: %v", err)
		return nil, utils.RichError(codes.Internal, "BUILD_PROXY_FAILED", err.Error())
	}
	submitJobProxyInfo.ForwardNodes = forwardNodes
	logrus.Infof("task %v forwardNodes is: %v", in.JobId, forwardNodes)

	var (
		sessionInfo string
		hostPorts   = jobInfo.HostPorts
	)
	sessionInfo = "server_session_info.json"
	if in.AppType == nil {
		sessionInfo = "server_session_info.json"
	} else {
		switch *in.AppType {
		case protos.AppType_APP_TYPE_JUPYTER_LAB:
			sessionInfo = fmt.Sprintf("server_session_%s.json", utils.Jupyterlab)
			hostPorts = []int32{utils.JupyterPort}
		case protos.AppType_APP_TYPE_VSCODE:
			sessionInfo = fmt.Sprintf("server_session_%s.json", utils.Vscode)
			hostPorts = []int32{utils.VscodePort}
		default:
			sessionInfo = "server_session_info.json"
		}
	}

	err = utils.GlobalProxyManager.CreateAndStartProxy(submitJobProxyInfo, hostPorts)
	if err != nil {
		logrus.Errorf("Failed to create proxy for app job %v: %v", jobName, err)
		return nil, utils.RichError(codes.Internal, "BUILD_PROXY_FAILED", err.Error())
	}

	//task, step, err := utils.GetContainerStep(in.JobId, 1, false)
	//if err != nil {
	//	message := fmt.Errorf("get container step failed: %v", err)
	//	logrus.Errorf("[GetAppConnectionInfo] %v", message)
	//	return nil, utils.RichError(codes.Internal, "CRANE_FAILED", message.Error())
	//}

	primarySteps := utils.GetJobPrimaryStep(taskInfo.StepInfoList)
	step := primarySteps[0]
	//nodeName, err := utils.ResolveTargetNode(step)
	//if err != nil {
	//	message := fmt.Errorf("resolve target node failed: %v", err)
	//	logrus.Errorf("[GetAppConnectionInfo] %v", message)
	//	return nil, utils.RichError(codes.Internal, "CRANE_FAILED", message.Error())
	//}

	_, containerPort := taskInfo.PodMeta.Ports[0].HostPort, taskInfo.PodMeta.Ports[0].ContainerPort
	hostname, _ := os.Hostname()
	nodeName := forwardNodes[0].ExecutionNode

	jobType := jobInfo.JobType

	proxyInfo, err := utils.LoadJobProxyMetaFromFile(jobID, nodeName)
	if err != nil {
		logrus.Errorf("load job proxy info failed: %v", err)
		return nil, utils.RichError(codes.Internal, "CRANE_FAILED", err.Error())
	}
	logrus.Infof("proxy info: %v", proxyInfo)

	if jobType == utils.APP || jobType == utils.DevHost {
		webFilePath := fmt.Sprintf("/tmp/%s", sessionInfo)
		dstDir := "/tmp"
		currentTimestamp := time.Now().UnixMicro()
		strNum := strconv.FormatInt(currentTimestamp, 10)
		webFileDestPath := dstDir + "/" + "server_session_info.json" + "-" + strNum
		if err = utils.CopyFromPod(taskInfo.TaskId, step.StepId, webFilePath, webFileDestPath, nodeName); err != nil {
			logrus.Errorf("copy file failed: %v", err)
			return &protos.GetAppConnectionInfoResponse{}, nil
		}
		logrus.Tracef("copy container %s file %s successful", jobName, webFilePath)
		if containerPort == 6901 { // vnc
			// 生成随机密码
			rand.Seed(time.Now().UnixNano())
			charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			result := make([]byte, 10)
			for i := range result {
				result[i] = charset[rand.Intn(len(charset))]
			}
			randomPassword := string(result)
			cmd := fmt.Sprintf("echo -e %q | vncpasswd -f > ~/.vnc/passwd", randomPassword)
			err = utils.ExecContainerCMD(taskInfo.TaskId, step.StepId, nodeName, cmd)
			if err != nil {
				err = fmt.Errorf("modify vnc password failed, %v", err)
				logrus.Errorf("GetAppConnectionInfo failed: %v", err)
				return nil, utils.RichError(codes.Internal, "MODIFY_VNC_PASSWORD_FAILED", err.Error())
			}
			logrus.Info("modify vnc passwd successful")
			err = os.Remove(webFileDestPath)
			if err != nil {
				err = fmt.Errorf("remove web file failed, %v", err)
				logrus.Errorf("GetAppConnectionInfo failed: %v", err)
			}

			responseMessage := &protos.GetAppConnectionInfoResponse{
				Response: &protos.GetAppConnectionInfoResponse_AppConnectionInfo_{
					AppConnectionInfo: &protos.GetAppConnectionInfoResponse_AppConnectionInfo{
						Host:     hostname,
						Port:     uint32(proxyInfo.ProxyPort),
						Password: randomPassword,
					},
				},
			}
			logrus.Infof("GetAppConnectionInfo response: %v", responseMessage)
			return responseMessage, nil
		} else {
			_, webPassword, err := utils.GetWebJobFileContent(webFileDestPath)
			if err != nil {
				logrus.Errorf("GetAppConnectionInfo parse file %s failed, %v", webFileDestPath, err)
				return &protos.GetAppConnectionInfoResponse{}, nil
			}
			_ = os.Remove(webFileDestPath)
			responseMessage := &protos.GetAppConnectionInfoResponse{
				Response: &protos.GetAppConnectionInfoResponse_AppConnectionInfo_{
					AppConnectionInfo: &protos.GetAppConnectionInfoResponse_AppConnectionInfo{
						Host:     hostname,
						Port:     uint32(proxyInfo.ProxyPort),
						Password: webPassword,
					},
				},
			}
			logrus.Infof("GetAppConnectionInfo response: %v", responseMessage)
			return responseMessage, nil
		}
	} else if jobType == utils.Inference {
		responseMessage := &protos.GetAppConnectionInfoResponse{
			Response: &protos.GetAppConnectionInfoResponse_AppConnectionInfo_{
				AppConnectionInfo: &protos.GetAppConnectionInfoResponse_AppConnectionInfo{
					Host: hostname,
					Port: uint32(proxyInfo.ProxyPort),
				},
			},
		}
		logrus.Tracef("GetAppConnectionInfo response: %v", responseMessage)
		return responseMessage, nil
	} else {
		// 目前只支持app的连接, 训练不支持连接
		err = fmt.Errorf("not support")
		logrus.Errorf("GetAppConnectionInfo failed: %v", err)
		return nil, utils.RichError(codes.Internal, "NOT_SUPPORT", err.Error())
	}
}
