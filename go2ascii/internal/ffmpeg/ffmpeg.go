//go:build cgo

// Package ffmpeg is a thin cgo binding over FFmpeg for frame decoding and
// audio extraction. It requires FFmpeg >= 6 and CGO_ENABLED=1.
package ffmpeg

/*
#cgo pkg-config: libavformat libavcodec libavutil libswscale libswresample
#cgo CFLAGS: -Wno-deprecated-declarations
#include <stdlib.h>
#include "ffmpeg.h"
*/
import "C"

import (
	"fmt"
	"math"
	"sync"
	"unsafe"
)

// Frame is a decoded RGBA image.
//
// In the parallel path, K is the rounded time-slot (k = floor(t/step + 0.5))
// used as the merge key, and Free releases the C-side pixel buffer the frame
// zero-copy-views (Pixels points into C memory, not Go heap). Both are zero/nil
// for frames produced by DecodeFrames, which copies into Go memory.
type Frame struct {
	Index  int
	Width  int
	Height int
	Pixels []byte // RGBA, len == Width*Height*4
	K      int
	Free   func()
}

// ProbeInfo is the cheap stream metadata needed to plan a decode.
type ProbeInfo struct {
	Duration        float64
	Width           int
	Height          int
	HasVideo        bool
	HasAudio        bool
	AudioSampleRate int
	AudioChannels   int
}

// AudioData is interleaved signed 16-bit PCM.
type AudioData struct {
	PCM        []byte
	SampleRate int
	Channels   int
}

// DecodeFrames decodes the video at path, yielding one frame per
// stepDuration seconds (relative to the first decoded frame's timestamp).
// yield is called sequentially by the decoder goroutine.
func DecodeFrames(path string, stepDuration float64, yield func(Frame) error) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var c C.g2a
	rc := C.g2a_open(&c, cpath)
	defer C.g2a_close(&c)
	if rc < 0 {
		return fmt.Errorf("open %s: ffmpeg error %d", path, int(rc))
	}
	if c.video_stream < 0 {
		return fmt.Errorf("no video stream in %s", path)
	}

	lastK := -1
	index := 0
	for {
		rc := C.g2a_next_frame(&c)
		if rc < 0 {
			return fmt.Errorf("decode: ffmpeg error %d", int(rc))
		}
		if rc == 0 {
			return nil // EOF
		}
		var t C.double
		C.g2a_frame_time(&c, &t)
		k := int(math.Floor(float64(t)/stepDuration + 0.5))
		if k <= lastK {
			continue
		}
		lastK = k

		var w, h C.int
		buf := C.g2a_export_frame(&c, &w, &h)
		if buf == nil {
			return fmt.Errorf("decode: export frame failed")
		}
		n := int(w) * int(h) * 4
		pixels := C.GoBytes(unsafe.Pointer(buf), C.int(n))
		C.g2a_free(unsafe.Pointer(buf))

		if err := yield(Frame{Index: index, Width: int(w), Height: int(h), Pixels: pixels}); err != nil {
			return err
		}
		index++
	}
}

// DecodeAudio decodes the audio stream at path into interleaved S16 PCM.
// Returns nil, nil if the file has no audio stream.
func DecodeAudio(path string) (*AudioData, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var c C.g2a
	rc := C.g2a_open(&c, cpath)
	defer C.g2a_close(&c)
	if rc < 0 {
		return nil, fmt.Errorf("open %s: ffmpeg error %d", path, int(rc))
	}
	if c.audio_stream < 0 {
		return nil, nil
	}
	for {
		rc := C.g2a_audio_next(&c)
		if rc < 0 {
			return nil, fmt.Errorf("audio decode: ffmpeg error %d", int(rc))
		}
		if rc == 0 {
			break
		}
	}
	var buf *C.uint8_t
	var n C.int
	C.g2a_audio_take(&c, &buf, &n)
	if buf == nil || n == 0 {
		return nil, nil
	}
	pcm := C.GoBytes(unsafe.Pointer(buf), n)
	C.free(unsafe.Pointer(buf))
	return &AudioData{PCM: pcm, SampleRate: int(c.sample_rate), Channels: int(c.nb_channels)}, nil
}

// Duration returns the video length in seconds.
func Duration(path string) (float64, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var c C.g2a
	rc := C.g2a_open(&c, cpath)
	defer C.g2a_close(&c)
	if rc < 0 {
		return 0, fmt.Errorf("open %s: ffmpeg error %d", path, int(rc))
	}
	return float64(C.g2a_duration(&c)), nil
}

// Probe returns stream metadata for path without decoding any frame.
func Probe(path string) (ProbeInfo, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var c C.g2a
	rc := C.g2a_open(&c, cpath)
	defer C.g2a_close(&c)
	if rc < 0 {
		return ProbeInfo{}, fmt.Errorf("open %s: ffmpeg error %d", path, int(rc))
	}
	info := ProbeInfo{Duration: float64(C.g2a_duration(&c))}
	if c.video_stream >= 0 {
		info.HasVideo = true
		var w, h C.int
		if C.g2a_video_dims(&c, &w, &h) == 0 {
			info.Width = int(w)
			info.Height = int(h)
		}
	}
	if c.audio_stream >= 0 {
		info.HasAudio = true
		info.AudioSampleRate = int(c.sample_rate)
		info.AudioChannels = int(c.nb_channels)
	}
	return info, nil
}

