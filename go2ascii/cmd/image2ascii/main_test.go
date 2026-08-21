package main

import "testing"

func TestParseImageArgs(t *testing.T) {
	a, err := parseImageArgs([]string{"-i", "a.jpg", "-w", "80", "-b"})
	if err != nil {
		t.Fatal(err)
	}
	if a.image != "a.jpg" || a.width != 80 || !a.block {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseImageArgsStyle(t *testing.T) {
	a, err := parseImageArgs([]string{"--image", "a.jpg", "-s", "text"})
	if err != nil {
		t.Fatal(err)
	}
	if a.image != "a.jpg" || a.style != "text" {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseImageArgsHelp(t *testing.T) {
	a, err := parseImageArgs([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.help {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseImageArgsVersion(t *testing.T) {
	a, err := parseImageArgs([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.version {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseImageArgsUnknownFlag(t *testing.T) {
	if _, err := parseImageArgs([]string{"--nope"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}
