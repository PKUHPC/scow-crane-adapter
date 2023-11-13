protos: 
	buf generate --template buf.gen.yaml https://github.com/PKUHPC/scow-scheduler-adapter-interface.git#subdir=protos,tag=v1.3.0
run: 
	go run *.go

build:
	go build

test:
	go test