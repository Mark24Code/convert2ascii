package convert2ascii

import (
	"context"
	"errors"
	"testing"
	"time"
)

// streamTestVideo is the repo asset one level up from the go2ascii module.
const streamTestVideo = "../test/assets/fireworks.mp4"

// TestGeneratePlayStreamsWithoutTempFile verifies the live play path wires up a
// real-time audio stream (no temp WAV, no full-track wait), produces frames,
// and shuts both pipelines down cleanly on cancel.
func TestGeneratePlayStreamsWithoutTempFile(t *testing.T) {
	v := NewVideo2Ascii(VideoOptions{URI: streamTestVideo, Width: 40})
	if err := v.Generate(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if v.cancel != nil {
			v.cancel()
			<-v.playerErrCh // video pipeline stops
			if v.audioErrCh != nil {
				select {
				case err := <-v.audioErrCh:
					if err != nil && !errors.Is(err, context.Canceled) {
						t.Errorf("audio stream ended: %v", err)
					}
				default:
					// audio decode observes cancel and exits
				}
			}
		}
	}()

	if v.audioStream == nil {
		t.Fatal("play mode should have a live audio stream")
	}
	if v.audioPath != "" {
		t.Fatalf("play mode must not create a temp wav, got %q", v.audioPath)
	}
	if v.framesCh == nil {
		t.Fatal("frames channel missing")
	}
	// the pipeline is actually producing frames
	for i := 0; i < 3; i++ {
		select {
		case f, ok := <-v.framesCh:
			if !ok || f == "" {
				t.Fatal("bad frame from stream")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no frame within 5s")
		}
	}
}
