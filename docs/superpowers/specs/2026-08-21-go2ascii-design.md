# go2ascii — Go 重写 convert2ascii 设计文档

日期:2026-08-21
状态:Approved(brainstorming 阶段已确认)

## 1. 背景与目标

`convert2ascii` 是一个 Ruby gem,把图片/视频在终端渲染为 ASCII art,提供 `image2ascii` 与 `video2ascii` 两个可执行文件。本项目在仓库根目录新建 `go2ascii/` 子目录,用 Go 按 Ruby 实现的思路重写一遍,**保持 CLI 参数兼容**。

### 用户已确认的关键决策

1. **图片解码用纯 Go**(标准库 image/jpeg、image/png、image/gif、image/bmp + golang.org/x/image/webp),去掉 ImageMagick 运行时依赖。
2. **视频帧提取用 cgo 绑定 FFmpeg**(libavformat/libavcodec/libavutil/libswscale),不再 spawn ffmpeg 二进制。
3. **音频全 Go 处理**:cgo 从视频解出音频流写 WAV,播放用 beep/oto,去掉 ffplay。
4. **必须用 goroutine 并行处理图片转换**(worker pool)。
5. **功能完整对齐 Ruby 版**:含终端播放(备用屏幕、A/V 同步、循环)。
6. 架构采用 **单一 Go module,库 + 二进制**(对应 Ruby 的 lib/ + exe/)。
7. cgo 绑定**手写薄封装**,不依赖 goav 等年久失修的第三方绑定。

## 2. 目录结构

```
go2ascii/
  go.mod                       # module github.com/Mark24Code/convert2ascii/go2ascii; go 1.22+
  README.md
  Makefile                     # build / test
  cmd/image2ascii/main.go      # 产出二进制 image2ascii
  cmd/video2ascii/main.go      # 产出二进制 video2ascii
  image2ascii.go               # 公开库:type Image2Ascii{...}
  video2ascii.go               # 公开库:type Video2Ascii{...}
  player.go                    # 公开库:type Player{...}
  internal/version/            # 版本常量 + --version 输出
  internal/imagelib/           # 纯 Go 图片解码 + 缩放
  internal/ffmpeg/             # cgo 薄封装:视频帧→RGBA;音频流→PCM
  internal/audio/              # WAV 编码 + beep/oto 播放
  internal/ansi/               # ANSI 转义 + 终端尺寸
  internal/tasker/             # goroutine worker pool(并行转帧)
  internal/player/             # A/V 同步播放引擎
  internal/cli/                # 表驱动参数解析(兼容 Ruby OptionParser)
```

## 3. CLI 兼容

### 3.1 参数解析

Go 标准库 `flag` 不支持 `-iURI`(短选项粘合值)与 `--image=URI` 混用,而 Ruby OptionParser 两者皆收。手写**表驱动解析器**(internal/cli,约 100 行),零依赖,精确复刻所有语法形式;错误文案复刻 Ruby,§3.2/§3.4 两处"参数缺失"文案为 Go 侧新增。

### 3.2 image2ascii

| 参数 | 行为 |
|---|---|
| `--version` | 打印版本块,退出 0 |
| `-i URI` / `-iURI` / `--image=URI` | 必填;缺失 → `Error: --image option is required.` 退出 1(Go 侧清理性改进:Ruby 此文案是死代码,无 `-i` 时实际 `URI.open(nil)` 崩溃) |
| `-w N` / `--width=N` | 整数;默认终端列数 |
| `-s S` / `--style=S` | `color` \| `text`;默认 color;非法 → `Error: --style option must be ["color" \| "text"].` 退出 1 |
| `-b` / `--block` | 布尔;默认 false |
| `-h` / `--help` | 打印 usage |

### 3.3 video2ascii 参数

video2ascii 与 image2ascii 共享 `-w`/`-s`/`-b`/`-h`/`--version`,另加:

| 参数 | 行为 |
|---|---|
| `-i URI` / `-iURI` / `--input=URI` | 必填(**注意:Ruby 用的是 `--input`,不是 `--image`**);缺失 → `Error: --input option is required.` 退出 1 |
| `-o DIR` / `--ouput=DIR` | 输出目录(**保留 Ruby 的 `--ouput` 拼写**;额外接受 `--output` 作别名) |
| `-p DIR` / `--play_dir=DIR` | 播放已生成的帧目录 |
| `--loop` | 循环播放 |

### 3.4 运行模式(与 Ruby 相同)

