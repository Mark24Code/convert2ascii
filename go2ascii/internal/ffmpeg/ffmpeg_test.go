//go:build cgo

package ffmpeg

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// From internal/ffmpeg, repo root is three levels up.
const video = "../../../test/assets/fireworks.mp4"

func TestDecodeFrames(t *testing.T) {
	count := 0
	err := DecodeFrames(video, 0.1, func(fr Frame) error {
		count++
		if fr.Width <= 0 || fr.Height <= 0 || len(fr.Pixels) != fr.Width*fr.Height*4 {
			t.Fatalf("bad frame %d: %dx%d len=%d", fr.Index, fr.Width, fr.Height, len(fr.Pixels))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatal("no frames decoded")
	}
}

func TestDecodeAudio(t *testing.T) {
	ad, err := DecodeAudio(video)
	if err != nil {
		t.Fatal(err)
	}
	if ad == nil || len(ad.PCM) == 0 {
		t.Fatal("no audio decoded")
	}
	if ad.SampleRate <= 0 || ad.Channels <= 0 {
		t.Fatalf("bad format: %+v", ad)
	}
}

func TestDuration(t *testing.T) {
	d, err := Duration(video)
	if err != nil {
		t.Fatal(err)
	}
	if d <= 0 {
		t.Fatalf("bad duration %f", d)
	}
}

func TestProbe(t *testing.T) {
	info, err := Probe(video)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasVideo || info.Duration <= 0 || info.Width <= 0 || info.Height <= 0 {
		t.Fatalf("bad probe: %+v", info)
	}
}

// TestDecodeFramesParallelCountEqual asserts the parallel segment decoder
// produces exactly the same number of target-size frames as the sequential
// DecodeFrames, with unique k slots (no duplicates across segments).
func TestDecodeFramesParallelCountEqual(t *testing.T) {
	step := 0.04
	seqCount := 0
	if err := DecodeFrames(video, step, func(Frame) error { seqCount++; return nil }); err != nil {
		t.Fatal(err)
	}
	if seqCount < 1 {
		t.Fatal("no frames decoded")
	}

	for _, segs := range []int{1, 2, 4} {
		segs := segs
		t.Run(fmt.Sprintf("segCount=%d", segs), func(t *testing.T) {
			var mu sync.Mutex
			ks := make([]int, 0, seqCount)
			err := DecodeFramesParallel(video, step, segs, 80, 22, func(fr Frame) error {
				defer fr.Free()
				if fr.Width != 80 || fr.Height != 22 {
					return fmt.Errorf("frame k=%d: %dx%d, want 80x22", fr.K, fr.Width, fr.Height)
				}
				if len(fr.Pixels) != 80*22*4 {
					return fmt.Errorf("frame k=%d: pixels %d, want %d", fr.K, len(fr.Pixels), 80*22*4)
				}
				mu.Lock()
				ks = append(ks, fr.K)
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(ks) != seqCount {
				t.Fatalf("segCount=%d: got %d frames, want %d", segs, len(ks), seqCount)
			}
			seen := make(map[int]bool, len(ks))
			for _, k := range ks {
				if seen[k] {
					t.Fatalf("segCount=%d: duplicate k %d", segs, k)
				}
				seen[k] = true
			}
		})
	}
}

// TestDecodeFramesParallelErrorPath exercises the yield-error cleanup path:
// the consumer frees the frames it received and DecodeFramesParallel returns
// the first error without a double-free or leak (running under -race).
func TestDecodeFramesParallelErrorPath(t *testing.T) {
	wantErr := fmt.Errorf("stop")
	var got atomic.Int32
	err := DecodeFramesParallel(video, 0.04, 4, 80, 22, func(fr Frame) error {
		fr.Free()
		if got.Add(1) == 10 {
			return wantErr
		}
		return nil
	})
	if err != wantErr {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if got.Load() < 10 {
		t.Fatalf("yield called %d times, want >= 10", got.Load())
	}
}
