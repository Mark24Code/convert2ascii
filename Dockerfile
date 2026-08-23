# convert2ascii —— Convert2Ascii 的 Go 版
# 单阶段构建：内置 Go 工具链、FFmpeg 运行时与开发库，产出的二进制与
# 镜像内 FFmpeg 同源，运行期无需额外依赖。
FROM golang:1.25-bookworm

# cgo 链接需要 gcc；FFmpeg 开发库在构建期编译进二进制
RUN apt-get update \
 && apt-get install -y --no-install-recommends gcc ffmpeg \
    libavcodec-dev libavformat-dev libavutil-dev libswscale-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .

RUN mkdir -p bin \
 && go build -o bin/image2ascii ./cmd/image2ascii \
 && go build -o bin/video2ascii ./cmd/video2ascii

# 容器内可直接调用 image2ascii / video2ascii
ENV PATH="/app/bin:${PATH}"

CMD ["image2ascii", "--version"]