1. 有 `-p` → 加载帧目录 + meta.json 播放(`--loop` 生效),忽略 `-i`/`-o`。
2. 有 `-i` + `-o` → 生成并存帧,退出。
3. 只有 `-i` → 生成并播放。
4. 无 `-i` 无 `-p` → Go 版直接报 `Error: --input option is required.` 退出 1(**Go 侧清理性改进**:Ruby 版此处会从 ffmpeg 冒出难懂报错)。

> 说明:上面两处"参数缺失"的报错文案在 Ruby 里都是**死代码**(位于 `-i` 处理器内部,只有传了值才会触发),Go 侧是刻意新增的清理性改进,并非复刻 Ruby 行为。

### 3.5 --version 输出

复刻 Ruby 原文 4 行(版本/作者/邮箱/项目地址),版本值用 go2ascii 自己的常量(存 internal/version)。

## 4. 图片转换核心(Image2Ascii)

### 4.1 解码

- 支持格式:jpeg、png、gif(取首帧)、webp、bmp。
- 输入:本地路径或 http(s) URL(net/http,对齐 Ruby `URI.open`)。

### 4.2 缩放

```
outW = width
outH = round(origRows * width / origCols) / 2   // 整数除法,复刻 Ruby 两步 scale
```

用 `x/image/draw` CatmullRom。

> 实现注意:`round(origRows * width / origCols)` 必须先算浮点再 `math.Round`,再整数除 2;直接整数 `origRows*width/origCols` 会截断,可能与 Ruby 差 1 像素。

### 4.3 字符斜坡

与 Ruby **逐字符相同**:

```
.'`^\",:;Il!i><~+_-?][}{1)(|\\/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$
```

(Go 字符串中对 `"` 与 `\` 做转义。)

### 4.4 亮度与映射

- `brightness = 0.2126*r + 0.7152*g + 0.0722*b`,RGB 取值 0-255。
- `idx = floor(brightness / (255.0 / len(chars)))`,**clamp 到 len-1**。
  - 顺带修复 Ruby 隐患:纯白像素亮度 255 时 `char_index.floor == len`,Ruby `@chars[len]` 为 nil 会 TypeError 崩溃。

### 4.5 样式与换行

- text = 纯字符。
- color = 每字符 24-bit 前景 `\x1b[38;2;r;g;bm<char>\x1b[0m`。
- color_block = 每字符 24-bit 背景色块 `\x1b[48;2;r;g;bm \x1b[0m`(复刻 Rainbow gem)。
- 保留内部参数 `color: full|greyscale`。
- 换行:`col % (width-1) == 0 && col != 0` 时追加 `\n`。

### 4.6 公开 API

```go
type Image2Ascii struct { ... }
func New(opts Options) *Image2Ascii
func (a *Image2Ascii) Generate() *Image2Ascii   // 可变、返回自身可链式
func (a *Image2Ascii) String() string           // 返回 ascii_string
```

## 5. 视频管线(Video2Ascii)

### 5.1 cgo FFmpeg 封装(internal/ffmpeg)

- 帧提取:`avformat_open_input` → `av_find_best_stream` → `avcodec` 解码 → `swscale` → RGBA,按 `fps = 1/step_duration`(默认 0.04,≈25fps)采帧。
- 音频提取:解出音频流 → PCM → 写 `audio.wav`。
- **关键优化:不落 jpg 中间文件**。Ruby 是 帧→jpg→Image2Ascii→txt;Go 直接 帧→RGBA(内存)→ASCII→txt。省一次磁盘往返与一次编解码,最终目录布局不变(txt + audio + meta.json)。

### 5.2 meta.json

```json
{ "step_duration": 0.04, "audio": "audio.wav", "frames_count": N }
```

与 Ruby schema 键名一致,双向可读:Go 播放器用 beep 也能解 Ruby 生成的 audio.mp3;Ruby 的 ffplay 也能放 audio.wav。

### 5.3 并行转换(硬性要求)

- internal/tasker:**goroutine worker pool**,并发数 = `runtime.NumCPU()`。
  - **刻意偏离 Ruby**:Ruby MultiTasker 用 `nprocessors > 4 ? nprocessors-2 : 1`(benchmark 调过参)。Go 帧间相互独立、goroutine 开销远低于进程,`NumCPU()` 是清醒决策,输出不受影响。
- 每帧一个任务从 channel 取出,转换后写 `N.txt`。
- 进度条复刻 Ruby:`processing... xx.xx% (time: xx.xx s)`。

