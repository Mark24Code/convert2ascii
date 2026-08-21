package convert2ascii

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/Mark24Code/convert2ascii/go2ascii/internal/ansi"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/audio"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/ffmpeg"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/imagelib"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/tasker"
)

// DefaultStepDuration mirrors the Ruby DEFAULT_STEP_DURATION (≈25 fps).
const DefaultStepDuration = 0.04

// VideoOptions configures a Video2Ascii conversion.
type VideoOptions struct {
	URI           string
	Width         int
	Style         string
	Color         string
	ColorBlock    bool
	StepDuration  float64
	Output        string // target dir for frames; empty = stream to player
	Threads       int    // render pool; 0 = NumCPU
	DecodeThreads int    // parallel decode segments; 0 = auto min(4, Threads)
}

// Video2Ascii extracts a video's frames as ASCII. With Output set it writes
// N.txt + audio.wav + meta.json directly into that directory (no staging
// copy); otherwise it streams rendered frames to the player (see Play).
type Video2Ascii struct {
	URI           string
	Width         int
	Style         string
	Color         string
	ColorBlock    bool
	StepDuration  float64
	Threads       int
	DecodeThreads int
	Output        string

	renderOpts imagelib.Options
	audioPath  string

	// play-mode state (set by Generate, consumed by Play)
	framesCh    chan string
	playerErrCh chan error
	cancel      context.CancelFunc
}

// NewVideo2Ascii builds a Video2Ascii. Width 0 means terminal columns.
func NewVideo2Ascii(opts VideoOptions) *Video2Ascii {
	width := opts.Width
	if width <= 0 {
		_, cols := ansi.Size()
		width = cols
	}
	step := opts.StepDuration
	if step <= 0 {
		step = DefaultStepDuration
	}
	threads := opts.Threads
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	return &Video2Ascii{
		URI:           opts.URI,
		Width:         width,
		Style:         opts.Style,
		Color:         opts.Color,
		ColorBlock:    opts.ColorBlock,
		StepDuration:  step,
		Threads:       threads,
		DecodeThreads: opts.DecodeThreads,
		Output:        opts.Output,
	}
}

func (v *Video2Ascii) decodeThreads() int {
	n := v.DecodeThreads
	if n <= 0 {
		n = 4
	}
	if n > v.Threads {
		n = v.Threads
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (v *Video2Ascii) frameRGBA(fr ffmpeg.Frame) *image.RGBA {
	return &image.RGBA{
		Pix:    fr.Pixels,
		Stride: fr.Width * 4,
		Rect:   image.Rect(0, 0, fr.Width, fr.Height),
	}
}

// Generate converts the video. With Output set it writes N.txt, audio.wav and
// meta.json directly into Output. With Output empty it starts streaming
// rendered frames to the player and returns once audio is ready; call Play to
// consume the stream.
func (v *Video2Ascii) Generate() error {
	v.renderOpts = imagelib.Options{
		Width:      v.Width,
		Style:      imagelib.Style(v.Style),
		Color:      imagelib.ColorMode(v.Color),
		ColorBlock: v.ColorBlock,
	}
	v.audioPath = ""

	info, err := ffmpeg.Probe(v.URI)
	if err != nil {
		return err
	}
	if !info.HasVideo || info.Width <= 0 {
		return fmt.Errorf("no video stream in %s", v.URI)
	}
	tw, th := imagelib.AspectSizeWH(info.Width, info.Height, v.Width)
	if tw < 2 {
		tw = 2
	}
	if th < 1 {
		th = 1
	}
	total := 0
	if info.Duration > 0 {
		total = int(info.Duration/v.StepDuration) + 1
	}

	if v.Output != "" {
		if err := os.MkdirAll(v.Output, 0o755); err != nil {
			return err
		}
		return v.generateSave(info, tw, th, total)
	}
	return v.generatePlay(info, tw, th)
}

// generateSave writes all frames + audio + meta.json directly into v.Output.
// Audio extraction runs concurrently with the decode+render pass. Frames are
// decoded in parallel segments and rendered by a worker pool; each is written
// under its k-slot name (gaps possible), then a final rename pass renumbers
// them to contiguous 1..N.txt so playback A/V sync stays correct.
func (v *Video2Ascii) generateSave(info ffmpeg.ProbeInfo, tw, th, total int) error {
	// Ruby's save removed the target dir first; match it so stale files never
	// mix with a fresh conversion.
	if err := os.RemoveAll(v.Output); err != nil {
		return err
	}
	if err := os.MkdirAll(v.Output, 0o755); err != nil {
		return err
	}

	// Audio: stream PCM to Output/audio.wav while the video pass runs.
	var wf *os.File
	if info.HasAudio {
		f, err := os.Create(filepath.Join(v.Output, "audio.wav"))
		if err != nil {
			return err
		}
		wf = f
		v.audioPath = f.Name()
	}
	audioCh := make(chan error, 1)
	go func() {
		if wf == nil {
			audioCh <- nil
			return
		}
		audioCh <- v.extractAudio(wf, info)
	}()

	// Video: decode in parallel segments, render in a worker pool, write each
	// frame straight to the output directory under its k-slot name.
	frameCh := make(chan ffmpeg.Frame, v.Threads*2)
	var decErr error
	go func() {
		defer close(frameCh)
		decErr = ffmpeg.DecodeFramesParallel(v.URI, v.StepDuration, v.decodeThreads(), tw, th, func(fr ffmpeg.Frame) error {
			frameCh <- fr
			return nil
		})
	}()
	convErr := tasker.RunParallel(v.Threads, total, frameCh, func(fr ffmpeg.Frame) error {
		if fr.Free != nil {
			defer fr.Free()
		}
		name := strconv.Itoa(fr.K+1) + ".txt"
		s := imagelib.RenderRGBA(v.frameRGBA(fr), v.renderOpts)
		return os.WriteFile(filepath.Join(v.Output, name), []byte(s+"\n"), 0o644)
	})
	if decErr != nil {
		return fmt.Errorf("decode frames: %w", decErr)
	}
	if convErr != nil {
		return fmt.Errorf("convert frames: %w", convErr)
	}
	if err := <-audioCh; err != nil {
		return fmt.Errorf("extract audio: %w", err)
	}
	if wf != nil {
		wf.Close()
	}

	n, err := renumberFrames(v.Output)
	if err != nil {
		return fmt.Errorf("renumber frames: %w", err)
	}

	meta := map[string]any{
		"step_duration": v.StepDuration,
		"audio":         nil,
		"frames_count":  n,
	}
	if v.audioPath != "" {
		meta["audio"] = filepath.Base(v.audioPath)
	}
	metaRaw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.Output, "meta.json"), metaRaw, 0o644)
}

