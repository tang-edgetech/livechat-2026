// Package ratelimit is the v1 abuse-protection baseline from overview.md
// §10.6: per-IP and per-visitor (phone/fingerprint) limits on chat-start
// and message-send, no CAPTCHA (see the Decisions Log — CAPTCHA is
// deferred by design until real abuse is observed, not something this
// phase adds). NewRedis backs the limiter with Redis (INCR + EXPIRE) so
// the same limit holds across every Go instance sharing that Redis —
// overview.md §11 Phase 7 ("activate multi-instance Go"), the same move
// the package comment already flagged as coming. New (in-memory,
// sliding-window) still exists for callers that only ever run
// single-instance, and NewRedis itself falls back to that same in-memory
// path if Redis is unreachable — same fail-open-to-local-only posture as
// ws.Hub and presence.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	visits map[string][]time.Time

	redis *redis.Client // nil = in-memory only
}

func New(window time.Duration, max int) *Limiter {
	return &Limiter{window: window, max: max, visits: make(map[string][]time.Time)}
}

func NewRedis(redisClient *redis.Client, window time.Duration, max int) *Limiter {
	return &Limiter{window: window, max: max, visits: make(map[string][]time.Time), redis: redisClient}
}

// Allow reports whether `key` (e.g. "chat-start:<ip>") is still under its
// limit, recording this attempt if so.
func (l *Limiter) Allow(key string) bool {
	if l.redis != nil {
		if ok, err := l.allowRedis(key); err == nil {
			return ok
		}
		// Redis call failed (not just "no client") — fall through to
		// local-only rather than either blocking or admitting everything.
	}
	return l.allowLocal(key)
}

// allowRedis is a fixed-window counter: INCR on first hit sets the TTL,
// every hit after that just increments. Slightly less precise at window
// boundaries than the sliding-window local path, but it's O(1) per call
// and exactly correct across instances, which is what matters here.
func (l *Limiter) allowRedis(key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisKey := "ratelimit:" + key
	count, err := l.redis.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		l.redis.Expire(ctx, redisKey, l.window)
	}
	return count <= int64(l.max), nil
}

// allowLocal is the original per-instance sliding-window path; old
// entries are pruned lazily on each call rather than with a separate
// sweep goroutine.
func (l *Limiter) allowLocal(key string) bool {
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
