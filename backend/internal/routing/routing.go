// Package routing implements overview.md §6.9's chat-assignment rules:
// manual (new chat lands pending, any Agent on that merchant can claim
// it) or round-robin (auto-assigned to the next currently-online Agent
// in rotation). Either way, an Admin/Super Admin can always manually
// reassign afterward — routing only decides the *default* assignment.
package routing

import (
	"context"
	"database/sql"
	"strings"

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
	next, err := pickOnline(ctx, conn, redisClient, merchantID, agents, lastRouted)
	if err != nil {
		return nil, "", err
	}
	if next == nil {
		// No one online to round-robin to — fall back to the manual
		// queue rather than assigning a chat to nobody.
		return nil, "pending", nil
	}

	return next, "active", nil
}

func merchantAgentIDs(conn *sql.DB, merchantID int64) ([]int64, error) {
	return queryAgentIDs(conn, merchantID, false)
}

func vipAgentIDs(conn *sql.DB, merchantID int64) ([]int64, error) {
	return queryAgentIDs(conn, merchantID, true)
}

func queryAgentIDs(conn *sql.DB, merchantID int64, vipOnly bool) ([]int64, error) {
	query := `SELECT u.id FROM user u
	          JOIN role r ON r.id = u.role_id
	          JOIN user_merchant um ON um.user_id = u.id
	          WHERE r.slug = 'agent' AND um.merchant_id = ? AND u.status = 'active'`
	if vipOnly {
		query += ` AND um.handles_vip = TRUE`
	}
	query += ` ORDER BY u.id`

	rows, err := conn.Query(query, merchantID)
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

// pickOnline round-robins through candidates (agent IDs already scoped to
// a merchant) restricted to whoever is currently online in Redis presence,
// continuing from merchant.last_routed_agent_id. Shared by Route and
// RouteVIP so both round-robin the same way over their respective pools.
func pickOnline(ctx context.Context, conn *sql.DB, redisClient *redis.Client, merchantID int64, candidates []int64, lastRouted sql.NullInt64) (*int64, error) {
	online, err := presence.OnlineUserIDs(ctx, redisClient)
	if err != nil {
		return nil, err
	}
	onlineSet := make(map[int64]bool, len(online))
	for _, id := range online {
		onlineSet[id] = true
	}

	var connectedCandidates []int64
	for _, id := range candidates {
		if onlineSet[id] {
			connectedCandidates = append(connectedCandidates, id)
		}
	}
	if len(connectedCandidates) == 0 {
		return nil, nil
	}

	onlineCandidates, err := filterAvailable(conn, connectedCandidates)
	if err != nil {
		return nil, err
	}
	if len(onlineCandidates) == 0 {
		return nil, nil
	}

	next := onlineCandidates[0]
	if lastRouted.Valid {
		for i, id := range onlineCandidates {
			if id == lastRouted.Int64 {
				next = onlineCandidates[(i+1)%len(onlineCandidates)]
				break
			}
		}
	}

	if _, err := conn.Exec(`UPDATE merchant SET last_routed_agent_id = ? WHERE id = ?`, next, merchantID); err != nil {
		return nil, err
	}
	return &next, nil
}

// RouteVIP decides the initial agent_id/status for a VIP visitor's chat
// (overview.md §6.9.1). Unlike Route, it ignores merchant.routing_mode
// entirely — a VIP client always gets a direct-routing attempt regardless
// of whether this merchant otherwise routes manually — and its pool is
// only agents flagged handles_vip for this merchant. Per the confirmed
// product decision, if no VIP-designated agent is currently online, it
// falls back to Route's normal pool rather than leaving the VIP client
// waiting for a dedicated agent specifically.
func RouteVIP(ctx context.Context, conn *sql.DB, redisClient *redis.Client, merchantID int64) (agentID *int64, status string, err error) {
	var lastRouted sql.NullInt64
	if err := conn.QueryRow(`SELECT last_routed_agent_id FROM merchant WHERE id = ?`, merchantID).Scan(&lastRouted); err != nil {
		return nil, "", err
	}

	vipAgents, err := vipAgentIDs(conn, merchantID)
	if err != nil {
		return nil, "", err
	}
	if len(vipAgents) > 0 {
		if next, err := pickOnline(ctx, conn, redisClient, merchantID, vipAgents, lastRouted); err != nil {
			return nil, "", err
		} else if next != nil {
			return next, "active", nil
		}
	}

	return Route(ctx, conn, redisClient, merchantID)
}

// filterAvailable narrows a candidate list (already known to hold a live
// WebSocket connection) down to whoever hasn't manually set themselves
// Offline (Sidebar's status toggle) — a plain SQL filter rather than
// another presence.* call since manual_status lives on `user`, not Redis.
func filterAvailable(conn *sql.DB, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := conn.Query(`SELECT id FROM user WHERE id IN (`+placeholders+`) AND manual_status = 'online'`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// AnyAvailableAgent reports whether at least one Agent for merchantID is
// both actually connected (ws.Hub presence) and hasn't set themselves
// Offline right now — the widget's "is anyone able to answer this" check
// (overview.md item: enquiry-form fallback when nobody's reachable).
// Read-only: unlike pickOnline it never mutates last_routed_agent_id.
func AnyAvailableAgent(ctx context.Context, conn *sql.DB, redisClient *redis.Client, merchantID int64) (bool, error) {
	agents, err := merchantAgentIDs(conn, merchantID)
	if err != nil {
		return false, err
	}
	if len(agents) == 0 {
		return false, nil
	}

	online, err := presence.OnlineUserIDs(ctx, redisClient)
	if err != nil {
		return false, err
	}
	onlineSet := make(map[int64]bool, len(online))
	for _, id := range online {
		onlineSet[id] = true
	}

	var connected []int64
	for _, id := range agents {
		if onlineSet[id] {
			connected = append(connected, id)
		}
	}
	if len(connected) == 0 {
		return false, nil
	}

	available, err := filterAvailable(conn, connected)
	if err != nil {
		return false, err
	}
	return len(available) > 0, nil
}
