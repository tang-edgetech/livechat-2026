// Package routing implements overview.md §6.9's chat-assignment rules:
// manual (new chat lands pending, any Agent on that merchant can claim
// it) or round-robin (auto-assigned to the next currently-online Agent
// in rotation). Either way, an Admin/Super Admin can always manually
// reassign afterward — routing only decides the *default* assignment.
package routing

import (
	"context"
	"database/sql"

	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/presence"
)

// Route decides the initial agent_id/status for a brand-new chat.
func Route(ctx context.Context, conn *sql.DB, redisClient *redis.Client, merchantID int64) (agentID *int64, status string, err error) {
	var routingMode string
	var lastRouted sql.NullInt64
	if err := conn.QueryRow(
		`SELECT routing_mode, last_routed_agent_id FROM merchant WHERE id = ?`, merchantID,
	).Scan(&routingMode, &lastRouted); err != nil {
		return nil, "", err
	}

	if routingMode != "round_robin" {
		return nil, "pending", nil
	}

	agents, err := merchantAgentIDs(conn, merchantID)
	if err != nil {
		return nil, "", err
	}
	online, err := presence.OnlineUserIDs(ctx, redisClient)
	if err != nil {
		return nil, "", err
	}
	onlineSet := make(map[int64]bool, len(online))
	for _, id := range online {
		onlineSet[id] = true
	}

	var candidates []int64
	for _, id := range agents {
		if onlineSet[id] {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		// No one online to round-robin to — fall back to the manual
		// queue rather than assigning a chat to nobody.
		return nil, "pending", nil
	}

	next := candidates[0]
	if lastRouted.Valid {
		for i, id := range candidates {
			if id == lastRouted.Int64 {
				next = candidates[(i+1)%len(candidates)]
				break
			}
		}
	}

	if _, err := conn.Exec(`UPDATE merchant SET last_routed_agent_id = ? WHERE id = ?`, next, merchantID); err != nil {
		return nil, "", err
	}

	return &next, "active", nil
}

func merchantAgentIDs(conn *sql.DB, merchantID int64) ([]int64, error) {
	rows, err := conn.Query(
		`SELECT u.id FROM user u
		 JOIN role r ON r.id = u.role_id
		 JOIN user_merchant um ON um.user_id = u.id
		 WHERE r.slug = 'agent' AND um.merchant_id = ? AND u.status = 'active'
		 ORDER BY u.id`,
		merchantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
