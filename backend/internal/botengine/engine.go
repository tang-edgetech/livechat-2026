// Package botengine interprets the bot_flow node-graph shape from
// overview.md §4/§6.4: a Trigger, then chained tools/actions. It's a
// synchronous, in-request interpreter — no separate worker queue — which
// is why `delay` is a documented no-op (§6.4 calls a canvas/async engine
// a possible later upgrade; v1 stays on this same request-response model
// the rest of the app uses).
package botengine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/automation"
	"livechat/backend/internal/routing"
	"livechat/backend/internal/ws"
)

type Node struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Config   map[string]any    `json:"config"`
	Next     string            `json:"next,omitempty"`
	Branches map[string]string `json:"branches,omitempty"`
}

type FlowDef struct {
	Nodes []Node `json:"nodes"`
	Entry string `json:"entry"`
}

type TriggerDef struct {
	Type       string                  `json:"type"`
	Keyword    string                  `json:"keyword"`
	Conditions automation.ConditionSet `json:"conditions"`
}

type botMessage struct {
	ID         int64     `json:"id"`
	ChatUUID   string    `json:"chat_uuid,omitempty"`
	SenderType string    `json:"sender_type"`
	Body       string    `json:"body"`
	Type       string    `json:"type"`
	Metadata   *string   `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type candidateFlow struct {
	ID      int64
	Trigger string
	Flow    string
}

// TryStart looks for the first active chat_start bot flow in scope for
// merchantID whose trigger conditions match ctx, and if found, puts the
// chat into bot-driven mode and runs it from the entry node. Returns
// false (not an error) when nothing matches — the caller falls back to
// normal routing (overview.md §6.9), exactly as if Phase 4 didn't exist.
func TryStart(ctx context.Context, conn *sql.DB, hub *ws.Hub, redisClient *redis.Client, chatID, merchantID int64, evalCtx map[string]string) (bool, error) {
	rows, err := conn.Query(
		`SELECT DISTINCT bf.id, bf.trigger_config, bf.flow FROM bot_flow bf
		 LEFT JOIN bot_flow_merchant bfm ON bfm.bot_flow_id = bf.id
		 WHERE bf.is_active = TRUE AND (bf.is_global = TRUE OR bfm.merchant_id = ?)
		 ORDER BY bf.created_at ASC`,
		merchantID,
	)
	if err != nil {
		return false, err
	}
	var candidates []candidateFlow
	for rows.Next() {
		var f candidateFlow
		if err := rows.Scan(&f.ID, &f.Trigger, &f.Flow); err != nil {
			rows.Close()
			return false, err
		}
		candidates = append(candidates, f)
	}
	rows.Close()

	for _, f := range candidates {
		var trigger TriggerDef
		if err := json.Unmarshal([]byte(f.Trigger), &trigger); err != nil {
			continue
		}
		if trigger.Type != "chat_start" {
			continue
		}
		if !automation.Evaluate(trigger.Conditions, evalCtx) {
			continue
		}

		var flow FlowDef
		if err := json.Unmarshal([]byte(f.Flow), &flow); err != nil {
			continue
		}

		if _, err := conn.Exec(
			`UPDATE chat SET status = 'bot', bot_flow_id = ?, bot_node_id = NULL, bot_variables = '{}' WHERE id = ?`,
			f.ID, chatID,
		); err != nil {
			return false, err
		}

		fc, err := newFlowContext(conn, hub, redisClient, chatID, f.ID, flow)
		if err != nil {
			return false, err
		}
		return true, fc.run(ctx, flow.Entry)
	}
	return false, nil
}

// ContinueOnVisitorMessage feeds a visitor's message into the flow as
// the answer to whatever ask_question node is currently pending. A
// no-op (handled=false) if this chat isn't currently bot-driven.
func ContinueOnVisitorMessage(ctx context.Context, conn *sql.DB, hub *ws.Hub, redisClient *redis.Client, chatID int64, messageBody string) (bool, error) {
	var botFlowID sql.NullInt64
	var nodeID sql.NullString
	var variablesRaw sql.NullString
	var status string
	if err := conn.QueryRow(
		`SELECT status, bot_flow_id, bot_node_id, bot_variables FROM chat WHERE id = ?`, chatID,
	).Scan(&status, &botFlowID, &nodeID, &variablesRaw); err != nil {
		return false, err
	}
	if status != "bot" || !botFlowID.Valid || !nodeID.Valid {
		return false, nil
	}

	var flowRaw string
	if err := conn.QueryRow(`SELECT flow FROM bot_flow WHERE id = ?`, botFlowID.Int64).Scan(&flowRaw); err != nil {
		return false, err
	}
	var flow FlowDef
	if err := json.Unmarshal([]byte(flowRaw), &flow); err != nil {
		return false, err
	}

	fc, err := newFlowContext(conn, hub, redisClient, chatID, botFlowID.Int64, flow)
	if err != nil {
		return false, err
	}
	if variablesRaw.Valid {
		json.Unmarshal([]byte(variablesRaw.String), &fc.variables)
	}

	node := fc.find(nodeID.String)
	if node == nil || node.Type != "ask_question" {
		return false, nil
	}
	varName, _ := node.Config["variable"].(string)
	if varName == "" {
		varName = "last_answer"
	}
	fc.variables[varName] = messageBody

	return true, fc.run(ctx, node.Next)
}

type flowContext struct {
	conn        *sql.DB
	hub         *ws.Hub
	redisClient *redis.Client
	chatID      int64
	visitorID   int64
	merchantID  int64
	chatUUID    string
	flowID      int64
	flow        FlowDef
	variables   map[string]any
}

func newFlowContext(conn *sql.DB, hub *ws.Hub, redisClient *redis.Client, chatID, flowID int64, flow FlowDef) (*flowContext, error) {
	fc := &flowContext{conn: conn, hub: hub, redisClient: redisClient, chatID: chatID, flowID: flowID, flow: flow, variables: map[string]any{}}
	err := conn.QueryRow(`SELECT visitor_id, merchant_id, uuid FROM chat WHERE id = ?`, chatID).Scan(&fc.visitorID, &fc.merchantID, &fc.chatUUID)
	return fc, err
}

func (fc *flowContext) find(id string) *Node {
	for i := range fc.flow.Nodes {
		if fc.flow.Nodes[i].ID == id {
			return &fc.flow.Nodes[i]
		}
	}
	return nil
}

// run walks the flow from `current` until it hits a pausing node
// (ask_question), a terminal node (handoff_to_agent/close_chat), or runs
// off the end of the graph.
func (fc *flowContext) run(ctx context.Context, current string) error {
	for current != "" {
		node := fc.find(current)
		if node == nil {
			return nil
		}

		switch node.Type {
		case "send_message":
			fc.sendBotMessage(fmt.Sprint(node.Config["message"]), "text", nil)
			current = node.Next

		case "ask_question":
			msgType := "text"
			var metadata *string
			if options, ok := node.Config["options"]; ok {
				msgType = "quick_reply"
				metaBytes, _ := json.Marshal(map[string]any{"options": options})
				s := string(metaBytes)
				metadata = &s
			}
			fc.sendBotMessage(fmt.Sprint(node.Config["message"]), msgType, metadata)
			return fc.persistState(node.ID)

		case "condition":
			field, _ := node.Config["field"].(string)
			operator, _ := node.Config["operator"].(string)
			value := node.Config["value"]
			actual := fmt.Sprint(fc.variables[field])
			cs := automation.ConditionSet{Logic: "and", Rules: []automation.Rule{{Field: field, Operator: operator, Value: value}}}
			if automation.Evaluate(cs, map[string]string{field: actual}) {
				current = node.Branches["true"]
			} else {
				current = node.Branches["false"]
			}

		case "set_variable":
			name, _ := node.Config["name"].(string)
			if name != "" {
				fc.variables[name] = node.Config["value"]
			}
			current = node.Next

		case "delay":
			// No-op — documented simplification, see package comment.
			current = node.Next

		case "call_integration":
			fc.callIntegration(node)
			current = node.Next

		case "handoff_to_agent":
			if err := fc.handoffToAgent(ctx); err != nil {
				return err
			}
			return nil

		case "close_chat":
			return fc.closeChat()

		default:
			current = node.Next
		}
	}
	return fc.persistState("")
}

func (fc *flowContext) persistState(nodeID string) error {
	variablesJSON, _ := json.Marshal(fc.variables)
	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}
	_, err := fc.conn.Exec(`UPDATE chat SET bot_node_id = ?, bot_variables = ? WHERE id = ?`, nodeArg, string(variablesJSON), fc.chatID)
	return err
}

func (fc *flowContext) sendBotMessage(body, msgType string, metadata *string) {
	now := time.Now()
	result, err := fc.conn.Exec(
		`INSERT INTO message (chat_id, sender_type, sender_id, body, type, metadata, created_at) VALUES (?, 'bot', ?, ?, ?, ?, ?)`,
		fc.chatID, fc.flowID, body, msgType, metadata, now,
	)
	if err != nil {
		return
	}
	id, _ := result.LastInsertId()
	fc.conn.Exec(`UPDATE chat SET last_message_at = ? WHERE id = ?`, now, fc.chatID)

	out := botMessage{ID: id, ChatUUID: fc.chatUUID, SenderType: "bot", Body: body, Type: msgType, Metadata: metadata, CreatedAt: now}
	fc.hub.Publish(ws.VisitorSubject(fc.visitorID), ws.Event{Type: "message", Data: out})
	fc.hub.Publish(ws.DashboardSubject(fc.merchantID), ws.Event{Type: "chat_updated"})
}

// callIntegration is the "AI"/third-party step (overview.md §6.4): a
// generic outbound REST call, never a hardcoded vendor SDK. Best-effort —
// a failed call posts a graceful fallback message instead of breaking
// the conversation.
// visitorSnapshot is the identity slice sent to an external system on
// every call_integration request — enough for a CRM/AI service to
// recognize the customer without exposing anything beyond what's already
// on the visitor record.
type visitorSnapshot struct {
	UUID        string  `json:"uuid"`
	DisplayName string  `json:"displayName"`
	Phone       *string `json:"phone,omitempty"`
	Email       *string `json:"email,omitempty"`
}

func (fc *flowContext) visitorSnapshot() visitorSnapshot {
	var v visitorSnapshot
	var phone, email sql.NullString
	fc.conn.QueryRow(`SELECT uuid, display_name, phone, email FROM visitor WHERE id = ?`, fc.visitorID).
		Scan(&v.UUID, &v.DisplayName, &phone, &email)
	if phone.Valid {
		v.Phone = &phone.String
	}
	if email.Valid {
		v.Email = &email.String
	}
	return v
}

// callIntegration is the "AI"/third-party step (overview.md §6.4): a
// generic outbound REST call, never a hardcoded vendor SDK. Best-effort —
// a failed call posts a graceful fallback message instead of breaking
// the conversation, unless sendAsMessage is explicitly turned off.
//
// A response can be captured into a bot variable (optionally extracted
// via a dot/index path) — that's deliberately the *only* new mechanic
// here. A flow branches on the outcome (including on the reserved
// "<var>_ok" success flag) with the existing `condition` step rather than
// this step inventing its own branching, matching the Live Helper Chat
// research this was scoped down from (see overview.md §12 for what was
// explicitly left out).
func (fc *flowContext) callIntegration(node *Node) {
	integrationID, ok := node.Config["integrationId"]
	if !ok {
		return
	}
	sendAsMessage := true
	if v, exists := node.Config["sendAsMessage"]; exists {
		if b, isBool := v.(bool); isBool {
			sendAsMessage = b
		}
	}
	saveResponseAs, _ := node.Config["saveResponseAs"].(string)
	responsePath, _ := node.Config["responsePath"].(string)

	fail := func(msg string) {
		if saveResponseAs != "" {
			fc.variables[saveResponseAs+"_ok"] = "false"
		}
		if sendAsMessage {
			fc.sendBotMessage(msg, "text", nil)
		}
	}

	var configRaw, secret string
	if err := fc.conn.QueryRow(`SELECT config, secret_hash FROM integration WHERE id = ?`, integrationID).Scan(&configRaw, &secret); err != nil {
		fail("Sorry, that connection isn't set up correctly.")
		return
	}
	var cfg struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	json.Unmarshal([]byte(configRaw), &cfg)

	visitor := fc.visitorSnapshot()
	payload, _ := json.Marshal(map[string]any{
		"chatUuid":    fc.chatUUID,
		"visitorUuid": visitor.UUID,
		"visitor":     visitor,
		"variables":   fc.variables,
	})
	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		fail("Sorry, something went wrong reaching that system.")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fail("Sorry, something went wrong reaching that system.")
		return
	}
	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	var extracted any
	if responsePath != "" {
		extracted = resolvePath(body, responsePath)
	} else if msg, exists := body["message"]; exists {
		extracted = msg
	}

	if saveResponseAs != "" {
		fc.variables[saveResponseAs] = extracted
		fc.variables[saveResponseAs+"_ok"] = strconv.FormatBool(resp.StatusCode >= 200 && resp.StatusCode < 300)
	}

	if sendAsMessage {
		text := stringifyForMessage(extracted)
		if text == "" {
			text = "(no response)"
		}
		fc.sendBotMessage(text, "text", nil)
	}
}

// resolvePath walks a decoded JSON value (map[string]any / []any) along a
// "."-separated path (e.g. "data.answer" or "items.0.name") — the small
// subset of Live Helper Chat's path-extraction idea that's actually
// needed here (see overview.md §12 for what wasn't adopted).
func resolvePath(data any, path string) any {
	current := data
	for _, segment := range strings.Split(path, ".") {
		switch v := current.(type) {
		case map[string]any:
			current = v[segment]
		case []any:
			idx, err := strconv.Atoi(segment)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			current = v[idx]
		default:
			return nil
		}
	}
	return current
}

// stringifyForMessage renders an extracted response value as visitor-
// facing text: a plain string stays as-is, anything else (number, bool,
// object, array) is JSON-encoded so it's at least visible rather than
// silently dropped.
func stringifyForMessage(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func (fc *flowContext) handoffToAgent(ctx context.Context) error {
	agentID, status, err := routing.Route(ctx, fc.conn, fc.redisClient, fc.merchantID)
	if err != nil {
		return err
	}
	if _, err := fc.conn.Exec(
		`UPDATE chat SET status = ?, agent_id = ?, bot_flow_id = NULL, bot_node_id = NULL WHERE id = ?`,
		status, agentID, fc.chatID,
	); err != nil {
		return err
	}
	if agentID != nil {
		fc.hub.Publish(ws.AgentSubject(*agentID), ws.Event{Type: "chat_updated"})
	}
	fc.hub.Publish(ws.DashboardSubject(fc.merchantID), ws.Event{Type: "chat_updated"})
	return nil
}

func (fc *flowContext) closeChat() error {
	if _, err := fc.conn.Exec(`UPDATE chat SET status = 'closed', closed_at = NOW() WHERE id = ?`, fc.chatID); err != nil {
		return err
	}
	fc.hub.Publish(ws.VisitorSubject(fc.visitorID), ws.Event{Type: "chat_closed", Data: map[string]string{"chatUuid": fc.chatUUID}})
	fc.hub.Publish(ws.DashboardSubject(fc.merchantID), ws.Event{Type: "chat_updated"})
	return nil
}
