// Package player renders a sequence of ASCII frames in the terminal with
// self-adaptive A/V sync, mirroring the Ruby TerminalPlayer. Rendering uses
// tcell double buffering: each frame is drawn into a cell grid and Show()
// paints only the cells that changed, so playback is flicker-free.
package player

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/term"

	"github.com/Mark24Code/convert2ascii/go2ascii/internal/audio"
)

// SAFE_SLOW_DELTA / SAFE_FAST_DELTA mirror the Ruby tolerances.
const (
	SAFE_SLOW_DELTA = 0.9 // seconds: video lags audio beyond this -> skip frames
	SAFE_FAST_DELTA = 0.2 // seconds: video leads audio beyond this -> pause
)

// AudioClock is the minimal interface the player needs from the audio layer,
// so it can run without a sound device.
type AudioClock interface {
	Play()
	Position() float64
	Close() error
}

// wallClock is the wall-clock fallback for videos without audio.
func wallClock(start time.Time) float64 { return time.Since(start).Seconds() }

// Options configure a playback session. Exactly one frame source is used:
// Frames (all in memory), FrameStream (live pull), or FrameDir (lazy disk).
type Options struct {
	Frames       []string
	FrameStream  <-chan string
	FrameDir     string
	AudioPath    string
	AudioStream  *audio.StreamParams // live PCM stream (play path); takes precedence over AudioPath
	StepDuration float64
	PlayLoop     bool
	Debug        bool
}

// frameSource yields frames in order. next returns ok=false only at the true
// end of a non-looping stream (looping sources wrap forever). count returns the
// total frame count when known, or -1 while unknown (a live stream before its
// channel closes); it drives the loop-mode videoTime wrap.
type frameSource interface {
	next() (string, bool)
	count() int
}

// sliceSource serves an in-memory frame slice (the legacy player input).
type sliceSource struct {
	frames []string
	loop   bool
	pos    int
	ended  bool
}

func (s *sliceSource) next() (string, bool) {
	if s.ended || len(s.frames) == 0 {
		return "", false
	}
	content := s.frames[s.pos]
	s.pos++
	if s.pos >= len(s.frames) {
		if s.loop {
			s.pos = 0
		} else {
			s.ended = true
		}
	}
	return content, true
}

func (s *sliceSource) count() int { return len(s.frames) }

// streamSource pulls frames from a live channel. Non-loop mode stays O(1)
// memory; loop mode buffers every pulled frame to replay after the channel
// closes (documented trade-off for live looping).
type streamSource struct {
	ch    <-chan string
	loop  bool
	buf   []string
	pos   int
	ended bool // channel closed; replaying buf when loop
}

func (s *streamSource) next() (string, bool) {
	if !s.ended {
		content, ok := <-s.ch
		if ok {
			if s.loop {
				s.buf = append(s.buf, content)
			}
			return content, true
		}
		s.ended = true
		if !s.loop || len(s.buf) == 0 {
			return "", false
		}
		s.pos = 0
	}
	content := s.buf[s.pos]
	s.pos++
	if s.pos >= len(s.buf) {
		s.pos = 0
	}
	return content, true
}

func (s *streamSource) count() int {
	if s.ended {
		return len(s.buf)
	}
	return -1
}

// dirSource reads a saved frames directory lazily, one file at a time, so
// replay memory stays O(1) regardless of video length.
type dirSource struct {
	paths []string
	loop  bool
	pos   int
	ended bool
}

func newDirSource(dir string) (*dirSource, error) {
	txts, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		return nil, err
	}
	sort.Slice(txts, func(i, j int) bool {
		ni, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(txts[i]), ".txt"))
		nj, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(txts[j]), ".txt"))
		return ni < nj
	})
	return &dirSource{paths: txts}, nil
}

func (s *dirSource) next() (string, bool) {
	if s.ended || len(s.paths) == 0 {
		return "", false
	}
	data, err := os.ReadFile(s.paths[s.pos])
	if err != nil {
		s.ended = true
		return "", false
	}
	s.pos++
	if s.pos >= len(s.paths) {
		if s.loop {
			s.pos = 0
		} else {
			s.ended = true
		}
	}
	return string(data), true
}

func (s *dirSource) count() int { return len(s.paths) }

func (o Options) source() (frameSource, error) {
	switch {
	case o.FrameDir != "":
		return newDirSource(o.FrameDir)
	case o.FrameStream != nil:
		return &streamSource{ch: o.FrameStream, loop: o.PlayLoop}, nil
	default:
		return &sliceSource{frames: o.Frames, loop: o.PlayLoop}, nil
	}
}

