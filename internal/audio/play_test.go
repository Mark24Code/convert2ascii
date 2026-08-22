package audio

import "testing"

func TestPCMStreamerMonoUpmix(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte{0x01, 0x00} // +1
	close(ch)
	s := &pcmStreamer{ch: ch, channels: 1}
	var buf [4][2]float64
	n, ok := s.Stream(buf[:])
	if !ok || n != 1 {
		t.Fatalf("n=%d ok=%v", n, ok)
	}
	want := 1.0 / 32768.0
	if buf[0][0] != want || buf[0][1] != want {
		t.Fatalf("mono upmix = %v, want both %v", buf[0], want)
	}

	// negative sample: 0xFFFF => -1
	ch2 := make(chan []byte, 1)
	ch2 <- []byte{0xFF, 0xFF}
	close(ch2)
	s2 := &pcmStreamer{ch: ch2, channels: 1}
	var buf2 [4][2]float64
	n, ok = s2.Stream(buf2[:])
	if !ok || n != 1 {
		t.Fatalf("negative n=%d ok=%v", n, ok)
	}
	wantNeg := -1.0 / 32768.0
	if buf2[0][0] != wantNeg || buf2[0][1] != wantNeg {
		t.Fatalf("negative upmix = %v", buf2[0])
	}
}

func TestPCMStreamerStereo(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte{0x01, 0x00, 0xFE, 0xFF} // L=+1, R=-2
	close(ch)
	s := &pcmStreamer{ch: ch, channels: 2}
	var buf [4][2]float64
	n, ok := s.Stream(buf[:])
	if !ok || n != 1 {
		t.Fatalf("n=%d ok=%v", n, ok)
	}
	if buf[0][0] != 1.0/32768.0 || buf[0][1] != -2.0/32768.0 {
		t.Fatalf("stereo = %v", buf[0])
	}
}

func TestPCMStreamerEOF(t *testing.T) {
	ch := make(chan []byte)
	close(ch)
	s := &pcmStreamer{ch: ch, channels: 2}
	var buf [4][2]float64
	if n, ok := s.Stream(buf[:]); ok || n != 0 {
		t.Fatalf("n=%d ok=%v, want EOF", n, ok)
	}
}

func TestPCMStreamerLoopReplay(t *testing.T) {
	ch := make(chan []byte, 1)
	ch <- []byte{0x01, 0x00} // mono +1
	close(ch)
	s := &pcmStreamer{ch: ch, channels: 1, loop: true}
	var buf [4][2]float64
	// Once the channel closes, loop mode wraps into the buffered chunk, so a
	// single Stream call fills the whole requested buffer.
	n, ok := s.Stream(buf[:])
	if !ok || n != len(buf) {
		t.Fatalf("loop should fill the buffer: n=%d ok=%v", n, ok)
	}
	for i, v := range buf {
		if v[0] != 1.0/32768.0 || v[1] != 1.0/32768.0 {
			t.Fatalf("sample %d = %v", i, v)
		}
	}
	// looping never EOFs: further calls keep returning data
	if n, ok := s.Stream(buf[:]); !ok || n != len(buf) {
		t.Fatalf("loop should never end: n=%d ok=%v", n, ok)
	}
}

func TestNewStreamPlayerRejectsInvalidParams(t *testing.T) {
	ch := make(chan []byte)
	defer close(ch)
	cases := []StreamParams{
		{SampleRate: 0, Channels: 2, PCM: ch},
		{SampleRate: 44100, Channels: 0, PCM: ch},
		{SampleRate: 44100, Channels: 3, PCM: ch},
		{SampleRate: 44100, Channels: 2, PCM: nil},
	}
	for i, sp := range cases {
		if _, err := NewStreamPlayer(sp, false); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}
