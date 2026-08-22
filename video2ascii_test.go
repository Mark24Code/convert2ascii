package convert2ascii_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mark24Code/convert2ascii/go2ascii"
)

// Repo test assets live in the repo-root test/assets directory.
const testVideo = "test/assets/fireworks.mp4"

func TestVideo2AsciiGenerateSave(t *testing.T) {
	dir := t.TempDir()
	v := convert2ascii.NewVideo2Ascii(convert2ascii.VideoOptions{
		URI:    testVideo,
		Width:  40,
		Output: dir,
	})
	if err := v.Generate(); err != nil {
		t.Fatal(err)
	}

	metaRaw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["step_duration"] == nil || meta["frames_count"] == nil {
		t.Fatalf("meta missing keys: %v", meta)
	}

	if _, err := os.Stat(filepath.Join(dir, "audio.wav")); err != nil {
		t.Fatalf("audio.wav missing: %v", err)
	}

	txts, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(txts) == 0 {
		t.Fatal("no txt frames saved")
	}
	fc, ok := meta["frames_count"].(float64)
	if !ok || int(fc) != len(txts) {
		t.Fatalf("meta frames_count %v != txt count %d", meta["frames_count"], len(txts))
	}
}

func TestVideo2AsciiGenerateTwice(t *testing.T) {
	dir := t.TempDir()
	v := convert2ascii.NewVideo2Ascii(convert2ascii.VideoOptions{
		URI:    testVideo,
		Width:  40,
		Output: dir,
	})
	if err := v.Generate(); err != nil {
		t.Fatal(err)
	}
	// A second Generate on the same instance must re-run the pipeline and
	// overwrite cleanly (no stale tmpdir coupling).
	if err := v.Generate(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "audio.wav")); err != nil {
		t.Fatalf("audio.wav missing after double Generate: %v", err)
	}
}

func TestVideo2AsciiTextFramesArePlain(t *testing.T) {
	dir := t.TempDir()
	v := convert2ascii.NewVideo2Ascii(convert2ascii.VideoOptions{
		URI:    testVideo,
		Width:  40,
		Style:  "text",
		Output: dir,
	})
	if err := v.Generate(); err != nil {
		t.Fatal(err)
	}
	txts, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	if len(txts) == 0 {
		t.Fatal("no txt frames")
	}
	raw, _ := os.ReadFile(txts[0])
	if strings.Contains(string(raw), "\x1b[") {
		t.Fatal("text-style frame contains ANSI")
	}
}