// Pre-buffer tuning for the live streaming (FrameStream) path. Playback starts
// once the adaptive target is banked (or maxStartupWait elapses), so decode and
// render jitter does not show up as startup stutter.
const (
	minPrebufferFrames  = 4               // smallest cache, even at very fast production
	maxPrebufferSeconds = 2.5             // cache capped at this many seconds of playback
	maxStartupWait      = 3 * time.Second // give up filling the cache after this
)

// targetPrebufferFrames returns how many frames to bank before starting
// playback, given the measured production interval produceInterval (seconds per
// produced frame) and the playback step. Production slower than realtime
// (produceInterval > step) banks more so a slow pipeline has a cushion at
// startup; fast production banks the minimum so playback starts sooner.
// Clamped to [minPrebufferFrames, maxPrebufferSeconds of playback].
func targetPrebufferFrames(produceInterval, step float64) int {
	maxFrames := max(minPrebufferFrames, int(maxPrebufferSeconds/step))
	t := int(produceInterval / step * float64(maxFrames))
	return min(maxFrames, max(minPrebufferFrames, t))
}

// prebufferSource serves buffered frames first, then delegates to src. Play()
// wraps a live frameSource with it to pre-fill a cache before the display loop
// starts; the main loop and loop-mode wrapping (count()) are untouched.
type prebufferSource struct {
	src frameSource
	buf []string
}

func (p *prebufferSource) next() (string, bool) {
	if len(p.buf) > 0 {
		s := p.buf[0]
		p.buf = p.buf[1:]
		return s, true
	}
	return p.src.next()
}

func (p *prebufferSource) count() int { return p.src.count() }

// fillPrebuffer pulls frames from src into pb.buf until the adaptive target is
// met, maxStartupWait elapses, or the source ends. Production speed is measured
// from frames after the first (cold-start) frame and smoothed with an EMA. The
// deadline is checked between pulls, so the real wait can overshoot by up to
// one production interval.
func fillPrebuffer(pb *prebufferSource, step float64) {
	deadline := time.Now().Add(maxStartupWait)
	var ema float64 // seconds per produced frame

	first, ok := pb.src.next()
	if !ok {
		return
	}
	pb.buf = append(pb.buf, first)
	last := time.Now()

	for len(pb.buf) < minPrebufferFrames {
		if time.Now().After(deadline) {
			return
		}
		nxt, ok := pb.src.next()
		if !ok {
			return
		}
		pb.buf = append(pb.buf, nxt)
		if dt := time.Since(last).Seconds(); dt > 0 {
			if ema == 0 {
				ema = dt
			} else {
				ema = ema*0.5 + dt*0.5
			}
		}
		last = time.Now()
	}

	target := targetPrebufferFrames(ema, step)
	for len(pb.buf) < target {
		if time.Now().After(deadline) {
			return
		}
		nxt, ok := pb.src.next()
		if !ok {
			return
		}
		pb.buf = append(pb.buf, nxt)
		if dt := time.Since(last).Seconds(); dt > 0 {
			ema = ema*0.5 + dt*0.5
			target = targetPrebufferFrames(ema, step)
		}
		last = time.Now()
	}
}

// Player plays frames with optional audio.
type Player struct {
	opts Options
}

// New builds a Player.
func New(o Options) *Player { return &Player{opts: o} }

