package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/automation"
	"livechat/backend/internal/botengine"
	"livechat/backend/internal/passthrough"
	"livechat/backend/internal/ratelimit"
	"livechat/backend/internal/routing"
	"livechat/backend/internal/storage"
	"livechat/backend/internal/visitor"
	"livechat/backend/internal/webhook"
	"livechat/backend/internal/ws"
)

// This file is the visitor side of the chat — unauthenticated by design,
// the same way a real anonymous website visitor would be. It's also the
// widget's actual backend now (Phase 3): the pre-chat form and the
// logged-in passthrough flow (§10.2/§10.3) both funnel through
// StartChatHandler, which used to double as the Phase 2 test harness.

// GetPublicMerchantHandler exposes only what the widget needs to render
// itself before a chat exists — branding, never anything internal.
// Unauthenticated by design, same as the rest of this file. canLiveChat
// is a best-effort hint for which copy/form to show (§ enquiry-form
// fallback) — the real, authoritative decision happens again at submit
// time in StartChatHandler, so a stale hint here can't misroute anything.
func GetPublicMerchantHandler(state *appstate.State, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		var merchantID int64
		var name, status string
		var widgetConfig sql.NullString
		if err := conn.QueryRow(
			`SELECT id, name, status, widget_config FROM merchant WHERE code = ?`, c.Param("code"),
		).Scan(&merchantID, &name, &status, &widgetConfig); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if status != "active" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		canLiveChat, _ := canStartLiveChat(context.Background(), conn, redisClient, merchantID, false)
		c.JSON(http.StatusOK, gin.H{"name": name, "widgetConfig": widgetConfig.String, "canLiveChat": canLiveChat})
	}
}

// canStartLiveChat decides whether a new chat for merchantID can go
// through the normal bot-or-route path, or whether nobody/nothing could
// plausibly answer it right now (item 4's enquiry-form fallback). A VIP
// visitor skips the bot check entirely — VIP routing never defers to a
// bot flow (routeNewChat below) — so only agent availability counts.
func canStartLiveChat(ctx context.Context, conn *sql.DB, redisClient *redis.Client, merchantID int64, isVIP bool) (bool, error) {
	available, err := routing.AnyAvailableAgent(ctx, conn, redisClient, merchantID)
	if err != nil || available || isVIP {
		return available, err
	}
	return botengine.HasActiveFlow(conn, merchantID)
}

