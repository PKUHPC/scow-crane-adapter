package utils

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	craneProtos "scow-crane-adapter/gen/crane"
)

var (
	CraneCtld     craneProtos.CraneCtldClient
	CConfig       *CraneConfig
	MongoDBClient *mongo.Client
	MongoDBConfig *DatabaseConfig
)

// InitClientAndConfig 为初始化CraneCtld客户端及鹤思配置文件、MongoDB客户端及配置文件
func InitClientAndConfig() {
	CConfig = ParseConfig(DefaultConfigPath)
	serverAddr := fmt.Sprintf("%s:%s", CConfig.ControlMachine, CConfig.CraneCtldListenPort)
	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Cannot connect to CraneCtld: " + err.Error())
	}
	CraneCtld = craneProtos.NewCraneCtldClient(conn)

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
