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
// Unauthenticated by design, same as the rest of this file.
func GetPublicMerchantHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		var name, status string
		var widgetConfig sql.NullString
		if err := conn.QueryRow(
			`SELECT name, status, widget_config FROM merchant WHERE code = ?`, c.Param("code"),
		).Scan(&name, &status, &widgetConfig); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if status != "active" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"name": name, "widgetConfig": widgetConfig.String})
	}
}

type startChatRequest struct {
	MerchantCode     string `json:"merchantCode" binding:"required"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	DisplayName      string `json:"displayName"`
	PassthroughToken string `json:"passthroughToken"`
	PageURL          string `json:"pageUrl"`
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
		if req.PassthroughToken != "" {
			// Logged-in visitor (§10.2) — never trust the payload until
			// its HMAC verifies against this merchant's own secret.
			identity, err := passthrough.Verify(conn, merchantID, req.PassthroughToken)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_passthrough_token"})
				return
			}
			phone, email, displayName = identity.Phone, identity.Email, identity.Name
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

		v, err := visitor.Resolve(conn, merchantID, phone, email, displayName, c.ClientIP())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
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

		botStarted, err := botengine.TryStart(context.Background(), conn, hub, redisClient, chatID, merchantID, evalCtx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		status := "pending"
		if !botStarted {
			agentID, routedStatus, err := routing.Route(context.Background(), conn, redisClient, merchantID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
				return
			}
			if _, err := conn.Exec(`UPDATE chat SET agent_id = ?, status = ? WHERE id = ?`, agentID, routedStatus, chatID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
				return
			}
			status = routedStatus
		} else {
			conn.QueryRow(`SELECT status FROM chat WHERE id = ?`, chatID).Scan(&status)
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
		`SELECT DISTINCT r.condition, r.message FROM automation_rule r
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
		if err := rows.Scan(&conditionRaw, &message); err != nil {
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

		msgID, createdAt, err := insertMessage(conn, chatID, "system", nil, message, "text", nil)
		if err != nil {
			return
		}
		out := messageOut{ID: msgID, ChatUUID: chatUUID, SenderType: "system", Body: message, Type: "text", CreatedAt: createdAt}
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
