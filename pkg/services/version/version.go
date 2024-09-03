package version

import (
	"context"

	"github.com/sirupsen/logrus"
	protos "scow-crane-adapter/gen/go"
)

type ServerVersion struct {
	protos.UnimplementedVersionServiceServer
}

// GetVersion means get version
func (s *ServerVersion) GetVersion(ctx context.Context, in *protos.GetVersionRequest) (*protos.GetVersionResponse, error) {
	// 记录日志
	logrus.Infof("Received request GetVersion: %v", in)
	return &protos.GetVersionResponse{Major: 1, Minor: 5, Patch: 0}, nil

}
