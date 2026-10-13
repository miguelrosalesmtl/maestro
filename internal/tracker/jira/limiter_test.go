package jira

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterCapsConcurrency(t *testing.T) {
	l := newLimiter(3, time.Now)
	var inFlight, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.acquire(context.Background()); err != nil {
				t.Error(err)
				return
			}
			n := inFlight.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			inFlight.Add(-1)
			l.release(true, false)
		}()
	}
	wg.Wait()
	if peak.Load() > 3 {
		t.Fatalf("peak concurrency %d > 3", peak.Load())
	}
}

func TestLimiterPauseAndCancel(t *testing.T) {
	l := newLimiter(2, time.Now)
	l.pause(time.Now().Add(time.Hour))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.acquire(ctx); err == nil {
		t.Fatal("acquire must wait during a pause and honour cancellation")
	}
}
