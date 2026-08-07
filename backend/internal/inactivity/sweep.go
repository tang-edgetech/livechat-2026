// Package inactivity auto-closes a chat after too many minutes of no
// Visitor activity (overview.md §10.5, merchant.inactivity_timeout_minutes,
// default 30). Deliberately keyed off the visitor's own last message
// (falling back to chat start if they've never sent one) — an Agent
// still typing away doesn't count, this is specifically about the
// visitor having gone quiet.
package inactivity

import (
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/botengine"
	"livechat/backend/internal/lock"
	"livechat/backend/internal/webhook"
	"livechat/backend/internal/ws"
)

// StartSweeper runs the sweep on a ticker. redisClient may be nil (no
// Redis configured); the lock is purely an optimization for when more
// than one Go instance is running (overview.md §11 Phase 7) so they
// don't all close, publish, and webhook-notify the same stale chat —
// every instance still sweeps correctly on its own if Redis is absent.
func StartSweeper(state *appstate.State, hub *ws.Hub, redisClient *redis.Client, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if !lock.TryAcquire(redisClient, "inactivity-sweep", interval-time.Second) {
				continue
			}
			sweep(state, hub)
		}
	}()
}

func sweep(state *appstate.State, hub *ws.Hub) {
	conn := state.DB()
	if conn == nil {
		return
	}

	rows, err := conn.Query(`
		SELECT c.id, c.uuid, c.visitor_id, c.merchant_id, c.status
		FROM chat c
		JOIN merchant m ON m.id = c.merchant_id
		WHERE c.status IN ('active', 'pending', 'bot')
		AND TIMESTAMPDIFF(MINUTE, COALESCE(
			(SELECT MAX(created_at) FROM message WHERE chat_id = c.id AND sender_type = 'visitor'),
			c.started_at
		), NOW()) >= m.inactivity_timeout_minutes
	`)
	if err != nil {
		log.Printf("inactivity sweep query failed: %v", err)
		return
	}

	type stale struct {
		id, visitorID, merchantID int64
		uuid, status              string
	}
	var toClose []stale
	for rows.Next() {
		var s stale
		if err := rows.Scan(&s.id, &s.uuid, &s.visitorID, &s.merchantID, &s.status); err != nil {
			rows.Close()
			log.Printf("inactivity sweep scan failed: %v", err)
			return
		}
		toClose = append(toClose, s)
	}
	rows.Close()

	for _, s := range toClose {
		if _, err := conn.Exec(`UPDATE chat SET status = 'closed', closed_at = NOW() WHERE id = ?`, s.id); err != nil {
			log.Printf("inactivity sweep close failed for chat %d: %v", s.id, err)
			continue
		}
		// A chat stuck at an ask_question node with no reply, or an
		// orphaned "ran off the end of the graph with no terminal step"
		// bot flow, previously lingered forever — this was the one status
		// the sweep excluded. Bot Analytics needs every exit path
		// captured, so it now closes on the same clock as active/pending.
		if s.status == "bot" {
			botengine.MarkAbandonedBySweep(conn, s.id)
		}
		hub.Publish(ws.VisitorSubject(s.visitorID), ws.Event{Type: "chat_closed", Data: map[string]string{"chatUuid": s.uuid, "reason": "inactivity"}})
		hub.Publish(ws.DashboardSubject(s.merchantID), ws.Event{Type: "chat_updated"})
		webhook.Dispatch(conn, s.merchantID, "chat.closed", map[string]string{"chatUuid": s.uuid, "reason": "inactivity"})
	}
}
