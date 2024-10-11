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
	logrus.Infof("Received request GetVersion: %v", in)
	return &protos.GetVersionResponse{Major: 1, Minor: 8, Patch: 0}, nil
}
