package version

import (
	"context"
	"scow-crane-adapter/pkg/utils"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	protos "scow-crane-adapter/gen/go"
)

type ServerVersion struct {
	protos.UnimplementedVersionServiceServer
}

// GetVersion means get version
func (s *ServerVersion) GetVersion(ctx context.Context, in *protos.GetVersionRequest) (*protos.GetVersionResponse, error) {
	logrus.Infof("Received request GetVersion: %v", in)
	versionSlice := strings.Split(utils.Version, ".")
	if len(versionSlice) == 3 {
		major, _ := strconv.Atoi(versionSlice[0])
		minor, _ := strconv.Atoi(versionSlice[1])
		patch, _ := strconv.Atoi(versionSlice[2])
		return &protos.GetVersionResponse{Major: uint32(major), Minor: uint32(minor), Patch: uint32(patch)}, nil
	}
	logrus.Infof("versionSlice: %v", versionSlice)
	return &protos.GetVersionResponse{Major: 1, Minor: 9, Patch: 1}, nil
}
