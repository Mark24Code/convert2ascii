// Command bench measures the video2ascii pipeline phases, mirroring
// benchmark/rb_benchmark.rb for the Go implementation.
//
// Usage:
//
//	go run ./cmd/bench <video> [width] [style]
//
// width defaults to 80, style to "text" (matching the production comparison).
package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Mark24Code/convert2ascii/go2ascii"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/audio"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/ffmpeg"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/imagelib"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/bench <video> [width] [style]\n")
}

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}
	video := args[0]
	width := 80
	if len(args) > 1 {
		fmt.Sscanf(args[1], "%d", &width)
	}
	style := "text"
	if len(args) > 2 {
		style = args[2]
	}
	step := convert2ascii.DefaultStepDuration

	info, err := ffmpeg.Probe(video)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !info.HasVideo {
		fmt.Fprintln(os.Stderr, "no video stream")
		os.Exit(1)
	}
	tw, th := imagelib.AspectSizeWH(info.Width, info.Height, width)
	segs := runtime.NumCPU()
	if segs > 4 {
		segs = 4
	}

	fmt.Println("============================================================")
	fmt.Println("go2ascii (Go) benchmark")
	fmt.Println("============================================================")
	fmt.Printf("machine          : %s %s, %d CPUs (GOMAXPROCS=%d)\n",
		runtime.GOARCH, runtime.GOOS, runtime.NumCPU(), runtime.GOMAXPROCS(0))
	fmt.Printf("go               : %s\n", runtime.Version())
	fmt.Printf("video            : %s\n", video)
	fmt.Printf("resolution       : %dx%d\n", info.Width, info.Height)
	fmt.Printf("duration         : %.2f s   audio: %v\n", info.Duration, info.HasAudio)
	fmt.Printf("width            : %d\n", width)
	fmt.Printf("step_duration    : %.2f s (%.0f fps)\n", step, 1.0/step)
	fmt.Printf("decode segments  : %d\n", segs)
	fmt.Printf("target size      : %dx%d\n", tw, th)
	fmt.Println()

	// Phase 1: audio extraction (streamed to a temp wav, mirroring production).
	t0 := time.Now()
	sampleRate, channels := 0, 0
	aw, err := os.CreateTemp("", "bench-audio-*.wav")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if info.HasAudio {
		ww, err := audio.NewWAVWriter(aw, info.AudioSampleRate, info.AudioChannels)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sampleRate, channels, err = ffmpeg.DecodeAudioStream(video, func(pcm []byte) error {
			_, err := ww.Write(pcm)
			return err
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := ww.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	aw.Close()
	os.Remove(aw.Name())
	audioDur := time.Since(t0)
	fmt.Printf("audio_extract     : %8.3f s  (%d Hz, %d ch)\n", audioDur.Seconds(), sampleRate, channels)

	// Phase 2: frame slicing (parallel decode + C downscale). Frames are kept
	// (not freed in the yield) so ascii_convert can reuse them without re-decoding.
	t0 = time.Now()
	opts := imagelib.Options{Width: width, Style: imagelib.Style(style), Color: imagelib.ColorFull}
	var mu sync.Mutex
	frames := make([]ffmpeg.Frame, 0, 6200)
	if err := ffmpeg.DecodeFramesParallel(video, step, segs, tw, th, func(fr ffmpeg.Frame) error {
		mu.Lock()
		frames = append(frames, fr)
		mu.Unlock()
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sliceDur := time.Since(t0)
	fmt.Printf("frame_slice       : %8.3f s  (%d frames, %.1f frames/s)\n",
		sliceDur.Seconds(), len(frames), float64(len(frames))/sliceDur.Seconds())

	// Phase 3: ascii conversion over the stored frames + write temp txt files.
	t0 = time.Now()
	outDir, err := os.MkdirTemp("", "bench-conv-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, fr := range frames {
		s := imagelib.RenderRGBA(&image.RGBA{
			Pix:    fr.Pixels,
			Stride: fr.Width * 4,
			Rect:   image.Rect(0, 0, fr.Width, fr.Height),
		}, opts)
		if err := os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%d.txt", fr.K+1)), []byte(s+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	convDur := time.Since(t0)
	fmt.Printf("ascii_convert     : %8.3f s  (%.1f frames/s)\n",
		convDur.Seconds(), float64(len(frames))/convDur.Seconds())
	for _, fr := range frames {
		if fr.Free != nil {
			fr.Free()
		}
	}
	os.RemoveAll(outDir)

	// Phase 4: TOTAL = full Generate through the public API, stdout silenced.
	totalDir, err := os.MkdirTemp("", "bench-total-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	oldStdout := os.Stdout
	os.Stdout = devnull
	t0 = time.Now()
	v := convert2ascii.NewVideo2Ascii(convert2ascii.VideoOptions{
		URI:    video,
		Width:  width,
		Style:  style,
		Output: totalDir,
	})
	genErr := v.Generate()
	totalDur := time.Since(t0)
	os.Stdout = oldStdout
	devnull.Close()
	if genErr != nil {
		fmt.Fprintln(os.Stderr, genErr)
		os.Exit(1)
	}
	txts, _ := filepath.Glob(filepath.Join(totalDir, "*.txt"))
	os.RemoveAll(totalDir)
	fmt.Printf("TOTAL generate    : %8.3f s\n", totalDur.Seconds())
	fmt.Printf("frame count       : %d (expected ~%.0f)\n", len(txts), info.Duration/step)
	overlap := totalDur.Seconds() - audioDur.Seconds() - sliceDur.Seconds() - convDur.Seconds()
	fmt.Printf("overlap total-(audio+slice+conv): %8.3f s  (negative = pipelined overlap)\n", overlap)
	fmt.Println()
	fmt.Println("(isolated phases do not sum to TOTAL; they demonstrate pipeline overlap)")
}
