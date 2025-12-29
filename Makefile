UNAME_M ?= $(shell uname -m)
ifeq ($(UNAME_M),x86_64)
	ARCH := amd64
endif

ifeq ($(UNAME_M),aarch64)
	ARCH := arm64
endif

COMMIT_ID := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date +'%a, %d %b %Y %H:%M:%S %z')
LDFLAGS := -ldflags \
           "-X 'scow-crane-adapter/pkg/utils.GitCommit=$(COMMIT_ID)' \
           -X 'scow-crane-adapter/pkg/utils.BuildTime=$(BUILD_TIME)'"


protos:
	buf generate --template buf.gen.yaml https://github.com/PKUHPC/scow-scheduler-adapter-interface.git#subdir=protos,tag=v1.14.0
build:
	CGO_ENABLED=1 GOARCH=${ARCH} go build $(LDFLAGS) -o scow-crane-adapter ./cmd/main.go

test:
	go test

cranesched:
	buf generate --template buf.genCrane.yaml https://github.com/PKUHPC/CraneSched.git#subdir=protos,tag=7dbe26ae6fc32de109b1f90a5aea64dfd3e3cd05
