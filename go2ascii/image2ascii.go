// Package convert2ascii is the public library mirroring the Ruby gem's
// Convert2Ascii::Image2Ascii and Convert2Ascii::Video2Ascii classes.
package convert2ascii

import (
	"fmt"

	"github.com/Mark24Code/convert2ascii/go2ascii/internal/ansi"
	"github.com/Mark24Code/convert2ascii/go2ascii/internal/imagelib"
)

// ImageOptions mirrors the Ruby Image2Ascii constructor/generate kwargs.
type ImageOptions struct {
	URI        string
	Width      int
	Style      string // "color" | "text"; empty -> "color"
	Color      string // "full" | "greyscale"; empty -> "full"
	ColorBlock bool
	Chars      string // empty -> DefaultChars
}

// Image2Ascii converts an image to an ASCII-art string.
type Image2Ascii struct {
	URI        string
	Width      int
	Style      string
	Color      string
	ColorBlock bool
	Chars      string

	asciiString string
}

// NewImage2Ascii builds an Image2Ascii. Width 0 means terminal columns.
func NewImage2Ascii(opts ImageOptions) *Image2Ascii {
	width := opts.Width
	if width <= 0 {
		_, cols := ansi.Size()
		width = cols
	}
	return &Image2Ascii{
		URI:        opts.URI,
		Width:      width,
		Style:      opts.Style,
		Color:      opts.Color,
		ColorBlock: opts.ColorBlock,
		Chars:      opts.Chars,
	}
}

// Generate renders the image into a. It returns nil on success; on error, a's
// output is unchanged.
func (a *Image2Ascii) Generate() error {
	if a.Width <= 0 {
		_, cols := ansi.Size()
		a.Width = cols
	}
	a.asciiString = ""
	img, err := imagelib.Open(a.URI)
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	style := imagelib.Style(a.Style)
	if style == "" {
		style = imagelib.StyleColor
	}
	a.asciiString = imagelib.Render(img, imagelib.Options{
		Width:      a.Width,
		Style:      style,
		Color:      imagelib.ColorMode(a.Color),
		ColorBlock: a.ColorBlock,
		Chars:      a.Chars,
	})
	return nil
}

// String returns the rendered ASCII string.
func (a *Image2Ascii) String() string { return a.asciiString }