### 5.4 [info] 提示行

复刻 Ruby:`parsing audio... / done.`、`video slicing... / done.`(绿色)。

### 5.5 Save 与清理

- `Save(dir)` 建目录,拷 `N.txt + audio.wav + meta.json`,清理暂存区。
- 暂存区 = `~/.convert2ascii`(与 Ruby 同名同语义),操作结束后清理。

### 5.6 公开 API

```go
type Video2Ascii struct { ... }
func New(...)
func (v *Video2Ascii) Generate() *Video2Ascii
func (v *Video2Ascii) Save(dir string) error
func (v *Video2Ascii) Play(playLoop bool) error
func (v *Video2Ascii) AfterClean()
```

## 6. 终端播放(Player)

### 6.1 音频

beep + oto 全 Go 播放(wav/mp3 均可解),**替换 ffplay** —— 运行时零外部二进制。

### 6.2 A/V 同步

以音频播放位置为**主时钟**,对比 `frame_index * step_duration`,复刻 Ruby 容差逻辑:

- 落后音频 > 0.9s(SAFE_SLOW_DELTA)→ 跳帧;
- 超前音频 > 0.2s(SAFE_FAST_DELTA)→ 停顿;
- 否则推进 1 帧。

### 6.3 ANSI

复刻 Ruby Terminal 的全部转义:备用屏 `\x1b[?1049h/l`、隐藏/显示光标 `\x1b[?25l/h`、清屏 `\x1b[2J`、清缓冲 `\x1b[3J`、回退 `\x1b[A`×(帧数+1)。`full_screen` 按终端高度补齐/截断。

### 6.4 循环与中断

- `--loop` 帧号取模。
- Ctrl-C → 清理(关备用屏、显光标、清屏)并退出 0(`signal.NotifyContext`)。
- 帧流式:实时播放路径按帧流式喂入(channel,内存 O(1)),`-p` 逐帧从磁盘读;仅程序化传入 `Frames` slice 时一次性载入内存。
- 终端尺寸用 `golang.org/x/term` GetSize。

## 7. 构建、依赖、测试、错误处理

### 7.1 构建前提

cgo 需 FFmpeg 开发头文件(`brew install ffmpeg` + pkg-config);macOS 运行时依赖其 dylib。README 写清楚。

### 7.2 Go 依赖

- `golang.org/x/image`(webp、draw)
- `golang.org/x/term`(终端尺寸)
- `gopxl/beep` + `hajimehoshi/oto`(音频播放)
- cgo 直连 libavformat / libavcodec / libavutil / libswscale

### 7.3 测试

- 单元:字符映射(含纯白 clamp)、宽高计算、CLI 解析器(全部语法形式 + 错误文案)、WAV 编码往返。
- 集成:`ruby.jpg` 三种样式输出非空;`fireworks.mp4` 生成 + 保存。
- 播放测试加环境变量守护(CI 无音频设备)。
- 镜像 Ruby 测试结构(`test_01_image2ascii` / `test_02_video2ascii` 语义)。

### 7.4 错误处理

- 用户参数错误:退出 1 + 与 Ruby 相同文案(其中"参数缺失"类文案是 Go 侧新增,见 §3.2/§3.4 说明,其余如 `--style` 非法值文案与 Ruby 逐字相同)。
- 转换失败:红色 `[Error] ...` 文案复刻 Ruby。
- 库 API 返回 error,二进制层负责退出码。

## 8. 非目标(本期不做)

- 播放控制按键(pause/next/prev/exit)—— Ruby TODO 中同样未做。
- ~~帧流式读取~~ —— 已在 realtime 分支落地:播放路径逐帧流式解码渲染 + 自适应预缓存 + 音频实时流式直送声卡(见 §6.4、README)。
- Windows 支持 —— Ruby 同样不支持。
- 与 Ruby 逐字节相同的 ASCII 输出 —— 解码器/缩放器不同,保证功能等价而非字节相同。

## 9. 验收标准

1. `go build ./cmd/image2ascii ./cmd/video2ascii` 产出两个二进制。
2. `image2ascii -i <img>` 三种样式(color/text/color+block)输出非空、可渲染。
3. `video2ascii -i <video> -o <dir>` 生成 txt + audio.wav + meta.json。
4. `video2ascii -p <dir>` 可播放,`--loop` 循环。
5. `video2ascii -p <dir>` 能播放 Ruby 生成的帧目录(audio.mp3)。
6. `go test ./...` 全绿。
