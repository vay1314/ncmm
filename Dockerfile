# ---- 构建阶段 ----
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download

COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo none) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /ncmm main.go


# ---- 运行阶段 ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
ENV NCMM_DOCKER_OFFICIAL=1

COPY --from=builder /ncmm /usr/local/bin/ncmm

# 复制默认配置模板作为系统备份
COPY config/config.yaml /etc/ncmm/config.yaml
COPY config/notify.yaml /etc/ncmm/notify.yaml

# 复制并配置入口脚本
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# 设置工作目录
WORKDIR /data

ENTRYPOINT ["/entrypoint.sh"]
CMD ["--help"]
