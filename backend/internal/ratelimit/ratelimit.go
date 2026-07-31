// Package ratelimit is the v1 abuse-protection baseline from overview.md
// §10.6: per-IP and per-visitor (phone/fingerprint) limits on chat-start
// and message-send, no CAPTCHA. In-memory and per-instance — fine at
// single-instance scale; if Phase 7 activates multiple Go instances,
// this would need to move to Redis (INCR + EXPIRE) the same way presence
// already did, but that's a config change to this package, not a
// call-site rewrite.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	visits map[string][]time.Time
}

func New(window time.Duration, max int) *Limiter {
	return &Limiter{window: window, max: max, visits: make(map[string][]time.Time)}
}

// Allow reports whether `key` (e.g. "chat-start:<ip>") is still under its
// limit, recording this attempt if so. Old entries are pruned lazily on
// each call rather than with a separate sweep goroutine.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	kept := l.visits[key][:0]
	for _, t := range l.visits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= l.max {
		l.visits[key] = kept
		return false
	}

	l.visits[key] = append(kept, now)
	return true
}
