// Package audio handles WAV encoding and playback.
package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// WAVWriter streams 16-bit PCM into a WAV file, writing the 44-byte header up
// front with placeholder sizes and patching the RIFF/data chunk lengths on
// Close. Use it when the full PCM buffer is too large to hold in memory (long
// videos). w must be seekable (e.g. *os.File).
type WAVWriter struct {
	w          io.Writer
	sampleRate uint32
	channels   uint16
	dataLen    uint32
	closed     bool
}

// NewWAVWriter writes a WAV header (with placeholder sizes) to w.
func NewWAVWriter(w io.Writer, sampleRate, channels int) (*WAVWriter, error) {
	if sampleRate <= 0 || channels <= 0 {
		return nil, errors.New("wav: invalid sample rate or channels")
	}
	ww := &WAVWriter{w: w, sampleRate: uint32(sampleRate), channels: uint16(channels)}
	byteRate := ww.sampleRate * uint32(ww.channels) * 2
	blockAlign := ww.channels * 2
	hdr := make([]byte, 44)
	copy(hdr, "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], 0) // patched on Close
	copy(hdr[8:], "WAVE")
	copy(hdr[12:], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:], ww.channels)
	binary.LittleEndian.PutUint32(hdr[24:], ww.sampleRate)
	binary.LittleEndian.PutUint32(hdr[28:], byteRate)
	binary.LittleEndian.PutUint16(hdr[32:], blockAlign)
	binary.LittleEndian.PutUint16(hdr[34:], 16) // bits per sample
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], 0) // patched on Close
	if _, err := w.Write(hdr); err != nil {
		return nil, err
	}
	return ww, nil
}

// Write appends interleaved 16-bit PCM samples.
func (ww *WAVWriter) Write(pcm []byte) (int, error) {
	if len(pcm)%2 != 0 {
		return 0, errors.New("wav: pcm must be byte-aligned to 16-bit samples")
	}
	n, err := ww.w.Write(pcm)
	ww.dataLen += uint32(n)
	return n, err
}

// Close patches the RIFF and data chunk sizes. idempotent.
func (ww *WAVWriter) Close() error {
	if ww.closed {
		return nil
	}
	ww.closed = true
	seeker, ok := ww.w.(io.Seeker)
	if !ok {
		return errors.New("wav: writer must be seekable to patch sizes")
	}
	pos, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	patch := func(off uint32, v uint32) error {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		if _, err := seeker.Seek(int64(off), io.SeekStart); err != nil {
			return err
		}
		_, err := ww.w.Write(b[:])
		return err
	}
	if err := patch(4, ww.dataLen+36); err != nil {
		return err
	}
	if err := patch(40, ww.dataLen); err != nil {
		return err
	}
	_, err = seeker.Seek(pos, io.SeekStart)
	return err
}

// EncodeWAV wraps 16-bit interleaved PCM samples in a standard 44-byte
// WAV header (PCM, mono/stereo+).
func EncodeWAV(sampleRate, channels int, pcm []byte) ([]byte, error) {
	if sampleRate <= 0 || channels <= 0 {
		return nil, errors.New("wav: invalid sample rate or channels")
	}
	if len(pcm)%2 != 0 {
		return nil, errors.New("wav: pcm must be byte-aligned to 16-bit samples")
	}
	dataLen := uint32(len(pcm))
	byteRate := uint32(sampleRate * channels * 2)
	blockAlign := uint16(channels * 2)

	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, dataLen+36)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(buf, binary.LittleEndian, uint16(channels))
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(buf, binary.LittleEndian, byteRate)
	binary.Write(buf, binary.LittleEndian, blockAlign)
	binary.Write(buf, binary.LittleEndian, uint16(16)) // bits per sample
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, dataLen)
	buf.Write(pcm)
	return buf.Bytes(), nil
}
