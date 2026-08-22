// Package tasker provides a goroutine worker pool that renders frames in
// parallel and reports a Ruby-style progress line.
package tasker

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Run processes indexes 0..total-1 with n workers, calling fn for each.
// fn may be called concurrently. The first error is returned; remaining
// items still run. Progress is printed to stdout in the Ruby format:
//
//	processing...  xx.xx % (time: xx.xx s)
//
// If total == 0, the percentage is omitted.
func Run(n, total int, fn func(i int) error) error {
	if n < 1 {
		n = 1
	}
	ch := make(chan int, n)
	go func() {
		defer close(ch)
		for i := 0; i < total; i++ {
			ch <- i
		}
	}()
	return RunParallel(n, total, ch, fn)
}

// RunParallel processes items from ch with n workers. Unlike Run, the item
// channel is closed by the producer; progress total is advisory (used for the
// percentage; total <= 0 shows a frame count instead). fn may be concurrent.
func RunParallel[T any](n int, total int, ch <-chan T, fn func(T) error) error {
	if n < 1 {
		n = 1
	}
	start := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var done int
	var firstErr error
	var errMu sync.Mutex
	report := func() {
		if total <= 0 {
			fmt.Printf("\rprocessing...  frames: %d (time: %.2f s)", done, time.Since(start).Seconds())
		} else {
			fmt.Printf("\rprocessing...  %.2f %% (time: %.2f s)",
				100.0*float64(done)/float64(total), time.Since(start).Seconds())
		}
	}
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range ch {
				if err := fn(item); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				mu.Lock()
				done++
				report()
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Fprint(os.Stdout, "\r\x1b[K")
	return firstErr
}
