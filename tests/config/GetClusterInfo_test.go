package main

import (
	"context"
	// craneProtos "scow-crane-adapter/gen/crane"
	protos "scow-crane-adapter/gen/go"
	"testing"

	// "github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestGetClusterInfo(t *testing.T) {
	// Set up a connection to the server
	conn, err := grpc.Dial("localhost:8972", grpc.WithInsecure())
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := protos.NewConfigServiceClient(conn)

	// Call the Add RPC with test data
	req := &protos.GetClusterInfoRequest{}
	_, err = client.GetClusterInfo(context.Background(), req)
	if err != nil {
		t.Fatalf("GetClusterConfig failed: %v", err)
	}

	// Check the result
	// assert.IsType(t, []*protos.Partition{}, res.Partitions)
}