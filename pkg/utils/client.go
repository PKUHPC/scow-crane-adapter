package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	craneProtos "scow-crane-adapter/gen/crane"
)

const (
	DefaultUserConfigPrefix = ".config/crane"
)

var (
	CraneCtld             craneProtos.CraneCtldClient
	CConfig               *CraneConfig
	MongoDBClient         *mongo.Client
	MongoDBConfig         *DatabaseConfig
	ClientKeepAliveParams = keepalive.ClientParameters{
		Time:                20 * time.Second, // 20s GRPC_ARG_KEEPALIVE_TIME_MS
		Timeout:             10 * time.Second, // 10s GRPC_ARG_KEEPALIVE_TIMEOUT_MS
		PermitWithoutStream: true,             // GRPC_ARG_KEEPALIVE_PERMIT_WITHOUT_CALLS
	}

	ClientConnectParams = grpc.ConnectParams{
		Backoff: backoff.Config{
			BaseDelay: 1 * time.Second,  // 1s GRPC_ARG_INITIAL_RECONNECT_BACKOFF_MS
			MaxDelay:  30 * time.Second, // 30s GRPC_ARG_MAX_RECONNECT_BACKOFF_MS
			//No min delay available GRPC_ARG_MIN_RECONNECT_BACKOFF_MS
		},
	}
)

// InitClientAndConfig 为初始化CraneCtld客户端及鹤思配置文件
func InitClientAndConfig() {
	var (
		serverAddr string
		err        error
	)
	CConfig = ParseConfig(DefaultConfigPath)

	if CConfig.TlsConfig.Enabled {
		serverAddr = fmt.Sprintf("%s.%s:%s",
			CConfig.ControlMachine, CConfig.TlsConfig.DomainSuffix, CConfig.CraneCtldListenPort)

		if CConfig.TlsConfig.UserTlsCertPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				log.Fatal(err.Error())
			}
			CConfig.TlsConfig.UserTlsCertPath = filepath.Join(home, DefaultUserConfigPrefix)
		}

		tlsConfig, err := ReadTLSConfig(CConfig)
		if err != nil {
			log.Fatalf("Failed to load user certificate: %v", err)
		}

		conn, err := grpc.NewClient(serverAddr,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
			grpc.WithKeepaliveParams(ClientKeepAliveParams),
			grpc.WithConnectParams(ClientConnectParams),
			grpc.WithIdleTimeout(time.Duration(math.MaxInt64)),
		)
		if err != nil {
			logrus.Errorln("Cannot connect to CraneCtld: " + err.Error())
			os.Exit(4)
		}

		CraneCtld = craneProtos.NewCraneCtldClient(conn)
	} else {
		serverAddr = fmt.Sprintf("%s:%s", CConfig.ControlMachine, CConfig.CraneCtldListenPort)

		conn, err := grpc.NewClient(serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithKeepaliveParams(ClientKeepAliveParams),
			grpc.WithConnectParams(ClientConnectParams),
			grpc.WithIdleTimeout(time.Duration(math.MaxInt64)), // GRPC_ARG_CLIENT_IDLE_TIMEOUT_MS
		)
		if err != nil {
			logrus.Errorf("Cannot connect to CraneCtld %s: %s", serverAddr, err.Error())
			os.Exit(4)
		}

		CraneCtld = craneProtos.NewCraneCtldClient(conn)
	}

	// 加载配置
	MongoDBConfig, err = LoadDBConfig(DefaultMongoDBPath)
	if err != nil {
		log.Fatalf("Loading configuration failed: %v", err)
	}

	// 创建 MongoDB 客户端
	client, err := createMongoClient(MongoDBConfig)
	if err != nil {
		log.Fatalf("Failed to create MongoDB client: %v", err)
	}

	MongoDBClient = client
}

func ReadTLSConfig(config *CraneConfig) (*tls.Config, error) {
	userKeyPath := fmt.Sprintf("%s/user.key", config.TlsConfig.UserTlsCertPath)
	userCertPath := fmt.Sprintf("%s/user.pem", config.TlsConfig.UserTlsCertPath)

	if !FileExists(userKeyPath) || !FileExists(userCertPath) || !FileExists(config.TlsConfig.ExternalCertFilePath) {
		return nil, fmt.Errorf("certificate files not found")
	}

	cert, err := tls.LoadX509KeyPair(userCertPath, userKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %v", err)
	}

	CaCertContent, err := os.ReadFile(config.TlsConfig.ExternalCertFilePath)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(CaCertContent); !ok {
		return nil, fmt.Errorf("failed to append cert Content")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return tlsConfig, nil
}

// 创建 MongoDB 客户端
func createMongoClient(config *DatabaseConfig) (*mongo.Client, error) {
	// 构建连接字符串
	uri := fmt.Sprintf("mongodb://%s:%s@%s:%d",
		config.DbUser,
		config.DbPassword,
		config.DbHost,
		config.DbPort)

	// 设置客户端选项
	clientOptions := options.Client().ApplyURI(uri)

	// 如果配置了副本集名称
	if config.DbReplSetName != "" {
		clientOptions.SetReplicaSet(config.DbReplSetName)
	}

	// 连接到 MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// 检查连接
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return nil, fmt.Errorf("MongoDB connection test failed: %v", err)
	}

	return client, nil
}
