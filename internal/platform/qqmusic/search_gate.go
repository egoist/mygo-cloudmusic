package qqmusic

import (
	"sync"
	"time"
)

// searchGate paces one endpoint and account. Failures open a short circuit;
// another endpoint remains available and a later request can try again.
type searchGate struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
	tokens   float64
	until    time.Time
	strikes  int
	cause    error
}

func (g *searchGate) reserve(now time.Time) (time.Duration, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if now.Before(g.until) {
		return 0, &SearchCooldownError{RetryAfter: g.until.Sub(now), Cause: g.cause}
	}
	if g.interval <= 0 {
		return 0, nil
	}
	if g.last.IsZero() {
		g.tokens, g.last = 3, now
	}
	if elapsed := now.Sub(g.last); elapsed > 0 {
		g.tokens = min(3, g.tokens+float64(elapsed)/float64(g.interval))
		g.last = now
	}
	if g.tokens >= 1 {
		g.tokens--
		return 0, nil
	}
	return time.Duration((1 - g.tokens) * float64(g.interval)), nil
}

func (g *searchGate) failed(now time.Time, cause error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.strikes = min(g.strikes+1, 3)
	g.until = now.Add((6 * time.Second) << (g.strikes - 1))
	g.tokens, g.last, g.cause = 0, now, cause
}

func (g *searchGate) succeeded(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// A request already in flight when another failed cannot end its active cooldown.
	if now.Before(g.until) {
		return
	}
	g.strikes, g.until, g.cause = 0, time.Time{}, nil
}
