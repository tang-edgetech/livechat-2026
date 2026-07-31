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

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/ws"
)

func StartSweeper(state *appstate.State, hub *ws.Hub, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
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
		SELECT c.id, c.uuid, c.visitor_id, c.merchant_id
		FROM chat c
		JOIN merchant m ON m.id = c.merchant_id
		WHERE c.status IN ('active', 'pending')
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
		uuid                      string
	}
	var toClose []stale
	for rows.Next() {
		var s stale
		if err := rows.Scan(&s.id, &s.uuid, &s.visitorID, &s.merchantID); err != nil {
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
		hub.Publish(ws.VisitorSubject(s.visitorID), ws.Event{Type: "chat_closed", Data: map[string]string{"chatUuid": s.uuid, "reason": "inactivity"}})
		hub.Publish(ws.DashboardSubject(s.merchantID), ws.Event{Type: "chat_updated"})
	}
}
