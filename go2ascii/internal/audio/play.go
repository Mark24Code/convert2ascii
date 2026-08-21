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

// StreamParams carries the metadata and PCM source for live audio streaming
// (the play path that skips the temp-WAV round-trip).
type StreamParams struct {
	SampleRate int           // audio sample rate in Hz
	Channels   int           // 1 (mono) or 2 (stereo); oto only accepts 1 or 2
	PCM        <-chan []byte // interleaved S16 little-endian chunks
}

// pcmStreamer adapts a channel of interleaved S16 PCM to beep's stereo float64
// Streamer. Mono sources are up-mixed to stereo (matching beep's file
// decoders). In loop mode every consumed chunk is buffered and replayed after
// the channel closes, so memory grows with the full track — the same documented
// trade-off as streamSource in the player package.
//
// pcmStreamer is not goroutine-safe; it is only driven through
// sampleCounter.Read, which serializes access under its own mutex.
type pcmStreamer struct {
	ch       <-chan []byte
	loop     bool
	channels int // 1 or 2

	pending []byte   // unconsumed bytes of the current chunk
	loopBuf [][]byte // every consumed chunk, for loop replay
	loopPos int
	ended   bool
}

// Stream fills samples from the PCM channel, blocking when the channel is empty
// (backpressure on the decoder). Returns (n, false) at EOF when not looping; in
// loop mode it wraps back into the buffered chunks.
func (s *pcmStreamer) Stream(samples [][2]float64) (int, bool) {
	out := 0
	for out < len(samples) {
		if len(s.pending) == 0 {
			if s.ended {
				if s.loop && len(s.loopBuf) > 0 {
					s.pending = s.loopBuf[s.loopPos]
					s.loopPos++
					if s.loopPos >= len(s.loopBuf) {
						s.loopPos = 0
					}
					continue
				}
				break
			}
			chunk, ok := <-s.ch
			if !ok {
				s.ended = true
				continue
			}
			if s.loop {
				s.loopBuf = append(s.loopBuf, chunk)
			}
			s.pending = chunk
		}
		frameBytes := s.channels * 2
		if len(s.pending) < frameBytes {
			// Partial frame at a chunk boundary; ffmpeg always emits aligned
			// chunks, so this is defensive only.
			s.pending = nil
			continue
		}
		l := float64(int16(uint16(s.pending[0])|uint16(s.pending[1])<<8)) / 32768.0
		r := l
		if s.channels == 2 {
			r = float64(int16(uint16(s.pending[2])|uint16(s.pending[3])<<8)) / 32768.0
		}
		samples[out] = [2]float64{l, r}
		s.pending = s.pending[frameBytes:]
		out++
	}
	if out == 0 {
		return 0, false
	}
	return out, true
}

func (s *pcmStreamer) Err() error { return nil }

// NewStreamPlayer builds a Player that streams interleaved S16 PCM from the
// channel to the audio device in real time, with the same 50ms oto buffer and
// sample-counter clock as NewPlayer, so Position() remains a valid A/V master
// clock. If loop is true the stream is buffered as it is consumed and replayed
// after the channel closes.
func NewStreamPlayer(sp StreamParams, loop bool) (*Player, error) {
	if sp.SampleRate <= 0 {
		return nil, fmt.Errorf("audio: invalid sample rate %d", sp.SampleRate)
	}
	if sp.Channels < 1 || sp.Channels > 2 {
		return nil, fmt.Errorf("audio: unsupported channel count %d", sp.Channels)
	}
	if sp.PCM == nil {
		return nil, errors.New("audio: nil PCM channel")
	}
	src := &pcmStreamer{ch: sp.PCM, channels: sp.Channels, loop: loop}
	ctx, ready, err := oto.NewContext(sp.SampleRate, channelCount, bitDepthInBytes)
	if err != nil {
		return nil, err
	}
	<-ready
	counter := &sampleCounter{Streamer: src, rate: float64(sp.SampleRate)}
	out := ctx.NewPlayer(counter)
	if bs, ok := out.(oto.BufferSizeSetter); ok {
		bs.SetBufferSize(int(float64(sp.SampleRate)*bufferSeconds) * pcmFrameBytes)
	}
	return &Player{ctx: ctx, out: out, counter: counter}, nil
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