// renumberFrames renames k-slot-named frame files (N.txt, gaps possible) to
// contiguous 1..N.txt in place. Sorted ascending, each file's target is always
// <= its source name, and earlier files are already renamed away before a
// target is claimed, so the pass is collision-free. Returns the frame count.
func renumberFrames(dir string) (int, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return 0, err
	}
	sort.Slice(paths, func(i, j int) bool {
		ni, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(paths[i]), ".txt"))
		nj, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(paths[j]), ".txt"))
		return ni < nj
	})
	for i, p := range paths {
		dst := filepath.Join(dir, strconv.Itoa(i+1)+".txt")
		if p == dst {
			continue
		}
		if err := os.Rename(p, dst); err != nil {
			return 0, err
		}
	}
	return len(paths), nil
}

// generatePlay stages audio to a temp wav and starts a single-segment decode +
// render pipeline streaming frames to the player. Production is far faster
// than the ~25 fps the player consumes, so a sequential pipeline is both
// ordered and O(1) memory.
func (v *Video2Ascii) generatePlay(info ffmpeg.ProbeInfo, tw, th int) error {
	var wf *os.File
	if info.HasAudio {
		f, err := os.CreateTemp("", "convert2ascii-audio-*.wav")
		if err != nil {
			return err
		}
		wf = f
		v.audioPath = f.Name()
	}
	audioCh := make(chan error, 1)
	go func() {
		if wf == nil {
			audioCh <- nil
			return
		}
		audioCh <- v.extractAudio(wf, info)
	}()

	v.framesCh = make(chan string, 4)
	v.playerErrCh = make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	go func() {
		defer close(v.framesCh)
		err := ffmpeg.DecodeFramesParallel(v.URI, v.StepDuration, 1, tw, th, func(fr ffmpeg.Frame) error {
			if fr.Free != nil {
				defer fr.Free()
			}
			s := imagelib.RenderRGBA(v.frameRGBA(fr), v.renderOpts)
			select {
			case v.framesCh <- s + "\n":
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		v.playerErrCh <- err
	}()

	// Join audio so Play can use it immediately.
	if err := <-audioCh; err != nil {
		cancel()
		<-v.playerErrCh // wait for the pipeline to stop
		if v.audioPath != "" {
			_ = os.Remove(v.audioPath)
			v.audioPath = ""
		}
		return fmt.Errorf("extract audio: %w", err)
	}
	if wf != nil {
		wf.Close()
	}
	return nil
}

func (v *Video2Ascii) extractAudio(wf *os.File, info ffmpeg.ProbeInfo) error {
	ww, err := audio.NewWAVWriter(wf, info.AudioSampleRate, info.AudioChannels)
	if err != nil {
		return err
	}
	if _, _, err := ffmpeg.DecodeAudioStream(v.URI, func(pcm []byte) error {
		_, err := ww.Write(pcm)
		return err
	}); err != nil {
		return err
	}
	return ww.Close()
}

// Play streams the frames produced by Generate to the terminal player. Only
// valid in play mode (Output empty). Blocks until playback ends or is
// interrupted.
func (v *Video2Ascii) Play(playLoop bool) error {
	if v.framesCh == nil {
		return fmt.Errorf("nothing to play: call Generate first")
	}
	defer func() {
		if v.cancel != nil {
			v.cancel()
			<-v.playerErrCh // wait for the pipeline to stop
		}
		if v.audioPath != "" {
			_ = os.Remove(v.audioPath)
		}
	}()
	p := &Player{
		FrameStream:  v.framesCh,
		AudioPath:    v.audioPath,
		StepDuration: v.StepDuration,
		PlayLoop:     playLoop,
	}
	return p.Play()
}
