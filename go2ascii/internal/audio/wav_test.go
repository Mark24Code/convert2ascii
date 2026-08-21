package audio

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeWAVHeader(t *testing.T) {
	pcm := make([]byte, 8) // 2 frames stereo s16
	b, err := EncodeWAV(44100, 2, pcm)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("RIFF")) || !bytes.Contains(b[:12], []byte("WAVE")) {
		t.Fatalf("bad magic: % x", b[:12])
	}
	// fmt chunk: subchunk1 size 16, PCM=1
	if binary.LittleEndian.Uint32(b[16:20]) != 16 {
		t.Fatal("fmt chunk size wrong")
	}
	if binary.LittleEndian.Uint16(b[20:22]) != 1 {
		t.Fatal("audio format != PCM")
	}
	if binary.LittleEndian.Uint16(b[22:24]) != 2 {
		t.Fatal("channels wrong")
	}
	if binary.LittleEndian.Uint32(b[24:28]) != 44100 {
		t.Fatal("sample rate wrong")
	}
	if binary.LittleEndian.Uint32(b[28:32]) != 44100*2*2 {
		t.Fatal("byte rate wrong")
	}
	if binary.LittleEndian.Uint16(b[34:36]) != 16 {
		t.Fatal("bits per sample wrong")
	}
	if string(b[36:40]) != "data" {
		t.Fatal("data chunk missing")
	}
	if binary.LittleEndian.Uint32(b[40:44]) != uint32(len(pcm)) {
		t.Fatal("data size wrong")
	}
}

func TestEncodeWAVRejectsOddPCM(t *testing.T) {
	if _, err := EncodeWAV(44100, 2, make([]byte, 3)); err == nil {
		t.Fatal("expected error for odd-length pcm")
	}
}

// TestWAVWriterMatchesEncodeWAV asserts the streaming writer produces
// byte-identical output to the one-shot encoder when written in chunks.
func TestWAVWriterMatchesEncodeWAV(t *testing.T) {
	pcm := make([]byte, 0, 1000)
	for i := 0; i < 500; i++ {
		pcm = append(pcm, byte(i), byte(i*7))
	}
	want, err := EncodeWAV(48000, 1, pcm)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "a.wav")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	ww, err := NewWAVWriter(f, 48000, 1)
	if err != nil {
		t.Fatal(err)
	}
	half := len(pcm) / 2
	if _, err := ww.Write(pcm[:half]); err != nil {
		t.Fatal(err)
	}
	if _, err := ww.Write(pcm[half:]); err != nil {
		t.Fatal(err)
	}
	if err := ww.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("streaming WAVWriter output != EncodeWAV output")
	}
}
