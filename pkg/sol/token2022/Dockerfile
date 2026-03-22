# 使用官方 Golang 镜像作为基础镜像
FROM golang:1.17

# 设置工作目录
WORKDIR /app

# 安装 anchor-go 所需的工具
RUN apt-get update && apt-get install -y \
    git \
    make \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# 安装 go module 依赖
RUN go install github.com/gagliardetto/anchor-go@latest

# 将本地文件夹映射到容器中的 /app 目录
# 假设你会在运行时使用 Docker -v 选项来挂载本地的 idl.json 和生成目录

# 设置启动命令（通过容器执行 anchor-go 命令）
CMD ["anchor-go"]