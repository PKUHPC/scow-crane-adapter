package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"reflect"
	"sort"
	"strconv"

	protos "scow-crane-adapter/gen/go"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v2"
)

type Config struct {
	ControlMachine      string `yaml:"ControlMachine"`
	CraneCtldListenPort string `yaml:"CraneCtldListenPort"`

	UseTls             bool        `yaml:"UseTls"`
	ServerCertFilePath string      `yaml:"ServerCertFilePath"`
	ServerKeyFilePath  string      `yaml:"ServerKeyFilePath"`
	CaCertFilePath     string      `yaml:"CaCertFilePath"`
	DomainSuffix       string      `yaml:"DomainSuffix"`
	Partitions         []Partition `yaml:"Partitions"`
}

type Partition struct {
	Name  string `yaml:"name"`
	Nodes string `yaml:"nodes"`
}

var DefaultConfigPath = "/etc/crane/config.yaml"

// 解析crane配置文件
func ParseConfig(configFilePath string) *Config {
	confFile, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatal(err)
	}
	config := &Config{}
	err = yaml.Unmarshal(confFile, config)
	if err != nil {
		log.Fatal(err)
	}
	return config
}

// 通过os/user包去获取用户的uid
func GetUidByUserName(userName string) (int, error) {
	u, err := user.Lookup(userName)
	if err != nil {
		fmt.Printf("Failed to lookup user: %s\n", err)
		return 0, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	return uid, nil
}

// rich error model 封装
func RichError(code codes.Code, reason string, message string) error {
	errInfo := &errdetails.ErrorInfo{
		Reason: reason,
	}
	st := status.New(code, message)
	st, _ = st.WithDetails(errInfo)
	return st.Err()
}

// 获取系统中Qos列表
func GetQos() ([]string, error) {
	var (
		Qoslist []string
	)
	config := ParseConfig(DefaultConfigPath)
	serverAddr := fmt.Sprintf("%s:%s", config.ControlMachine, config.CraneCtldListenPort)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Cannot connect to CraneCtld: " + err.Error())
	}
	defer conn.Close()
	stubCraneCtld := protos.NewCraneCtldClient(conn)
	request := &protos.QueryEntityInfoRequest{
		Uid:        uint32(os.Getuid()),
		EntityType: protos.EntityType_Qos,
	}
	response, err := stubCraneCtld.QueryEntityInfo(context.Background(), request)
	if err != nil {
		return []string{}, err
	}
	Qos := response.GetQosList()
	for _, value := range Qos {
		Qoslist = append(Qoslist, value.GetName())
	}
	return Qoslist, nil
}

func GetCraneStatesList(stateList []string) []protos.TaskStatus {
	var (
		statesList []protos.TaskStatus
	)
	for _, value := range stateList {
		if value == "PENDING" || value == "PENDDING" {
			statesList = append(statesList, protos.TaskStatus_Pending)
		} else if value == "RUNNING" {
			statesList = append(statesList, protos.TaskStatus_Running)
		} else if value == "CANCELED" {
			statesList = append(statesList, protos.TaskStatus_Cancelled)
		} else if value == "COMPLETED" {
			statesList = append(statesList, protos.TaskStatus_Completed)
		} else if value == "FAILED" || value == "NODE_FAIL" {
			statesList = append(statesList, protos.TaskStatus_Failed)
		} else if value == "TIMEOUT" {
			statesList = append(statesList, protos.TaskStatus_ExceedTimeLimit)
		} else {
			statesList = append(statesList, protos.TaskStatus_Invalid)
		}
	}
	return statesList
}

func GetUserHomedir(username string) (string, error) {
	// 获取指定用户名的用户信息
	u, err := user.Lookup(username)
	if err != nil {
		return "", err
	}

	// 获取家目录
	homeDir := u.HomeDir
	return homeDir, nil
}

func RemoveValue(list []string, value string) []string {
	result := []string{}
	for _, item := range list {
		if item != value {
			result = append(result, item)
		}
	}
	return result
}

func sortByKey(list []*protos.JobInfo, fieldName string, sortOrder string) bool {
	if sortOrder == "ASC" {
		sort.Slice(list, func(i, j int) bool {
			fieldValueI := reflect.ValueOf(list[i]).Elem().FieldByName(fieldName)
			fieldValueJ := reflect.ValueOf(list[j]).Elem().FieldByName(fieldName)
			switch fieldValueI.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return fieldValueI.Int() < fieldValueJ.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return fieldValueI.Uint() > fieldValueJ.Uint()
			case reflect.Float32, reflect.Float64:
				return fieldValueI.Float() < fieldValueJ.Float()
			case reflect.String:
				return fieldValueI.String() < fieldValueJ.String()
			default:
				return false
			}
		})
	} else if sortOrder == "DESC" {
		sort.Slice(list, func(i, j int) bool {
			fieldValueI := reflect.ValueOf(list[i]).Elem().FieldByName(fieldName)
			fieldValueJ := reflect.ValueOf(list[j]).Elem().FieldByName(fieldName)
			switch fieldValueI.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return fieldValueI.Int() > fieldValueJ.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return fieldValueI.Uint() > fieldValueJ.Uint()
			case reflect.Float32, reflect.Float64:
				return fieldValueI.Float() > fieldValueJ.Float()
			case reflect.String:
				return fieldValueI.String() > fieldValueJ.String()
			default:
				return false
			}
		})
	}
	return true
}

func SortJobInfo(sortKey string, sortOrder string, jobInfo []*protos.JobInfo) []*protos.JobInfo {
	sortByKey(jobInfo, sortKey, sortOrder)
	return jobInfo
}
