package jira

import (
	"context"
	"sync"
	"time"
)

// limiter caps parallel requests and adapts to rate limits: after a
// 429 it allows one request at a time until five succeed in a row
// and pause() holds every request until a time.
type limiter struct {
	mu         sync.Mutex
	max        int
	limit      int
	inFlight   int
	okStreak   int
	pauseUntil time.Time
	wake       chan struct{} // closed on every state change, then replaced
	now        func() time.Time
}

const recoverAfter = 5

func newLimiter(max int, now func() time.Time) *limiter {
	return &limiter{max: max, limit: max, wake: make(chan struct{}), now: now}
}

func (l *limiter) acquire(ctx context.Context) error {
	for {
		l.mu.Lock()
		wait := l.pauseUntil.Sub(l.now())
		if wait <= 0 && l.inFlight < l.limit {
			l.inFlight++
			l.mu.Unlock()
			return nil
		}
		wake := l.wake
		l.mu.Unlock()

		var timer *time.Timer
		var fire <-chan time.Time
		if wait > 0 {
			timer = time.NewTimer(wait)
			fire = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return ctx.Err()
		case <-wake:
		case <-fire:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (l *limiter) release(ok, throttled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.inFlight--
	switch {
	case throttled:
		l.limit, l.okStreak = 1, 0
	case ok:
		l.okStreak++
		if l.limit < l.max && l.okStreak >= recoverAfter {
			l.limit = l.max
		}
	}
	l.broadcast()
}

func (l *limiter) pause(until time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until.After(l.pauseUntil) {
		l.pauseUntil = until
	}
	l.broadcast()
}

func (l *limiter) current() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

func (l *limiter) broadcast() {
	close(l.wake)
	l.wake = make(chan struct{})
}
