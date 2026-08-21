package convert2ascii

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mark24Code/convert2ascii/go2ascii/internal/player"
)

// Player replays ASCII frames with optional audio. Exactly one frame source is
// used: Frames (all in memory), FrameStream (live pull), or FrameDir (lazy
// disk read, so replay memory stays O(1)).
type Player struct {
	Frames       []string
	FrameStream  <-chan string
	FrameDir     string
	AudioPath    string
	StepDuration float64
	PlayLoop     bool
	Debug        bool
}

// Play runs the playback until it ends or is interrupted.
func (p *Player) Play() error {
	return player.New(player.Options{
		Frames:       p.Frames,
		FrameStream:  p.FrameStream,
		FrameDir:     p.FrameDir,
		AudioPath:    p.AudioPath,
		StepDuration: p.StepDuration,
		PlayLoop:     p.PlayLoop,
		Debug:        p.Debug,
	}).Play()
}

type metaConfig struct {
	StepDuration float64 `json:"step_duration"`
	Audio        *string `json:"audio"`
	FramesCount  int     `json:"frames_count"`
}

// PlayFrames replays a saved frames directory (N.txt + meta.json [+ audio]).
// Frames are read from disk one at a time, so replay memory does not grow with
// video length.
func PlayFrames(dir string, playLoop bool) error {
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return fmt.Errorf("read meta.json: %w", err)
	}
	var cfg metaConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse meta.json: %w", err)
	}

	audioPath := ""
	if cfg.Audio != nil && *cfg.Audio != "" {
		audioPath = filepath.Join(dir, *cfg.Audio)
	}

	p := &Player{
		FrameDir:     dir,
		AudioPath:    audioPath,
		StepDuration: cfg.StepDuration,
		PlayLoop:     playLoop,
	}
	return p.Play()
}
