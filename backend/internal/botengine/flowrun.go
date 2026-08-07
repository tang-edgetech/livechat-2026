package botengine

import (
	"database/sql"
	"log"
)

// This file is Bot Analytics' write side — one bot_flow_run row per chat
// that ever entered bot mode (see migrations/0009_bot_flow_analytics.sql),
// so an Admin can see completion/handoff/abandonment rates and per-node
// drop-off instead of building a flow blind. Every function here is
// best-effort (log to stderr, never bubble up) — same contract as
// audit.Log, since a bad analytics write must never break a live
// conversation.

// recordFlowRunStart is called once from TryStart, covering both "steps"
// and "ai_passthrough" modes with the same call site.
func recordFlowRunStart(conn *sql.DB, flowID, chatID, merchantID int64) {
	if _, err := conn.Exec(
		`INSERT INTO bot_flow_run (bot_flow_id, chat_id, merchant_id) VALUES (?, ?, ?)`,
		flowID, chatID, merchantID,
	); err != nil {
		log.Printf("bot_flow_run insert failed for chat %d: %v", chatID, err)
	}
}

// recordNode mirrors persistState's own nodeID exactly — "" means either
// "no ask_question pending yet" or "the graph ran off the end", and the
// latter is also this run's natural completion, so it also resolves the
// outcome to 'completed'.
func (fc *flowContext) recordNode(nodeID string) {
	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}
	if _, err := fc.conn.Exec(`UPDATE bot_flow_run SET last_node_id = ? WHERE chat_id = ?`, nodeArg, fc.chatID); err != nil {
		log.Printf("bot_flow_run node update failed for chat %d: %v", fc.chatID, err)
	}
	if nodeID == "" {
		fc.recordOutcome("completed", "engine")
	}
}

func (fc *flowContext) recordOutcome(outcome, resolvedBy string) {
	markResolved(fc.conn, fc.chatID, outcome, resolvedBy)
}

// MarkHandoffByStaff/MarkClosedByStaff are exported for
// handlers/chat.go's ClaimChatHandler, AssignChatHandler, and
// CloseChatHandler — staff can pull a chat out of 'bot' status directly
// (claim/reassign/close) without ever going through handoffToAgent()/
// closeChat(), so those call sites resolve bot_flow_run themselves.
func MarkHandoffByStaff(conn *sql.DB, chatID int64) {
	markResolved(conn, chatID, "handoff", "staff")
}

func MarkClosedByStaff(conn *sql.DB, chatID int64) {
	markResolved(conn, chatID, "closed", "staff")
}

// MarkAbandonedBySweep is exported for inactivity/sweep.go, called only
// for chats whose status was 'bot' at close time. The outcome='active'
// guard inside markResolved means a chat that already completed (graph
// ran off the end but chat.status never left 'bot' — a pre-existing
// orphaning quirk the sweep now also cleans up) just gets closed without
// its outcome being overwritten, since it already resolved as 'completed'.
func MarkAbandonedBySweep(conn *sql.DB, chatID int64) {
	markResolved(conn, chatID, "abandoned", "sweep")
}

func markResolved(conn *sql.DB, chatID int64, outcome, resolvedBy string) {
	if _, err := conn.Exec(
		`UPDATE bot_flow_run SET outcome = ?, resolved_by = ?, ended_at = NOW() WHERE chat_id = ? AND outcome = 'active'`,
		outcome, resolvedBy, chatID,
	); err != nil {
		log.Printf("bot_flow_run resolve failed for chat %d: %v", chatID, err)
	}
}
