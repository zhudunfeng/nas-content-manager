# syntax=docker/dockerfile:1
# 前端构建环境：固定 Node 版本，依赖由 package-lock.json 锁定。
FROM node:22.14.0-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 后端构建环境：纯 Go SQLite，无需运行时 C 编译器。
FROM golang:1.27.4-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=frontend /src/web/dist ./internal/webassets/dist
RUN CGO_ENABLED=0 go build -trimpath -tags webembed -ldflags="-s -w" -o /out/nas ./cmd/nas

# 运行镜像：FFmpeg 和 ffprobe 随应用发布；tini 负责子进程回收。
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg curl tini \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 1000 nas && useradd --uid 1000 --gid nas --no-create-home nas \
    && mkdir -p /data /media /app/web/dist && chown nas:nas /data
WORKDIR /app
COPY --from=backend /out/nas /app/nas
COPY --from=frontend /src/web/dist /app/web/dist
# HTTP 监听全部容器网卡，固定使用 8090 端口。
ENV NAS_ADDR=0.0.0.0:8090
# SQLite、缓存和临时备份写入此持久目录。
ENV NAS_DATA_DIR=/data
# 网页允许浏览和登记的媒体根目录，由 Compose 只读挂载。
ENV NAS_MEDIA_ROOTS=/media
# 镜像中已构建的前端静态资源目录。
ENV NAS_WEB_DIR=/app/web/dist
# 使用非 root 用户，媒体宿主目录必须授予 UID/GID 1000 读取权限。
USER 1000:1000
# HTTP 服务端口，由 Compose 映射到 NAS 宿主机。
EXPOSE 8090
# 检查数据库与应用是否可用，不以单纯进程存活替代健康状态。
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD curl --fail --silent http://127.0.0.1:8090/healthz || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "/app/nas"]
