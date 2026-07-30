package redisclient

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/config"
)

// Connect returns a Redis client if reachable, or (nil, err) otherwise.
// Redis is required for multi-instance WebSocket pub/sub (overview.md §2)
// but optional for Phase 0 — callers should warn, not fail, if it's down.
func Connect(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return client, nil
}
