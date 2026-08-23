package main

import "testing"

func TestParseVideoArgsRemovedFlagsRejected(t *testing.T) {
	for _, args := range [][]string{
		{"-o", "out"},
		{"--output", "out"},
		{"-p", "frames"},
		{"--play_dir", "frames"},
	} {
		if _, err := parseVideoArgs(args); err == nil {
			t.Fatalf("expected error for removed flag %v", args)
		}
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
