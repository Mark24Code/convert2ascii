// Package ansi wraps the ANSI escape codes used by the Ruby Terminal helper.
package ansi

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

const (
	clearBuffer = "\x1b[3J"
	clearScreen = "\x1b[2J"
	hideCursor  = "\x1b[?25l"
	showCursor  = "\x1b[?25h"
	openBuffer  = "\x1b[?1049h"
	closeBuffer = "\x1b[?1049l"
)

// ClearBuffer clears the scrollback buffer.
func ClearBuffer() { fmt.Print(clearBuffer) }

// ClearScreen clears the screen.
func ClearScreen() { fmt.Print(clearScreen) }

// HideCursor hides the terminal cursor.
func HideCursor() { fmt.Print(hideCursor) }

// ShowCursor shows the terminal cursor.
func ShowCursor() { fmt.Print(showCursor) }

// OpenBuffer switches to the alternate screen buffer.
func OpenBuffer() { fmt.Print(openBuffer) }

// CloseBuffer leaves the alternate screen buffer.
func CloseBuffer() { fmt.Print(closeBuffer) }

// Size returns the terminal size (rows, cols), falling back to (24, 80).
func Size() (rows, cols int) {
	r, c, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || r <= 0 || c <= 0 {
		return 24, 80
	}
	return r, c
}