type startChatRequest struct {
	MerchantCode     string `json:"merchantCode" binding:"required"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	DisplayName      string `json:"displayName"`
	PassthroughToken string `json:"passthroughToken"`
	PageURL          string `json:"pageUrl"`
	// Message is only meaningful when this lands as an 'enquiry' (nobody
	// reachable right now) — it becomes the chat's opening message so an
	// operator sees what the visitor actually wanted before inviting them
	// in (overview.md item 4). Ignored otherwise; the normal live-chat
	// pre-chat form has no such field.
	Message string `json:"message"`
}

type openChat struct {
	uuid   string
	status string
}

// findOpenChat returns a visitor's most recent still-open chat, if any —
// shared by StartChatHandler and CreateChatV1Handler so a returning
// identity (resolved by visitor.Resolve via phone/email) resumes their
// conversation instead of getting a fresh blank one every time.
func findOpenChat(conn *sql.DB, visitorID int64) (*openChat, error) {
	var oc openChat
	err := conn.QueryRow(
		`SELECT uuid, status FROM chat WHERE visitor_id = ? AND status IN ('pending','active','bot','enquiry') ORDER BY started_at DESC LIMIT 1`,
		visitorID,
	).Scan(&oc.uuid, &oc.status)
	if err != nil {
		return nil, err
	}
	return &oc, nil
}

// routeNewChat decides and applies the initial agent_id/status for a
// freshly-inserted chat — shared by StartChatHandler and
// CreateChatV1Handler. A VIP visitor (overview.md §6.9.1) skips the bot
// entirely and goes straight to routing.RouteVIP (which itself falls
// back to the normal agent pool if no VIP-designated agent is online);
// everyone else keeps the existing bot-first-then-round-robin path.
func routeNewChat(ctx context.Context, conn *sql.DB, hub *ws.Hub, redisClient *redis.Client, chatID, merchantID int64, tier string, evalCtx map[string]string) (string, error) {
	if tier == "vip" {
		agentID, status, err := routing.RouteVIP(ctx, conn, redisClient, merchantID)
		if err != nil {
			return "", err
		}
		if _, err := conn.Exec(`UPDATE chat SET agent_id = ?, status = ? WHERE id = ?`, agentID, status, chatID); err != nil {
			return "", err
		}
		return status, nil
	}

	botStarted, err := botengine.TryStart(ctx, conn, hub, redisClient, chatID, merchantID, evalCtx)
	if err != nil {
		return "", err
	}
	if !botStarted {
		agentID, status, err := routing.Route(ctx, conn, redisClient, merchantID)
		if err != nil {
			return "", err
		}
		if _, err := conn.Exec(`UPDATE chat SET agent_id = ?, status = ? WHERE id = ?`, agentID, status, chatID); err != nil {
			return "", err
		}
		return status, nil
	}

	var status string
	conn.QueryRow(`SELECT status FROM chat WHERE id = ?`, chatID).Scan(&status)
	return status, nil
}

func StartChatHandler(state *appstate.State, hub *ws.Hub, redisClient *redis.Client, limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()

		var req startChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var merchantID int64
		var merchantStatus string
		if err := conn.QueryRow(`SELECT id, status FROM merchant WHERE code = ?`, req.MerchantCode).Scan(&merchantID, &merchantStatus); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "merchant_not_found"})
			return
		}
		if merchantStatus != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "merchant_suspended"})
			return
		}

		phone, email, displayName := req.Phone, req.Email, req.DisplayName
		tier := ""
		if req.PassthroughToken != "" {
			// Logged-in visitor (§10.2) — never trust the payload until
			// its HMAC verifies against this merchant's own secret.
			identity, err := passthrough.Verify(conn, merchantID, req.PassthroughToken)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_passthrough_token"})
				return
			}
			phone, email, displayName = identity.Phone, identity.Email, identity.Name
			// §6.9.1: only an explicit "vip" claim inside this signed
			// payload is a trusted signal — anything else (including a
			// garbage value) is treated as no signal at all.
			if identity.Tier == "vip" {
				tier = "vip"
			}
		}
		if phone == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "phone_required"})
			return
		}

		if !limiter.Allow("chat-start:" + c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			return
		}
		if !limiter.Allow("chat-start:" + phone) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			return
		}

		v, err := visitor.Resolve(conn, merchantID, phone, email, displayName, c.ClientIP(), tier)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		// Resume an existing open chat rather than always starting a new
		// one (overview.md §10.2) — a logged-in customer's own site
		// reissues a fresh passthrough token on every page load, so
		// without this, visiting from a new device (or after clearing
		// localStorage) would silently orphan their prior conversation
		// instead of reconnecting to it. A closed chat still starts
		// fresh — that conversation is genuinely done.
		if existing, err := findOpenChat(conn, v.ID); err == nil {
			c.JSON(http.StatusOK, gin.H{"chatUuid": existing.uuid, "visitorUuid": v.UUID, "status": existing.status})
			return
		}

		// Created as pending/unassigned first so Automation and the Bot
		// engine have a real chat_id to attach messages to; routing only
		// runs afterward, and only if no bot flow claimed the chat.
		chatUUID := uuid.New().String()
		result, err := conn.Exec(
			`INSERT INTO chat (uuid, merchant_id, visitor_id, status) VALUES (?, ?, ?, 'pending')`,
			chatUUID, merchantID, v.ID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		chatID, _ := result.LastInsertId()

		evalCtx := map[string]string{
			"page_url":    req.PageURL,
			"time_of_day": time.Now().Format("15:04"),
		}
		applyAutomationGreeting(conn, hub, chatID, merchantID, evalCtx)

		var status string
		if canLive, err := canStartLiveChat(context.Background(), conn, redisClient, merchantID, v.Tier == "vip"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		} else if !canLive {
			// Nobody/nothing could plausibly answer this right now — park
			// it as an enquiry instead of a live chat with no one coming
			// (overview.md item 4). The visitor's own message becomes the
			// chat's opening line so an operator sees it before inviting
			// them in.
			if _, err := conn.Exec(`UPDATE chat SET status = 'enquiry' WHERE id = ?`, chatID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
				return
			}
			if msg := req.Message; msg != "" {
				// Staff only ever see this message via the REST fetch once
				// they open the chat (there's no assigned agent to push a
				// live WS "message" event to yet) — notifyChatUpdated
				// below is what actually alerts the Chats list.
				insertMessage(conn, chatID, "visitor", &v.ID, msg, "text", nil)
			}
			status = "enquiry"
		} else {
			status, err = routeNewChat(context.Background(), conn, hub, redisClient, chatID, merchantID, v.Tier, evalCtx)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
				return
			}
		}

		notifyChatUpdated(conn, hub, merchantID, chatUUID)
		webhook.Dispatch(conn, merchantID, "chat.created", gin.H{"chatUuid": chatUUID, "visitorUuid": v.UUID, "status": status})

		c.JSON(http.StatusOK, gin.H{
			"chatUuid":    chatUUID,
			"visitorUuid": v.UUID,
			"status":      status,
		})
	}
}

// applyAutomationGreeting inserts the first matching global/merchant
// automation_rule's message as a system message (overview.md §6.3) —
// independent of whether a bot flow also fires for this chat.
func applyAutomationGreeting(conn *sql.DB, hub *ws.Hub, chatID, merchantID int64, evalCtx map[string]string) {
	rows, err := conn.Query(
		`SELECT DISTINCT r.condition, r.message, r.is_html FROM automation_rule r
		 LEFT JOIN automation_rule_merchant arm ON arm.automation_rule_id = r.id
		 WHERE r.is_active = TRUE AND r.trigger_type = 'chat_start'
		 AND (r.is_global = TRUE OR arm.merchant_id = ?)
		 ORDER BY r.created_at ASC`,
		merchantID,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var conditionRaw sql.NullString
		var message string
		var isHtml bool
		if err := rows.Scan(&conditionRaw, &message, &isHtml); err != nil {
			return
		}
		var cs automation.ConditionSet
		if conditionRaw.Valid {
			json.Unmarshal([]byte(conditionRaw.String), &cs)
		}
		if !automation.Evaluate(cs, evalCtx) {
			continue
		}

		var chatUUID string
		var visitorID int64
		conn.QueryRow(`SELECT uuid, visitor_id FROM chat WHERE id = ?`, chatID).Scan(&chatUUID, &visitorID)

		var metadata *string
		if isHtml {
			raw, _ := json.Marshal(map[string]bool{"isHtml": true})
			s := string(raw)
			metadata = &s
		}

		msgID, createdAt, err := insertMessage(conn, chatID, "system", nil, message, "text", metadata)
		if err != nil {
			return
		}
		out := messageOut{ID: msgID, ChatUUID: chatUUID, SenderType: "system", Body: message, Type: "text", CreatedAt: createdAt, Metadata: metadata}
		hub.Publish(ws.VisitorSubject(visitorID), ws.Event{Type: "message", Data: out})
		return // one greeting per chat is enough
	}
}

// visitorChatAccess validates the (visitor, chat) pair every visitor
// endpoint receives as query params — the visitor equivalent of
// chatAccess's merchant-scope check for staff.
func visitorChatAccess(conn *sql.DB, visitorUUID, chatUUID string) (*chatRef, error) {
	var ref chatRef
	err := conn.QueryRow(
		`SELECT c.id, c.merchant_id, c.visitor_id, c.agent_id, c.status
		 FROM chat c JOIN visitor v ON v.id = c.visitor_id
		 WHERE c.uuid = ? AND v.uuid = ?`,
		chatUUID, visitorUUID,
	).Scan(&ref.ID, &ref.MerchantID, &ref.VisitorID, &ref.AgentID, &ref.Status)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func GetVisitorChatHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		ref, err := visitorChatAccess(conn, c.Query("visitor"), c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		rows, err := conn.Query(
			`SELECT id, sender_type, body, type, metadata, created_at FROM message WHERE chat_id = ? ORDER BY created_at ASC`,
			ref.ID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		messages := []messageOut{}
		for rows.Next() {
			var m messageOut
			if err := rows.Scan(&m.ID, &m.SenderType, &m.Body, &m.Type, &m.Metadata, &m.CreatedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			messages = append(messages, m)
		}

		c.JSON(http.StatusOK, gin.H{"status": ref.Status, "messages": messages})
	}
}

func SendVisitorMessageHandler(state *appstate.State, hub *ws.Hub, redisClient *redis.Client, limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()

		var req sendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		ref, err := visitorChatAccess(conn, c.Query("visitor"), c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status == "closed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chat_closed"})
			return
		}
		if !limiter.Allow("message-send:" + c.ClientIP()) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			return
		}

		msgID, createdAt, err := insertMessage(conn, ref.ID, "visitor", &ref.VisitorID, req.Body, "text", nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		out := messageOut{ID: msgID, ChatUUID: c.Param("uuid"), SenderType: "visitor", Body: req.Body, Type: "text", CreatedAt: createdAt}
		if ref.AgentID.Valid {
			hub.Publish(ws.AgentSubject(ref.AgentID.Int64), ws.Event{Type: "message", Data: out})
		}
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		webhook.Dispatch(conn, ref.MerchantID, "message.received", out)

		if ref.Status == "bot" {
			// Best-effort: a bot-engine hiccup shouldn't fail the
			// visitor's own message from being saved/delivered above.
			botengine.ContinueOnVisitorMessage(context.Background(), conn, hub, redisClient, ref.ID, req.Body)
		}

		c.JSON(http.StatusOK, out)
	}
}

func UploadVisitorFileHandler(state *appstate.State, hub *ws.Hub, driver storage.Driver) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		ref, err := visitorChatAccess(conn, c.Query("visitor"), c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status == "closed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chat_closed"})
			return
		}

		fh, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no_file"})
			return
		}

		out, err := storeChatFile(conn, driver, ref, "visitor", &ref.VisitorID, fh)
		if err != nil {
			var ruleErr *fileRuleError
			if errors.As(err, &ruleErr) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "file_rejected", "detail": ruleErr.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload_failed", "detail": err.Error()})
			return
		}
		out.ChatUUID = c.Param("uuid")

		if ref.AgentID.Valid {
			hub.Publish(ws.AgentSubject(ref.AgentID.Int64), ws.Event{Type: "message", Data: out})
		}
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		c.JSON(http.StatusOK, out)
	}
}
