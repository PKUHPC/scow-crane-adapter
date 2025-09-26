package monitor

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"scow-crane-adapter/pkg/utils"
)

func StartSystemMetricsCollector() {
	go func() {
		proc, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			logrus.Fatalf("Failed to get process info: %v", err)
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// 采集进程级别指标
			collectProcessMetrics(proc)

			// 采集数据库指标
			collectDatabaseMetrics()
		}
	}()
}

func collectProcessMetrics(proc *process.Process) {
	// 采集CPU使用率
	cpuPercent, err := proc.Percent(0)
	if err == nil {
		ProcessCpuUsage.Set(cpuPercent)
	}

	// 采集内存使用
	memInfo, err := proc.MemoryInfo()
	if err == nil {
		ProcessMemoryUsage.Set(float64(memInfo.RSS))
	}

	// 采集Goroutine数量
	ProcessGoroutines.Set(float64(runtime.NumGoroutine()))
}

func collectDatabaseMetrics() {
	// MongoDB数据库监控
	collectMongoDBMetrics()
}

func collectMongoDBMetrics() {
	// 获取连接数
	connCount, err := getConnectionCount(utils.MongoDBClient)
	if err != nil {
		logrus.Errorf("Failed to get active connections: %v", err)
	} else {
		DatabaseConnections.WithLabelValues("database", "active").Set(float64(connCount))
	}

	// 获取数据库大小
	sizes, err := getDatabaseSize(utils.MongoDBClient, utils.MongoDBConfig.DbName)
	if err != nil {
		logrus.Errorf("Failed to get database sizes: %v", err)
	} else {
		DatabaseSize.WithLabelValues(utils.MongoDBConfig.DbName).Set(float64(sizes))
	}
}

// 获取数据库连接数
func getConnectionCount(client *mongo.Client) (int, error) {
	// 在 admin 数据库上执行 serverStatus 命令
	var result struct {
		Connections struct {
			Current int `bson:"current"`
		} `bson:"connections"`
	}

	cmd := bson.D{{Key: "serverStatus", Value: 1}}
	err := client.Database("admin").RunCommand(context.TODO(), cmd).Decode(&result)
	if err != nil {
		return 0, fmt.Errorf("failed to get the number of connections: %v", err)
	}

	return result.Connections.Current, nil
}

// 获取数据库大小
func getDatabaseSize(client *mongo.Client, dbName string) (int64, error) {
	// 获取数据库状态
	var result struct {
		TotalSize int64 `bson:"totalSize"`
	}

	cmd := bson.D{{Key: "dbStats", Value: 1}}
	err := client.Database(dbName).RunCommand(context.TODO(), cmd).Decode(&result)
	if err != nil {
		return 0, fmt.Errorf("failed to get database size: %w", err)
	}

	return result.TotalSize, nil
}
