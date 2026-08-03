// Package lock is a tiny Redis-backed mutual-exclusion helper for
// overview.md §11 Phase 7 ("activate multi-instance Go"). Without it,
// every per-instance background ticker (the inactivity sweeper, the
// retention purge scheduler) would run its own copy on every instance in
// the fleet each tick — not incorrect (every query/update is idempotent),
// but it multiplies DB load and, worse, fires a duplicate audit_log entry
// and outbound webhook per instance for the same real-world event. If
// Redis is unreachable, TryAcquire always succeeds (fail-open) so a
// single-instance or Redis-less dev setup keeps working exactly as
// before — this is purely an optimization for when more than one
// instance is actually running, not a correctness dependency.
package lock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// TryAcquire reports whether the caller won the lock named `key` for
// `ttl`. Backed by Redis SET NX — first instance to call it within the
// window wins; everyone else gets false until the key expires.
func TryAcquire(client *redis.Client, key string, ttl time.Duration) bool {
	if client == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := client.SetNX(ctx, "lock:"+key, "1", ttl).Result()
	if err != nil {
		// Can't confirm exclusivity — fail open rather than let a Redis
		// hiccup silently stop every instance from ever sweeping/purging.
		return true
	}
	return ok
}
