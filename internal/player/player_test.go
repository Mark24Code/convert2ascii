package player

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestNoFrames(t *testing.T) {
	p := &Player{}
	if err := p.Play(); err == nil {
		t.Fatal("expected error for empty frames")
	}
}

func TestSyncOffset(t *testing.T) {
	// offset ahead of audio (video lags) -> skip several frames
	if o := syncOffset(1.0, 0.1, 0.9, 0.2); o != 10 {
		t.Fatalf("slow offset = %d", o)
	}
	// offset behind audio (video runs ahead) -> pause
	if o := syncOffset(-0.3, 0.1, 0.9, 0.2); o != 0 {
		t.Fatalf("fast offset = %d", o)
	}
	// in tolerance -> advance one
	if o := syncOffset(0.1, 0.1, 0.9, 0.2); o != 1 {
		t.Fatalf("normal offset = %d", o)
	}
}

func TestSyncOffsetBoundaries(t *testing.T) {
	if o := syncOffset(0.9, 0.1, 0.9, 0.2); o != 1 {
		t.Fatalf("offset == slowDelta must advance 1, got %d", o)
	}
	if o := syncOffset(-0.2, 0.1, 0.9, 0.2); o != 1 {
		t.Fatalf("offset == -fastDelta must advance 1, got %d", o)
	}
}

func TestWallClock(t *testing.T) {
	start := time.Now()
	time.Sleep(2 * time.Millisecond)
	if wallClock(start) <= 0 {
		t.Fatal("wall clock should be positive")
	}
}

func TestParseAnsiForeground(t *testing.T) {
	seq := "\x1b[38;2;10;20;30m"
	s, n := parseAnsi(seq, 0, tcell.StyleDefault)
	if n != len(seq) {
		t.Fatalf("consumed %d, want %d", n, len(seq))
	}
	fg, _, _ := s.Decompose()
	if fg != tcell.NewRGBColor(10, 20, 30) {
		t.Fatalf("fg = %v", fg)
	}
}

func TestParseAnsiBackground(t *testing.T) {
	seq := "\x1b[48;2;1;2;3m"
	s, n := parseAnsi(seq, 0, tcell.StyleDefault)
	if n != len(seq) {
		t.Fatalf("consumed %d, want %d", n, len(seq))
	}
	_, bg, _ := s.Decompose()
	if bg != tcell.NewRGBColor(1, 2, 3) {
		t.Fatalf("bg = %v", bg)
	}
}

func TestParseAnsiReset(t *testing.T) {
	style := tcell.StyleDefault.Foreground(tcell.ColorRed)
	s, n := parseAnsi("\x1b[0m", 0, style)
	if n != len("\x1b[0m") {
		t.Fatalf("consumed %d", n)
	}
	fg, _, _ := s.Decompose()
	if fg != tcell.ColorDefault {
		t.Fatalf("fg after reset = %v, want default", fg)
	}
}

func TestParseAnsiSkipsUnknown(t *testing.T) {
	style := tcell.StyleDefault.Foreground(tcell.ColorGreen)
	s, n := parseAnsi("\x1b[5m", 0, style) // blink, unhandled -> skipped
	if n != len("\x1b[5m") {
		t.Fatalf("consumed %d", n)
	}
	fg, _, _ := s.Decompose()
	if fg != tcell.ColorGreen {
		t.Fatalf("fg changed on unknown seq: %v", fg)
	}
}

// TestRenderFrame drives the ANSI frame parser into tcell's in-memory
// simulation screen, verifying characters land in the right cells with the
// parsed foreground color.
func TestRenderFrame(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	defer scr.Fini()
	scr.SetSize(4, 2)

	renderFrame(scr, "ab\x1b[38;2;255;0;0mC\x1b[0m\nde")

	c0, _, _, _ := scr.GetContent(0, 0)
	c1, _, _, _ := scr.GetContent(1, 0)
	c2, _, st, _ := scr.GetContent(2, 0)
	d0, _, _, _ := scr.GetContent(0, 1)
	if c0 != 'a' || c1 != 'b' || c2 != 'C' || d0 != 'd' {
		t.Fatalf("cells: %q %q %q %q", c0, c1, c2, d0)
	}
	fg, _, _ := st.Decompose()
	if fg != tcell.NewRGBColor(255, 0, 0) {
		t.Fatalf("fg = %v", fg)
	}
}

// stubSource is a minimal frameSource for exercising the pre-buffer logic.
type stubSource struct {
	frames []string
	pos    int
	ended  bool
}

func (s *stubSource) next() (string, bool) {
	if s.ended || s.pos >= len(s.frames) {
		return "", false
	}
	f := s.frames[s.pos]
	s.pos++
	if s.pos >= len(s.frames) {
		s.ended = true
	}
	return f, true
}

func (s *stubSource) count() int { return len(s.frames) }

func TestTargetPrebufferFrames(t *testing.T) {
	step := 0.04
	maxFrames := max(minPrebufferFrames, int(maxPrebufferSeconds/step))
	cases := []struct {
		interval float64
		want     int
	}{
		{0.0001, minPrebufferFrames}, // very fast -> bank the minimum
		{0.001, minPrebufferFrames},
		{0.005, 7},
		{0.02, 31},
		{0.04, maxFrames}, // realtime (25fps) banks the full cushion
		{0.2, maxFrames},  // slower than realtime clamps to the cap
	}
	for _, c := range cases {
		if got := targetPrebufferFrames(c.interval, step); got != c.want {
			t.Fatalf("interval %v: got %d, want %d", c.interval, got, c.want)
		}
	}
	// monotonic non-decreasing as production slows
	prev := 0
	for _, iv := range []float64{0.0001, 0.001, 0.005, 0.02, 0.04, 0.1, 0.5} {
		got := targetPrebufferFrames(iv, step)
		if got < prev {
			t.Fatalf("not monotonic at %v: %d < %d", iv, got, prev)
		}
		prev = got
	}
}

func TestPrebufferSourceServesBufferThenDelegates(t *testing.T) {
	stub := &stubSource{frames: []string{"a", "b", "c"}}
	pb := &prebufferSource{src: stub, buf: []string{"x", "y"}}
	for _, want := range []string{"x", "y", "a", "b", "c"} {
		got, ok := pb.next()
		if !ok || got != want {
			t.Fatalf("next() = %q, %v; want %q", got, ok, want)
		}
	}
	if _, ok := pb.next(); ok {
		t.Fatal("expected EOF after exhausting source")
	}
	if pb.count() != 3 {
		t.Fatalf("count() = %d, want 3", pb.count())
	}
}

func TestFillPrebufferBanksMinWhenFast(t *testing.T) {
	frames := make([]string, 10)
	for i := range frames {
		frames[i] = fmt.Sprintf("f%d", i)
	}
	pb := &prebufferSource{src: &stubSource{frames: frames}}
	fillPrebuffer(pb, 0.04)
	if len(pb.buf) < minPrebufferFrames {
		t.Fatalf("buffered %d, want >= %d", len(pb.buf), minPrebufferFrames)
	}
	if len(pb.buf) >= len(frames) {
		t.Fatalf("fast production should stop early, buffered all %d", len(pb.buf))
	}
}

func TestFillPrebufferStopsAtSourceEnd(t *testing.T) {
	pb := &prebufferSource{src: &stubSource{frames: []string{"a", "b"}}}
	fillPrebuffer(pb, 0.04)
	if len(pb.buf) != 2 {
		t.Fatalf("buffered %d, want 2", len(pb.buf))
	}
}
