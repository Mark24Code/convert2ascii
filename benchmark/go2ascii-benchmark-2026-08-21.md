# video2ascii Benchmark: Ruby vs Go

**日期**:2026-08-21(初始)→ 2026-08-21(重构后)
**输入**:`videos/demo.mp4`(1280×720 h264 + aac,时长 238.93s,57MB)
**参数**:`-w 80 -s text -o <dir>`(生成+保存完整管线:音频提取 → 帧切片 → ASCII 转换 → 保存)
**协议**:Ruby 与 Go 各跑 4 次(Go 重构后 3 次),顺序执行,隔离 `HOME`。取墙钟时间(`/usr/bin/time -p` 的 real;Ruby 的生成时间为 rb_benchmark.rb 进程内 TOTAL)。
**机器**:Apple M4,arm64,10 CPUs,Go 1.27.0,Ruby 4.0.1

## 结果(重构后)

| 指标 | Ruby | Go(基线) | **Go(重构后)** |
|---|---|---|---|
| 二进制 real(s) | 22.41* | 24.46 | **2.71** |
| generate real(s) | 11.73 | — | **2.69** |
| user CPU(s) | 81.91* | 95.91 | **9.95** |
| sys CPU(s) | 3.97* | 4.10 | **0.76** |
| 并行度 user/wall | 3.70 | 3.92 | **3.67** |
| 帧数 | 5972 | 5972 | **5972** |

\* Ruby 22.41s 为二进制 real(含 Ruby 启动 + CheckPackage 壳检查);rb_benchmark.rb 进程内 TOTAL generate = 11.73s 是纯生成时间。

**Go generate 2.69s vs Ruby 11.73s = 4.4× 快;Go 二进制 2.71s vs 基线 24.46s = 9× 快。**

## 分阶段对比(重构后,同机)

| 阶段 | Ruby | Go | 说明 |
|---|---|---|---|
| audio_extract | 1.903s | **0.174s** | Ruby 走 ffmpeg 子进程写 mp3;Go cgo 流式写 wav,与视频管线重叠 |
| frame_slice | 3.574s | **2.637s** | Ruby ffmpeg `-threads 9` 落 jpg;Go **4 段并行解码** + C `sws_scale` 直接缩到 80×22 |
| ascii_convert | 6.206s(962 帧/s) | **0.307s(19437 帧/s)** | Ruby fork 进程 + ImageMagick;Go goroutine 池 + `[]byte` 查表渲染 |
| TOTAL generate | 11.729s | **2.691s** | — |

## 重构内容与收益归因

1. **解码并行(主)**:Go 原实现单 goroutine 顺序解码 + 每帧全分辨率(1280×720 RGBA 3.7MB)导出 + Go 二次缩放,串行瓶颈喂不满并行转换。重构后:
   - **按 k 槽切 4 段**,各段独立 cgo context(关键帧 seek + 窗口过滤),goroutine 并行解码;
   - **缩放下沉 C `sws_scale`** 直接出 80×22 RGBA(7KB),去掉全分辨率缓冲与 Go `ApproxBiLinear` 二次缩放;
   - 输出帧先按 k 命名,最后一遍 rename 重编号成连续 `1..N.txt`(保 A/V 同步,不重不漏)。
2. **零拷贝**:`unsafe.Slice` 视图 C 内存,帧像素不再 `C.GoBytes` 拷贝;`Free` 随帧走,`-race` 下无泄漏。
3. **音频并行 + 流式**:DecodeAudioStream 与视频管线重叠,`WAVWriter` 流式落盘(占位 header→chunk→patch),内存有界。
4. **直接写目标目录**:砍掉 tmpdir→Save 拷贝往返(sys 0.76s vs 4.10s)。
5. **渲染微优化**:color 模式 `strconv.AppendInt` 拼 ANSI,免每字符 3× `Itoa` 分配。
6. **有界内存流式管线**:内存 ≈ (解码段 + channel + worker)× 7KB 恒定,不随视频时长增长(替代 Ruby 的磁盘临时文件)。

## 附带发现(重构前)

- **无 tty 鲁棒性**:Ruby 无 tty 时 `IO.console` 为 nil 会 `undefined method 'winsize'` 报错;Go 无此问题。
- **输出格式**:Go 用 `audio.wav`,Ruby 用 `audio.mp3`;meta.json 键一致(`step_duration`/`audio`/`frames_count`),双向可互播。
- **CLI 对齐**:Go 版改用标准库 flag(`--input/--image/--output`),去掉 Ruby 的 `--ouput` 拼写与 `-iURI` 粘合语法。

## 复现

```bash
cd go2ascii && go build -o bin/video2ascii ./cmd/video2ascii
bin/video2ascii -i ../videos/demo.mp4 -w 80 -s text -o /tmp/out   # ~2.7s,5972 帧
go run ./cmd/bench ../videos/demo.mp4 80 text                       # 分阶段计时
ruby benchmark/rb_benchmark.rb videos/demo.mp4 80                   # Ruby 对照
```
