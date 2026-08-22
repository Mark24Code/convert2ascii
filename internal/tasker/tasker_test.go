package tasker

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestRunAllItems(t *testing.T) {
	const total = 100
	var seen atomic.Int32
	err := Run(4, total, func(i int) error {
		seen.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Load() != total {
		t.Fatalf("ran %d, want %d", seen.Load(), total)
	}
}

func TestRunError(t *testing.T) {
	err := Run(2, 5, func(i int) error {
		if i == 3 {
			return errors.New("boom")
		}
		return nil
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err=%v", err)
	}
}

func TestRunContinuesAfterError(t *testing.T) {
	const total = 5
	var seen atomic.Int32
	err := Run(2, total, func(i int) error {
		seen.Add(1)
		if i == 1 {
			return errors.New("boom")
		}
		return nil
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err=%v", err)
	}
	if seen.Load() != total {
		t.Fatalf("ran %d, want %d", seen.Load(), total)
	}
}

func TestRunParallelAllItems(t *testing.T) {
	const total = 100
	ch := make(chan int, 16)
	go func() {
		defer close(ch)
		for i := 0; i < total; i++ {
			ch <- i
		}
	}()
	var seen atomic.Int32
	err := RunParallel(4, total, ch, func(i int) error {
		seen.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Load() != total {
		t.Fatalf("ran %d, want %d", seen.Load(), total)
	}
}

func TestRunParallelError(t *testing.T) {
	ch := make(chan int, 16)
	go func() {
		defer close(ch)
		for i := 0; i < 5; i++ {
			ch <- i
		}
	}()
	err := RunParallel(2, 5, ch, func(i int) error {
		if i == 3 {
			return errors.New("boom")
		}
		return nil
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err=%v", err)
	}
}
