# go2ascii

Go 版 convert2ascii:把图片/视频在终端渲染为 ASCII art。CLI 与 Ruby 版兼容(参数语法用 Go 标准库 `flag`,主参数 `--input/--image/--output` 与 Ruby 语义一致,不再保留 `--ouput` 拼写)。

## Build

需要 Go ≥ 1.25、FFmpeg ≥ 6 开发库(pkg-config),cgo 编译:

```bash
brew install ffmpeg      # macOS(含开发头文件)
make build               # 产出 bin/image2ascii 与 bin/video2ascii
```

## Usage

```bash
bin/image2ascii -i <image> [-w WIDTH] [-s color|text] [-b]
bin/video2ascii -i <video> [-w WIDTH] [-s color|text] [-b] [-o DIR]
bin/video2ascii -p <frames_dir> [--loop]     # 播放已生成的帧目录
```

示例(生成 + 保存,直接写目标目录):

```bash
bin/video2ascii -i videos/demo.mp4 -w 80 -s text -o /tmp/ascii_frames
bin/video2ascii -p /tmp/ascii_frames        # 重放(逐帧从磁盘读,内存 O(1))
```

## 架构要点

- **纯 Go + cgo FFmpeg**:图片解码纯 Go(`image/…` + `x/image`);视频帧 cgo 直连 libavformat/libavcodec/libswscale,不 spawn ffmpeg 二进制、不落 jpg 中间文件。
- **分段并行解码**:按 k 槽切段(默认 4),各段独立 cgo context 并行解码;`sws_scale` 直接缩到目标尺寸(SIMD),帧像素 `unsafe.Slice` 零拷贝视图。
- **有界内存流式管线**:内存 ≈ 常量(每帧 ~7KB),超大视频不爆内存;`-o` 直接写目标目录,播放路径流式喂播放器。
- **无闪烁播放**:播放器用 [tcell](https://github.com/gdamore/tcell) 双缓冲渲染 —— 每帧写入 cell 网格,`Show()` 只刷新变化的格子(逐行光标定位),替代逐帧整屏清除,画面持续流畅;支持 24-bit 前景/背景色。
- **音频**:cgo 流式解码 PCM → WAV(流式写盘,内存有界),与视频管线并行。

## Test

```bash
make test          # go test ./...;需 ffmpeg 开发库
go test -race ./...   # 含并行解码/零拷贝的竞态检测
```
