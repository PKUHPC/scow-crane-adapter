package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	Version = "1.9.11"
)

var (
	GitCommit = "UnKnown"
	BuildTime = "Unknown"
)

var AdapterStartTime = time.Now()

func SetAdapterStartTime(t time.Time) {
	AdapterStartTime = t
}

func GetVersion() string {
	return fmt.Sprintf("Adapter Version: %s-%s\nBuild Time: %s", Version, GitCommit, BuildTime)
}

func VersionTemplate() string {
	return `{{.Version}}` + "\n"
}

var craneVersionOnce sync.Once
var craneVersion string

func GetCraneVersion() string {
	craneVersionOnce.Do(func() {
		craneVersion = detectCraneVersion()
	})
	return craneVersion
}

func detectCraneVersion() string {
	if v := strings.TrimSpace(os.Getenv("CRANESCHED_VERSION")); v != "" {
		return v
	}

	candidates := [][]string{
		{"cinfo", "--version"},
		{"ccontrol", "--version"},
	}

	for _, args := range candidates {
		out, err := commandOutputWithTimeout(2*time.Second, args[0], args[1:]...)
		if err != nil {
			continue
		}
		out = strings.TrimSpace(out)
		if out == "" {
			continue
		}
		if v := extractCraneSchedVersionLine(out); v != "" {
			return v
		}
		if v := extractSemanticVersion(out); v != "" {
			return v
		}
		if line := firstNonEmptyLine(out); line != "" {
			return line
		}
	}

	return "unknown"
}

func commandOutputWithTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var craneSchedVersionLineRE = regexp.MustCompile(`(?m)\bCraneSched\s+(\S+)(?:\s+\(git:\s*([0-9a-fA-F]+)\))?\b`)
var semanticVersionRE = regexp.MustCompile(`\bv?\d+\.\d+\.\d+(?:\.\d+)*(?:[-+][0-9A-Za-z.-]+)?\b`)

func extractCraneSchedVersionLine(s string) string {
	m := craneSchedVersionLineRE.FindStringSubmatch(s)
	if len(m) == 0 {
		return ""
	}
	version := m[1]
	git := ""
	if len(m) >= 3 {
		git = m[2]
	}
	if git != "" {
		return fmt.Sprintf("%s (git: %s)", version, git)
	}
	return version
}

func extractSemanticVersion(s string) string {
	return semanticVersionRE.FindString(s)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if v := strings.TrimSpace(line); v != "" {
			return v
		}
	}
	return ""
}
