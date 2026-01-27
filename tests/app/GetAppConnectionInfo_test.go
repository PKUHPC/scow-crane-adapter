package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	pb "scow-crane-adapter/gen/go"
)

func TestGetAppConnectionInfo(t *testing.T) {

	// Set up a connection to the server
	conn, err := grpc.Dial("localhost:8972", grpc.WithInsecure(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*1024)))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewAppServiceClient(conn)
	appType := pb.AppType_APP_TYPE_VSCODE

	req := &pb.GetAppConnectionInfoRequest{
		JobId:   1,
		AppType: &appType,
	}
	res, err := client.GetAppConnectionInfo(context.Background(), req)
	fmt.Println(err)
	if err != nil {
		t.Fatalf("create dev host failed: %v", err)
	}
	fmt.Println(res)

	// assert.Empty(t, err)
	assert.IsType(t, uint32(1), res)
}
