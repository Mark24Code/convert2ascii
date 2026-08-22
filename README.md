# go2ascii

Go 版 convert2ascii：把图片 / 视频在终端里渲染为 ASCII art。提供两个 CLI：`image2ascii`（单张图片）与 `video2ascii`（视频，支持生成、保存、播放）。

## Build

需要 Go ≥ 1.25 与 FFmpeg ≥ 6 开发库：

```bash
brew install ffmpeg      # macOS（含开发头文件）
make build               # 产出 bin/image2ascii 与 bin/video2ascii
```

## Usage

### image2ascii —— 图片转 ASCII

```bash
bin/image2ascii -i <image> [-w WIDTH] [-s color|text] [-b]
```

```bash
# 直接输出到终端（默认彩色，宽度为终端列数）
bin/image2ascii -i bin/rocket.jpg

# 指定宽度、文本风格
bin/image2ascii -i bin/rocket.jpg -w 80 -s text
```

### video2ascii —— 视频转 ASCII

三种模式：

**1. 播放（默认）**—— 转码的同时实时在终端播放（含音频）：

```bash
bin/video2ascii -i <video> [-w WIDTH] [-s color|text] [-b]
```

**2. 生成并保存** —— 把帧写入指定目录（`N.txt` + `audio.wav` + `meta.json`），不播放：

```bash
bin/video2ascii -i <video> -o <DIR> [-w WIDTH] [-s color|text] [-b]
```

**3. 播放已保存的帧目录**：

```bash
bin/video2ascii -p <frames_dir> [--loop]
```

```bash
# 例子：生成 + 保存，再重放
bin/video2ascii -i videos/demo.mp4 -w 80 -s text -o /tmp/ascii_frames
bin/video2ascii -p /tmp/ascii_frames
bin/video2ascii -p /tmp/ascii_frames --loop   # 循环播放
```

### 选项一览

| 选项 | 说明 |
|---|---|
| `-i, --image / --input` | 图片 / 视频路径（必填） |
| `-w, --width` | 输出宽度（字符数）；缺省为终端列数 |
| `-s, --style` | `color`（彩色，默认）或 `text`（纯字符） |
| `-b, --block` | 彩色模式下使用实心色块 |
| `-o, --output` | video：把帧保存到该目录（不播放） |
| `-p, --play_dir` | video：播放已生成的帧目录（忽略 `-i`/`-o`） |
| `--loop` | 配合 `-p` 循环播放 |
| `--version` | 打印版本信息 |

## Performance

内置基准测试，按阶段打印耗时并对比全流程（处理期间音频与视频并行、存在重叠）：

```bash
go run ./cmd/bench <video> [width] [style]
```

本机实测（Apple Silicon，10 核，Go 1.27）：

> 测试视频 `test/assets/fireworks.mp4`（1280×720，8.72 s，含音频），宽度 80（输出 80×22），25 fps

| 阶段 | 耗时 | 吞吐 |
|---|---|---|
| 音频提取 | 0.017 s | — |
| 视频帧提取 | 0.204 s | 1024 帧/s |
| ASCII 转换 | 0.017 s | 12072 帧/s |
| **全流程（Generate 总耗时）** | **0.219 s** | ≈ **40× 实时** |

- 8.72 s 的视频全流程生成只要 0.22 s，远快于播放速度，因此默认模式能边处理边实时播放。
- 生成 + 保存模式直接写目标目录、播放模式逐帧从磁盘读取，内存占用与视频时长无关——超长视频也不会因内存增长而卡顿。

> 性能受机器、视频分辨率与宽度影响：宽度越大、分辨率越高耗时越长。可按需自测上面的 bench 命令。

## Test

```bash
make test            # go test ./...；需 FFmpeg 开发库
go test -race ./...  # 含并行解码的竞态检测
```

## Version

```bash
bin/image2ascii --version
bin/video2ascii --version
```

- 版本：v0.1.0
- 项目：<https://github.com/Mark24Code/convert2ascii>
