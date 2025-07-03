# 使用指定的阿里云CR镜像作为基础镜像
FROM crpi-7a6qgvce1ic8faue.cn-hangzhou.personal.cr.aliyuncs.com/qinguangrui/rockylinux:8.9-go AS builder

# 设置工作目录
WORKDIR /app

# 设置 Go 环境变量
ENV GOPATH /go
ENV PATH $PATH:/usr/local/go/bin:$GOPATH/bin
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,direct

# 复制代码到容器中
COPY . .

RUN go mod tidy

# 执行编译
RUN make build

# 创建干净的输出阶段
FROM scratch AS output
COPY --from=builder /app /app