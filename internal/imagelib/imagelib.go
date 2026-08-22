package imagelib

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// DefaultChars is the exact character ramp used by the Ruby version.
const DefaultChars = ".'`^\",:;Il!i><~+_-?][}{1)(|\\/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$"

// maxImageBytes bounds HTTP downloads to prevent memory exhaustion.
const maxImageBytes = 64 << 20

// Style mirrors the Ruby STYLE_ENUM: "color" (ANSI) or "text" (plain).
type Style string

const (
	StyleColor Style = "color"
	StyleText  Style = "text"
)

// ColorMode mirrors the Ruby COLOR_ENUM: "full" or "greyscale".
type ColorMode string

const (
	ColorFull ColorMode = "full"
	ColorGrey ColorMode = "greyscale"
)

// Options configure a render.
type Options struct {
	Width      int
	Style      Style
	Color      ColorMode
	ColorBlock bool
	Chars      string
}

// Open loads an image from a local path or an http(s) URL.
// GIFs render as their first frame.
func Open(uri string) (image.Image, error) {
	var data []byte
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(uri)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("imagelib: unexpected status %s", resp.Status)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
		if err != nil {
			return nil, err
		}
		if len(data) >= maxImageBytes {
			return nil, fmt.Errorf("imagelib: image too large (> 64 MiB)")
		}
	} else {
		var err error
		data, err = os.ReadFile(uri)
		if err != nil {
			return nil, err
		}
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// AspectSizeWH computes the output dimensions exactly as the Ruby version's
// two-step scale from source pixel dims: width becomes width; height is
// round(srcH*width/srcW)/2.
func AspectSizeWH(srcW, srcH, width int) (w, h int) {
	if srcW <= 0 {
		srcW = 1
	}
	scaledRows := int(math.Round(float64(srcH) * float64(width) / float64(srcW)))
	return width, scaledRows / 2
}

// AspectSize computes the output dimensions from an image.
func AspectSize(orig image.Image, width int) (w, h int) {
	return AspectSizeWH(orig.Bounds().Dx(), orig.Bounds().Dy(), width)
}

// Render converts img to an ASCII-art string, scaling it to the output size
// (white composite + ApproxBiLinear, matching the Ruby look at ASCII res).
func Render(img image.Image, o Options) string {
	w, h := AspectSize(img, o.Width)
	if w < 2 {
		w = 2
	}
	if h < 1 {
		h = 1
	}

	// Composite onto white so transparent pixels render as bright.
	// ApproxBiLinear (~0.36ms/frame for 720p->80x22) is visually equivalent to
	// CatmullRom (~6.7ms) at ASCII resolution and ~18x faster; Ruby's
	// ImageMagick `scale` is comparably cheap.
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return RenderRGBA(dst, o)
}

// RenderRGBA renders an already-scaled RGBA image to an ASCII string. dst must
// be a tight (0,0,w,h) RGBA with Stride == w*4. The video pipeline uses this on
// frames that sws_scale already downscaled, avoiding a second resize and the
// white composite (sws YUV->RGBA output is opaque).
func RenderRGBA(dst *image.RGBA, o Options) string {
	w := dst.Rect.Dx()
	h := dst.Rect.Dy()

	chars := o.Chars
	if chars == "" {
		chars = DefaultChars
	}
	runes := []rune(chars)
	charLen := len(runes)

	style := o.Style
	if style == "" {
		style = StyleColor
	}
	colorMode := o.Color
	if colorMode == "" {
		colorMode = ColorFull
	}

	scale := 255.0 / float64(charLen)

	// Build into a []byte with strconv.AppendInt to avoid per-char allocations
	// (color mode writes ~30 bytes/char through 3+ strconv.Itoa calls).
	buf := make([]byte, 0, w*h*8)
	for y := 0; y < h; y++ {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
		for x := 0; x < w; x++ {
			i := x * 4
			r := float64(row[i])
			g := float64(row[i+1])
			b := float64(row[i+2])
			brightness := 0.2126*r + 0.7152*g + 0.0722*b
			idx := int(brightness / scale)
			if idx >= charLen {
				idx = charLen - 1
			}
			if idx < 0 {
				idx = 0
			}
			ch := runes[idx]

			switch style {
			case StyleText:
				buf = utf8.AppendRune(buf, ch)
			default: // StyleColor
				cr, cg, cb := int(r), int(g), int(b)
				if colorMode == ColorGrey {
					cg, cb = cr, cr
				}
				if o.ColorBlock {
					buf = append(buf, "\x1b[48;2;"...)
					buf = strconv.AppendInt(buf, int64(cr), 10)
					buf = append(buf, ';')
					buf = strconv.AppendInt(buf, int64(cg), 10)
					buf = append(buf, ';')
					buf = strconv.AppendInt(buf, int64(cb), 10)
					buf = append(buf, "m \x1b[0m"...)
				} else {
					buf = append(buf, "\x1b[38;2;"...)
					buf = strconv.AppendInt(buf, int64(cr), 10)
					buf = append(buf, ';')
					buf = strconv.AppendInt(buf, int64(cg), 10)
					buf = append(buf, ';')
					buf = strconv.AppendInt(buf, int64(cb), 10)
					buf = append(buf, "m"...)
					buf = utf8.AppendRune(buf, ch)
					buf = append(buf, "\x1b[0m"...)
				}
			}
		}
		buf = append(buf, '\n')
	}
	return string(buf)
}
