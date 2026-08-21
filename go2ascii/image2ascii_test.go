package convert2ascii_test

import (
	"strings"
	"testing"

	"github.com/Mark24Code/convert2ascii/go2ascii"
)

// From the go2ascii root package, repo test assets are one level up.
const testRuby = "../test/assets/ruby.jpg"

func TestImage2AsciiTextNonEmpty(t *testing.T) {
	a := convert2ascii.NewImage2Ascii(convert2ascii.ImageOptions{URI: testRuby, Width: 20, Style: "text"})
	if err := a.Generate(); err != nil {
		t.Fatal(err)
	}
	if len(a.String()) == 0 {
		t.Fatal("empty output")
	}
}

func TestImage2AsciiColorNonEmpty(t *testing.T) {
	a := convert2ascii.NewImage2Ascii(convert2ascii.ImageOptions{URI: testRuby, Width: 20, Style: "color"})
	if err := a.Generate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.String(), "\x1b[") {
		t.Fatal("color style missing ANSI")
	}
}

func TestImage2AsciiColorBlockNonEmpty(t *testing.T) {
	a := convert2ascii.NewImage2Ascii(convert2ascii.ImageOptions{URI: testRuby, Width: 20, Style: "color", ColorBlock: true})
	if err := a.Generate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.String(), "\x1b[48;2;") {
		t.Fatal("color_block style missing background ANSI")
	}
}

func TestImage2AsciiMissingFile(t *testing.T) {
	a := convert2ascii.NewImage2Ascii(convert2ascii.ImageOptions{URI: "does-not-exist.jpg"})
	if err := a.Generate(); err == nil {
		t.Fatal("expected error for missing file")
	}
}
