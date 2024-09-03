protos: 
	buf generate --template buf.gen.yaml https://github.com/PKUHPC/scow-scheduler-adapter-interface.git#subdir=protos,tag=v1.5.0

build:
	go build -o scow-crane-adapter ./cmd/main.go

test:
	go test

cranesched:
	buf generate --template buf.genCrane.yaml https://github.com/PKUHPC/CraneSched.git#subdir=protos,tag=master
