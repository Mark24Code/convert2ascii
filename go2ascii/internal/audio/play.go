package audio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/wav"
	"github.com/hajimehoshi/oto/v2"
)

// PCM frame layout fed to oto: 2 interleaved channels x 16-bit little-endian.
// beep's Streamer interface is inherently stereo ([2]float64) — mono sources
// are up-mixed to stereo by the decoders — so playback is always stereo, and
// oto only accepts 1 or 2 channels anyway.
const (
	channelCount    = 2
	bitDepthInBytes = 2
	pcmFrameBytes   = channelCount * bitDepthInBytes

	// bufferSeconds is the target length of oto's internal player buffer. A
	// small buffer keeps Position() close to real playback time (see NewPlayer).
	bufferSeconds = 0.05
)

// Player plays a local audio file (wav or mp3) and exposes a playback clock
// in seconds. Position() is derived from a sample counter, giving a stable
// master clock for A/V sync.
type Player struct {
	ctx     *oto.Context
	out     oto.Player
	counter *sampleCounter
	once    sync.Once
}

// sampleCounter wraps a beep.Streamer and counts the samples oto pulls from it,
// exposing a playback clock. It also implements io.Reader by converting the
// streamed float samples to little-endian 16-bit PCM, so it can be handed to
// oto.NewPlayer directly: oto pulls PCM bytes through Read, which pulls samples
// through the embedded streamer — the very path that advances the clock.
type sampleCounter struct {
	beep.Streamer
	mu   sync.Mutex
	pos  int64
	rate float64
	buf  [][2]float64
}

func (c *sampleCounter) position() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return float64(c.pos) / c.rate
}

// Read converts streamed float64 samples into little-endian 16-bit PCM. oto
// may invoke Read concurrently across a Pause→Play transition (its playerImpl
// releases its lock around the external Read call), so the whole body runs
// under c.mu, which also guards c.buf (the shared scratch buffer), c.pos, and
// position(). The embedded streamer is called directly — not c.Stream, which
// no longer exists and would re-lock c.mu — so the sample clock advances under
// the same lock that position() reads.
func (c *sampleCounter) Read(buf []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(buf)%pcmFrameBytes != 0 {
		return 0, errors.New("audio: read buffer not aligned to PCM frame")
	}
	ns := len(buf) / pcmFrameBytes
	if len(c.buf) < ns {
		c.buf = make([][2]float64, ns)
	}
	n, ok := c.Streamer.Stream(c.buf[:ns])
	c.pos += int64(n)
	if !ok {
		if c.Streamer.Err() != nil {
			return 0, c.Streamer.Err()
		}
		if n == 0 {
			return 0, io.EOF
		}
	}
	for i := 0; i < n; i++ {
		for ch := 0; ch < channelCount; ch++ {
			v := c.buf[i][ch]
			if v > 1 {
				v = 1
			}
			if v < -1 {
				v = -1
			}
			s := int16(v * (1<<15 - 1))
			o := i*pcmFrameBytes + ch*bitDepthInBytes
			buf[o] = byte(s)
			buf[o+1] = byte(s >> 8)
		}
	}
	return n * pcmFrameBytes, nil
}

// NewPlayer opens path (wav or mp3) and prepares an oto player. If loop is
// true, the audio repeats indefinitely.
func NewPlayer(path string, loop bool) (*Player, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	streamer, format, err := decode(f, path)
	if err != nil {
		return nil, err
	}

	buffered := beep.NewBuffer(format)
	buffered.Append(streamer)
	// Append stops silently when the decoder hits a read error; surface it so
	// a truncated or corrupt file fails here instead of playing partially with
	// a frozen Position clock.
	if err := streamer.Err(); err != nil {
		return nil, err
	}
	if err := streamer.Close(); err != nil {
		return nil, err
	}

	seeker := buffered.Streamer(0, buffered.Len()) // beep.StreamSeeker
	var src beep.Streamer = seeker
	if loop {
		src = beep.Loop(-1, seeker)
	}

	ctx, ready, err := oto.NewContext(int(format.SampleRate), channelCount, bitDepthInBytes)
	if err != nil {
		return nil, err
	}
	<-ready

	// The counter MUST be the oto source (oto pulls bytes from it), so the
	// sample clock actually advances; handing ctx.NewPlayer anything else
	// would leave Position() stuck at 0 forever.
	counter := &sampleCounter{Streamer: src, rate: float64(format.SampleRate)}
	out := ctx.NewPlayer(counter)

	// Shrink oto's player buffer from its ~0.5s default down to ~50ms. oto
	// pulls samples from the counter into this buffer ahead of playback, so a
	// big buffer would make Position() run up to a second ahead of what is
	// actually heard — useless as an A/V sync clock. The byte count is kept a
	// multiple of pcmFrameBytes so oto's read buffers stay frame-aligned.
	if bs, ok := out.(oto.BufferSizeSetter); ok {
		bs.SetBufferSize(int(float64(format.SampleRate)*bufferSeconds) * pcmFrameBytes)
	}

	return &Player{
		ctx:     ctx,
		out:     out,
		counter: counter,
	}, nil
}

// Play starts audio output.
func (p *Player) Play() {
	if p.out != nil {
		p.out.Play()
	}
}

// Position returns seconds of audio played so far.
func (p *Player) Position() float64 {
	if p.counter == nil {
		return 0
	}
	return p.counter.position()
}

// Close stops audio and releases resources. Idempotent.
func (p *Player) Close() error {
	var err error
	p.once.Do(func() {
		if p.out != nil {
			err = p.out.Close()
		}
	})
	return err
}

func decode(f *os.File, path string) (beep.StreamSeekCloser, beep.Format, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		return mp3.Decode(f)
	case ".wav":
		return wav.Decode(f)
	default:
		return nil, beep.Format{}, fmt.Errorf("unsupported audio format: %s", ext)
	}
}
