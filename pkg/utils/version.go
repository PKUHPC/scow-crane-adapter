package utils

import "fmt"

const (
	Version = "1.9.4"
)

var (
	GitCommit = "UnKnown"
	BuildTime = "Unknown"
)

func GetVersion() string {
	return fmt.Sprintf("Adapter Version: %s-%s\nBuild Time: %s", Version, GitCommit, BuildTime)
}

func VersionTemplate() string {
	return `{{.Version}}` + "\n"
}
