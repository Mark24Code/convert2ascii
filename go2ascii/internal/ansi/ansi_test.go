package ansi

import (
	"os"
	"testing"
)

func TestEscapeSequences(t *testing.T) {
	if clearBuffer != "\x1b[3J" || clearScreen != "\x1b[2J" ||
		hideCursor != "\x1b[?25l" || showCursor != "\x1b[?25h" ||
		openBuffer != "\x1b[?1049h" || closeBuffer != "\x1b[?1049l" {
		t.Fatal("escape constants wrong")
	}
}

func TestSizeFallback(t *testing.T) {
	// Non-tty stdout should fall back to (24, 80).
	rows, cols := Size()
	if rows <= 0 || cols <= 0 {
		t.Fatalf("bad size %dx%d", rows, cols)
	}
	_ = os.Stdout
}
