# 使用指定的阿里云CR镜像作为基础镜像(支持双架构)
FROM crpi-7a6qgvce1ic8faue.cn-hangzhou.personal.cr.aliyuncs.com/qinguangrui/rockylinux:8.9 AS builder

# 使用构建参数来指定目标架构
ARG TARGETARCH

# 设置工作目录
WORKDIR /app

# 安装必要的系统工具和依赖
RUN dnf install -y wget tar gzip make git gcc && \
    dnf clean all

# 下载并安装 Go 1.23.5
RUN wget https://mirrors.aliyun.com/golang/go1.23.5.linux-${TARGETARCH}.tar.gz -O /tmp/go.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

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