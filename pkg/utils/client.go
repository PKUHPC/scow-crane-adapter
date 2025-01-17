package utils

import (
	"fmt"
	"log"

	craneProtos "github.com/PKUHPC/CraneSched-FrontEnd/generated/protos"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	CraneCtld craneProtos.CraneCtldClient
	CConfig   *CraneConfig
)

// InitClientAndCraneConfig 为初始化CraneCtld客户端及鹤思配置文件
func InitClientAndCraneConfig() {
	CConfig = ParseConfig(DefaultConfigPath)
	serverAddr := fmt.Sprintf("%s:%s", CConfig.ControlMachine, CConfig.CraneCtldListenPort)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Cannot connect to CraneCtld: " + err.Error())
	}
	CraneCtld = craneProtos.NewCraneCtldClient(conn)
}
