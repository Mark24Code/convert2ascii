package imagelib

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/draw"
)

func TestAspectSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100)) // 2:1
	w, h := AspectSize(img, 50)
	if w != 50 || h != 12 { // round(100*50/200)=25, /2=12
		t.Fatalf("got %dx%d, want 50x12", w, h)
	}
}

func TestAspectSizeWH(t *testing.T) {
	cases := []struct{ sw, sh, width, w, h int }{
		{1280, 720, 80, 80, 22}, // round(720*80/1280)=45, /2=22
		{200, 100, 50, 50, 12},
		{640, 480, 40, 40, 15}, // round(480*40/640)=30, /2=15
	}
	for _, c := range cases {
		w, h := AspectSizeWH(c.sw, c.sh, c.width)
		if w != c.w || h != c.h {
			t.Fatalf("AspectSizeWH(%d,%d,%d) = %dx%d, want %dx%d", c.sw, c.sh, c.width, w, h, c.w, c.h)
		}
	}
}

// TestRenderRGBAEqualsRender asserts the pre-scaled path produces identical
// output to the rescale path for the same target pixels, across styles.
func TestRenderRGBAEqualsRender(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x % 256), G: uint8((x + y) % 256), B: uint8(y % 256), A: 255})
		}
	}
	w, h := AspectSize(img, 50)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	for _, o := range []Options{
		{Width: 50, Style: StyleText},
		{Width: 50, Style: StyleColor},
		{Width: 50, Style: StyleColor, ColorBlock: true},
		{Width: 50, Style: StyleColor, Color: ColorGrey},
	} {
		if got, want := RenderRGBA(dst, o), Render(img, o); got != want {
			t.Fatalf("RenderRGBA != Render for %+v\n--- got ---\n%s\n--- want ---\n%s", o, got, want)
		}
	}
}

func TestDefaultCharsLength(t *testing.T) {
	if len(DefaultChars) != 69 {
		t.Fatalf("DefaultChars length = %d, want 69", len(DefaultChars))
	}
}

func TestRenderTextAllWhiteUsesLastChar(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	out := Render(img, Options{Width: 8, Style: StyleText})
	last := string(DefaultChars[len(DefaultChars)-1])
	got := strings.Split(out, "\n")[0]
	if !strings.HasPrefix(got, strings.Repeat(last, 8)) {
		t.Fatalf("white image did not map to last char: %q", got)
	}
}

func TestRenderTextBlackUsesFirstChar(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	out := Render(img, Options{Width: 8, Style: StyleText})
	first := string(DefaultChars[0])
	got := strings.Split(out, "\n")[0]
	if !strings.HasPrefix(got, strings.Repeat(first, 8)) {
		t.Fatalf("black image did not map to first char: %q", got)
	}
}

func TestRenderOutputLineCount(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	out := Render(img, Options{Width: 20, Style: StyleText})
	lines := strings.Split(out, "\n")
	// 40x40 -> scaledRows=round(40*20/40)=20 -> h=20/2=10 rows; output ends
	// with "\n", so Split yields 11 elements (10 rows + trailing empty).
	if len(lines) != 11 {
		t.Fatalf("line count = %d, want 11", len(lines))
	}
	if len(strings.Split(out, "\n")[0]) != 20 {
		t.Fatalf("first row width = %d, want 20", len(strings.Split(out, "\n")[0]))
	}
}

func TestRenderColorStyleAnsi(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 0, B: 0, A: 255}}, image.Point{}, draw.Src)
	out := Render(img, Options{Width: 4, Style: StyleColor})
	if !strings.Contains(out, "\x1b[38;2;255;0;0m") {
		t.Fatalf("color style missing fg escape: %q", out)
	}
}

func TestRenderColorBlockStyleAnsi(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	out := Render(img, Options{Width: 4, Style: StyleColor, ColorBlock: true})
	if !strings.Contains(out, "\x1b[48;2;") {
		t.Fatalf("color_block style missing bg escape: %q", out)
	}
}

func TestRenderGreyStyle(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.NRGBA{R: 255, G: 0, B: 0, A: 255}}, image.Point{}, draw.Src)
	out := Render(img, Options{Width: 4, Style: StyleColor, Color: ColorGrey})
	if !strings.Contains(out, "\x1b[38;2;255;255;255") {
		t.Fatalf("greyscale style did not set r=g=b: %q", out)
	}
}

func TestOpenLocalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := Open(path)
	if err != nil {
		t.Fatalf("Open(local png) error: %v", err)
	}
	if got.Bounds() != img.Bounds() {
		t.Fatalf("decoded bounds = %v, want %v", got.Bounds(), img.Bounds())
	}
}

func TestOpenHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if _, err := Open(srv.URL + "/missing.png"); err == nil {
		t.Fatal("Open(404) expected error, got nil")
	}
}
