// Package presence tracks which Agents currently hold a live WebSocket
// connection — used for the "Online Agents" dashboard stat and for
// round-robin routing (overview.md §6.9: "the next Agent in rotation
// among that merchant's currently online Agents"). Backed by Redis (a
// hash of userID -> open-connection-count) rather than the ws.Hub's local
// map, since presence must be correct across multiple Go instances.
package presence

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const onlineKey = "presence:online"

// Connect increments the open-connection count for userID — a user with
// two browser tabs open is still "one online agent", counted down to zero
// only once every tab has disconnected.
func Connect(ctx context.Context, client *redis.Client, userID int64) error {
	if client == nil {
		return nil
	}
	return client.HIncrBy(ctx, onlineKey, strconv.FormatInt(userID, 10), 1).Err()
}

// Disconnect decrements the count and removes the field entirely once it
// reaches zero, so OnlineUserIDs never has to filter out zero-valued keys.
func Disconnect(ctx context.Context, client *redis.Client, userID int64) error {
	if client == nil {
		return nil
	}
	field := strconv.FormatInt(userID, 10)
	remaining, err := client.HIncrBy(ctx, onlineKey, field, -1).Result()
	if err != nil {
		return err
	}
	if remaining <= 0 {
		return client.HDel(ctx, onlineKey, field).Err()
	}
	return nil
}

// OnlineUserIDs returns every currently-online user id. Small dataset by
// construction (staff headcount, not visitors) — safe to fetch in full
// and intersect with a merchant's roster in application code.
func OnlineUserIDs(ctx context.Context, client *redis.Client) ([]int64, error) {
	if client == nil {
		return nil, nil
	}
	fields, err := client.HKeys(ctx, onlineKey).Result()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(fields))
	for _, f := range fields {
		id, err := strconv.ParseInt(f, 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
