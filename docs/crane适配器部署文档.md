# **Crane适配器安装部署文档**


## **1 Crane适配器安装部署环境要求**

### **1.1 准备一台能连外网的服务器或虚拟机用来编译生成二进制文件**
### **1.2 在准备的服务器或虚拟机安装go语言、配置go相关环境变量**

```bash
# 下载go语言安装包，安装go
cd download/
wget https://golang.google.cn/dl/go1.19.7.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.19.7.linux-amd64.tar.gz

# 在/etc/profile中设置环境变量
export GOROOT=/usr/local/go
export GOPATH=/usr/local/gopath
export PATH=$PATH:/$GOROOT/bin:$GOPATH/bin

# source环境变量
source /etc/profile

# 验证
go version

# 设置代理
go env -w GOPROXY=https://goproxy.cn,direct

# 开启go mod管理
go env -w GO111MODULE=on
```

### **1.3 在准备好的服务器或虚拟机上安装buf**
```bash
GO111MODULE=on
GOBIN=/usr/local/bin
go install github.com/bufbuild/buf/cmd/buf@v1.19.0
```

## **2 编译Crane适配器项目**

### **2.1 在准备好的服务器或虚拟机上拉取Crane适配器代码**
```bash
cd /root    # 将crane适配器代码放在root目录下
git clone https://github.com/PKUHPC/scow-crane-adapter.git  #克隆代码
```


### **2.2 生成proto文件**
```bash
# 在scow-crane-adapter目录下执行下面命令
[root@crane01 scow-crane-adapter]# make protos

# 执行完上面的命令后会在当前目录下生成gen/go目录和相关的proto文件
[root@crane01 scow-crane-adapter]# ls gen/go/*
account_grpc.pb.go  account.pb.go  config_grpc.pb.go  config.pb.go  Crane_grpc.pb.go  Crane.pb.go  CraneSubprocess.pb.go  job_grpc.pb.go  job.pb.go  PublicDefs.pb.go  user_grpc.pb.go  user.pb.go

# 在scow-crane-adapter目录下执行下面命令
[root@crane01 scow-crane-adapter]# make cranesched

# 执行完上面的命令后会在当前目录下生成gen/目录和相关的proto文件
[root@crane01 scow-crane-adapter]# ls gen/crane/*
gen/crane/Crane.pb.go  gen/crane/CraneSubprocess.pb.go  gen/crane/Crane_grpc.pb.go  gen/crane/PublicDefs.pb.go
```

### **2.3 编译项目**
```bash
# 在代码根目录下执行make build生成二进制文件(scow-crane-adapter)
[root@crane01 scow-crane-adapter]# make build 
go build -o scow-crane-adapter ./cmd/main.go

[root@crane01 scow-crane-adapter]# ls
Makefile  README.md  buf.gen.yaml  buf.genCrane.yaml  cmd  config.yaml  docs  gen  go.mod  go.sum  pkg  protos  scow-crane-adapter  server.log  tests
```

## **3 部署Crane适配器（将服务器上生成的二进制文件拷贝至 Crane管理节点）**
### **4.1 将服务器上生成的执行程序拷贝至Crane管理节点的部署目录中**
```bash
# 将服务器或虚拟机上生成的二进制文件拷贝至需要部署适配器的Crane管理节点上
scp -f scow-crane-adapter config.yaml crane_mn:/adapter     
# crane_mn 为需要部署适配器的crane管理节点、/adapter目录为部署目录
```

### **4.3 启动Crane适配器**
```bash
# 在Crane管理节点上启动服务
cd /adapter && nohup ./scow-crane-adapter -c config.yaml > server.log 2>&1 &
```