// Play runs the playback loop. It blocks until playback ends or is interrupted.
func (p *Player) Play() error {
	o := p.opts
	src, err := o.source()
	if err != nil {
		return err
	}

	var clock AudioClock
	if o.AudioStream != nil {
		ap, err := audio.NewStreamPlayer(*o.AudioStream, o.PlayLoop)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] audio unavailable, playing without audio: %v\n", err)
		} else {
			clock = ap
		}
	} else if o.AudioPath != "" {
		ap, err := audio.NewPlayer(o.AudioPath, o.PlayLoop)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] audio unavailable, playing without audio: %v\n", err)
		} else {
			clock = ap
		}
	}

	// First frame; the source pulls it, so live playback can begin as soon as
	// the producer emits the first rendered frame (the pre-buffer below then
	// banks more frames before the first draw).
	current, ok := src.next()
	if !ok {
		return errors.New("[Error] frame's length must be >= 0")
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("play requires an interactive terminal (stdout is not a tty)")
	}

	step := o.StepDuration
	if step <= 0 {
		step = 0.04
	}

	// Live streams: pre-fill a cache so decode/render jitter does not show as
	// startup stutter. Playback starts as soon as the adaptive target is met or
	// the bounded wait elapses; the main loop below is unchanged.
	if o.FrameStream != nil {
		pb := &prebufferSource{src: src}
		fillPrebuffer(pb, step)
		src = pb
	}

	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	fmt.Print("\x1b[?1049h\x1b[?25l") // enter alternate screen, hide cursor
	screen.Clear()

	var cleanOnce sync.Once
	cleanup := func() {
		cleanOnce.Do(func() {
			screen.ShowCursor(0, 0)
			screen.Fini()
			fmt.Print("\x1b[?25h\x1b[?1049l")
			if clock != nil {
				_ = clock.Close()
			}
		})
	}

	// External SIGINT (e.g. kill -INT). Ctrl-C on the keyboard arrives as a
	// KeyCtrlC event instead (tcell runs in raw mode, ISIG disabled).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	done := make(chan struct{})
	defer func() {
		close(done)
		cleanup()
	}()
	go func() {
		select {
		case <-sig:
			cleanup()
			os.Exit(0)
		case <-done:
		}
	}()

	// Ctrl-C / Esc / resize via tcell events.
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				return // Fini closed the event source
			}
			switch e := ev.(type) {
			case *tcell.EventKey:
				if e.Key() == tcell.KeyCtrlC || e.Key() == tcell.KeyEscape {
					cleanup()
					os.Exit(0)
				}
			case *tcell.EventResize:
				screen.Sync()
			}
		}
	}()

	if clock != nil {
		clock.Play()
	}

	start := time.Now()
	frameIndex := 0

	for {
		renderFrame(screen, current)
		screen.Show()
		time.Sleep(time.Duration(step * float64(time.Second)))

		var actual float64
		if clock != nil {
			actual = clock.Position()
		} else {
			actual = wallClock(start)
		}
		videoTime := float64(frameIndex) * step
		if o.PlayLoop {
			if n := src.count(); n > 0 {
				videoTime = float64(frameIndex%n) * step
			}
		}
		offset := actual - videoTime
		advance := syncOffset(offset, step, SAFE_SLOW_DELTA, SAFE_FAST_DELTA)

		// Advance the source by `advance` frames (skip when video lags audio;
		// pause when advance == 0).
		for j := 0; j < advance; j++ {
			nxt, ok := src.next()
			if !ok {
				return nil // non-looping stream exhausted
			}
			current = nxt
			frameIndex++
		}

		if o.Debug {
			fmt.Printf("\nRealTime: %.2f s\nFrameTime: %.2f s\nCurrentFrame: %d\nOffset: %.2f s\nAdvance: %d\n",
				time.Since(start).Seconds(), videoTime, frameIndex, offset, advance)
		}
	}
}

// renderFrame parses an ANSI-colored frame string into the tcell cell grid,
// clipped to the terminal. Cells the frame does not touch stay blank.
func renderFrame(screen tcell.Screen, content string) {
	cols, rows := screen.Size()
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	x, y := 0, 0
	style := tcell.StyleDefault
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '\x1b':
			var next int
			style, next = parseAnsi(content, i, style)
			i = next - 1 // the loop's i++ lands just past the sequence
		case '\n':
			y++
			x = 0
		default:
			if y < rows && x < cols {
				screen.SetContent(x, y, rune(content[i]), nil, style)
			}
			x++
		}
	}
}

// parseAnsi consumes one ANSI escape sequence starting at content[i]=='\x1b'
// and returns the updated style and the index just past the sequence. Only the
// sequences the renderer emits are handled: "0" reset, "38;2;R;G;B" foreground
// and "48;2;R;G;B" background (24-bit color). Others are skipped.
func parseAnsi(content string, i int, style tcell.Style) (tcell.Style, int) {
	if i+1 >= len(content) || content[i+1] != '[' {
		return style, i + 1
	}
	j := i + 2
	for j < len(content) && !isAnsiFinal(content[j]) {
		j++
	}
	if j >= len(content) {
		return style, len(content)
	}
	if content[j] == 'm' {
		parts := strings.Split(content[i+2:j], ";")
		if len(parts) == 5 && (parts[0] == "38" || parts[0] == "48") && parts[1] == "2" {
			r, _ := strconv.Atoi(parts[2])
			g, _ := strconv.Atoi(parts[3])
			b, _ := strconv.Atoi(parts[4])
			col := tcell.NewRGBColor(int32(r), int32(g), int32(b))
			if parts[0] == "38" {
				style = style.Foreground(col)
			} else {
				style = style.Background(col)
			}
		} else if parts[0] == "0" {
			style = tcell.StyleDefault
		}
	}
	return style, j + 1
}

func isAnsiFinal(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// syncOffset computes how many frames to advance given the offset between the
// audio clock and the video clock. Mirrors the Ruby self_adaption_frame_play.
func syncOffset(offset float64, step, slowDelta, fastDelta float64) int {
	switch {
	case offset > slowDelta:
		return int(offset / step)
	case offset < -fastDelta:
		return 0
	default:
		return 1
	}
}