// DecodeAudioStream decodes the audio stream at path, calling write for each
// chunk of interleaved S16 PCM as it is produced, so memory stays bounded on
// long videos. Returns the sample rate and channel count, or 0,0 if the file
// has no audio stream (write is not called).
func DecodeAudioStream(path string, write func(pcm []byte) error) (sampleRate, channels int, err error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var c C.g2a
	rc := C.g2a_open(&c, cpath)
	defer C.g2a_close(&c)
	if rc < 0 {
		return 0, 0, fmt.Errorf("open %s: ffmpeg error %d", path, int(rc))
	}
	if c.audio_stream < 0 {
		return 0, 0, nil
	}
	for {
		rc := C.g2a_audio_next(&c)
		if rc < 0 {
			return 0, 0, fmt.Errorf("audio decode: ffmpeg error %d", int(rc))
		}
		if rc == 0 {
			break
		}
		var buf *C.uint8_t
		var n C.int
		C.g2a_audio_take(&c, &buf, &n)
		if buf == nil || n == 0 {
			continue
		}
		pcm := C.GoBytes(unsafe.Pointer(buf), n)
		C.free(unsafe.Pointer(buf))
		if err := write(pcm); err != nil {
			return 0, 0, err
		}
	}
	// Flush whatever the EOF drain left in the buffer.
	var buf *C.uint8_t
	var n C.int
	C.g2a_audio_take(&c, &buf, &n)
	if buf != nil && n > 0 {
		pcm := C.GoBytes(unsafe.Pointer(buf), n)
		C.free(unsafe.Pointer(buf))
		if err := write(pcm); err != nil {
			return 0, 0, err
		}
	}
	return int(c.sample_rate), int(c.nb_channels), nil
}

// DecodeFramesParallel decodes path with segCount independent cgo contexts,
// each covering a contiguous k-slot range, and calls yield for every selected
// frame. When targetW/H > 0 each exported frame is downscaled to that size in
// C (sws_scale); frames are zero-copy views over C memory.
//
// yield may be called concurrently from the segment goroutines, in arrival
// order (NOT k order), and each frame carries K (its global time slot) so the
// caller can order/number output. yield receives each frame exactly once and
// OWNS it: it MUST call fr.Free() (when non-nil) after the pixels are done
// with, including when it returns an error. The produced frame set is
// identical to DecodeFrames (same k = floor(t/step+0.5) selection, deduped
// per segment). Any error stops all segments.
func DecodeFramesParallel(path string, step float64, segCount, targetW, targetH int, yield func(Frame) error) error {
	info, err := Probe(path)
	if err != nil {
		return err
	}
	if !info.HasVideo || info.Duration <= 0 {
		return fmt.Errorf("no video stream in %s", path)
	}
	if segCount < 1 {
		segCount = 1
	}
	if targetW <= 0 || targetH <= 0 {
		targetW, targetH = 0, 0
	}

	totalK := int(info.Duration/step) + 1
	if totalK < 1 {
		totalK = 1
	}
	segSize := (totalK + segCount - 1) / segCount

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	done := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(done) }) }

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for i := 0; i < segCount; i++ {
		startK := i * segSize
		openEnd := (i == segCount-1)
		startSec := float64(startK)*step - 0.5*step
		endSec := 0.0 // open window for the last segment
		if !openEnd {
			endSec = float64(startK+segSize)*step + 0.5*step
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := decodeSegment(cpath, step, startK, segSize, openEnd, startSec, endSec, targetW, targetH, yield, done); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				stop()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// decodeSegment decodes one k-range [startK, startK+segSize) of path, dropping
// frames owned by other segments and calling yield for each owned k at most
// once (concurrently with other segments).
func decodeSegment(cpath *C.char, step float64, startK, segSize int, openEnd bool, startSec, endSec float64, targetW, targetH int, yield func(Frame) error, done <-chan struct{}) error {
	var c C.g2a
	rc := C.g2a_open(&c, cpath)
	if rc < 0 {
		return fmt.Errorf("open segment: ffmpeg error %d", int(rc))
	}
	defer C.g2a_close(&c)
	if c.video_stream < 0 {
		return fmt.Errorf("segment: no video stream")
	}
	if rc := C.g2a_config(&c, C.double(startSec), C.double(endSec), C.int(targetW), C.int(targetH)); rc < 0 {
		return fmt.Errorf("g2a_config: ffmpeg error %d", int(rc))
	}
	lastK := -1
	for {
		select {
		case <-done:
			return nil
		default:
		}
		rc := C.g2a_next_frame(&c)
		if rc < 0 {
			return fmt.Errorf("decode: ffmpeg error %d", int(rc))
		}
		if rc == 0 {
			return nil // EOF or segment end
		}
		var t C.double
		C.g2a_abs_frame_time(&c, &t)
		k := int(math.Floor(float64(t)/step + 0.5))
		if k < startK {
			continue // pre-window (post-seek); drop
		}
		if !openEnd && k >= startK+segSize {
			continue // owned by the next segment
		}
		if k <= lastK {
			continue // dedup within segment (same rule as DecodeFrames)
		}
		lastK = k

		var w, h C.int
		buf := C.g2a_export_frame(&c, &w, &h)
		if buf == nil {
			return fmt.Errorf("decode: export frame failed")
		}
		n := int(w) * int(h) * 4
		fr := Frame{
			K:      k,
			Width:  int(w),
			Height: int(h),
			Pixels: unsafe.Slice((*byte)(unsafe.Pointer(buf)), n),
			Free:   func() { C.g2a_free(unsafe.Pointer(buf)) },
		}
		// Ownership passes to yield; it must Free the frame, including on error.
		if err := yield(fr); err != nil {
			return err
		}
	}
}
