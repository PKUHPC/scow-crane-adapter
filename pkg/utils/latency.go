package utils

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// UnaryServerLatencyInterceptor 统一记录耗时和函数名
func UnaryServerLatencyInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	logrus.Infof("%s time consuming=%v", info.FullMethod, time.Since(start))
	return resp, err
}
