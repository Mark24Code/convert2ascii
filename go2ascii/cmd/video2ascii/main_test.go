package main

import "testing"

func TestParseVideoArgsPlayDir(t *testing.T) {
	a, err := parseVideoArgs([]string{"-p", "frames", "--loop"})
	if err != nil {
		t.Fatal(err)
	}
	if a.playDir != "frames" || !a.loop {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseVideoArgsOutputAlias(t *testing.T) {
	a, err := parseVideoArgs([]string{"-i", "a.mp4", "--output", "out"})
	if err != nil {
		t.Fatal(err)
	}
	if a.input != "a.mp4" || a.output != "out" {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseVideoArgsHelp(t *testing.T) {
	a, err := parseVideoArgs([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.help {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseVideoArgsVersion(t *testing.T) {
	a, err := parseVideoArgs([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.version {
		t.Fatalf("a=%+v", a)
	}
}

func TestParseVideoArgsUnknownFlag(t *testing.T) {
	if _, err := parseVideoArgs([]string{"--nope"}); err == nil {
		t.Fatal("expected unknown flag error")
	}
}
