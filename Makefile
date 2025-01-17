UNAME_M ?= $(shell uname -m)
ifeq ($(UNAME_M),x86_64)
	ARCH := amd64
endif

ifeq ($(UNAME_M),aarch64)
	ARCH := arm64
endif

protos:
	buf generate --template buf.gen.yaml https://github.com/PKUHPC/scow-scheduler-adapter-interface.git#subdir=protos,tag=v1.8.0

build:
	CGO_BUILD=0 GOARCH=${ARCH} go build -o scow-crane-adapter-${ARCH} ./cmd/main.go

test:
	go test