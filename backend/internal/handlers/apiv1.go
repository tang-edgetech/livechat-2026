package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/botengine"
	"livechat/backend/internal/ratelimit"
	"livechat/backend/internal/visitor"
	"livechat/backend/internal/webhook"
	"livechat/backend/internal/ws"
)

// This file is the inbound half of REST API v1 (overview.md §6.5/§9): a
// server-to-server counterpart to the widget's visitor endpoints,
// authenticated via RequireAPIKey (Bearer key) instead of a
// visitor/session cookie, and scoped to exactly the one merchant the
// presented key belongs to. A message sent in here is treated exactly
// like a real visitor message arriving through a different channel (e.g.
// a WhatsApp/SMS gateway relaying it) — same sender_type, same bot
// continuation — since from the platform's perspective that's what it is.

type createChatV1Request struct {
	Phone       string `json:"phone" binding:"required"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	// Tier is trusted here because the whole request is already
	// Bearer-API-key-authenticated to exactly one merchant (overview.md
	// §6.9.1). Only the literal "vip" is meaningful — omit the field
	// entirely for a normal customer.
	Tier string `json:"tier"`
}

func CreateChatV1Handler(state *appstate.State, hub *ws.Hub, redisClient *redis.Client, limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		merchantID := c.MustGet("api_merchant_id").(int64)

		var req createChatV1Request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		if !limiter.Allow("api-chat-start:" + req.Phone) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
			return
		}

		tier := ""
		if req.Tier == "vip" {
			tier = "vip"
		}
		v, err := visitor.Resolve(conn, merchantID, req.Phone, req.Email, req.DisplayName, c.ClientIP(), tier)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		// Same resumption fix as StartChatHandler — a caller that
		// repeatedly resolves the same identity should reconnect to
		// their existing open chat, not spawn a new one each time.
		if existing, err := findOpenChat(conn, v.ID); err == nil {
			c.JSON(http.StatusOK, gin.H{"chatUuid": existing.uuid, "visitorUuid": v.UUID, "status": existing.status})
			return
		}

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

		evalCtx := map[string]string{"page_url": "", "time_of_day": time.Now().Format("15:04")}
		applyAutomationGreeting(conn, hub, chatID, merchantID, evalCtx)

		status, err := routeNewChat(context.Background(), conn, hub, redisClient, chatID, merchantID, v.Tier, evalCtx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		notifyChatUpdated(conn, hub, merchantID, chatUUID)
		webhook.Dispatch(conn, merchantID, "chat.created", gin.H{"chatUuid": chatUUID, "visitorUuid": v.UUID, "status": status})

		c.JSON(http.StatusOK, gin.H{"chatUuid": chatUUID, "visitorUuid": v.UUID, "status": status})
	}
}

// apiChatAccess scopes a chat lookup to the calling API key's own
// merchant — an integration for merchant A must never be able to read or
// post into merchant B's chat by guessing a UUID.
func apiChatAccess(conn *sql.DB, merchantID int64, chatUUID string) (*chatRef, error) {
	var ref chatRef
	err := conn.QueryRow(
		`SELECT id, merchant_id, visitor_id, agent_id, status FROM chat WHERE uuid = ? AND merchant_id = ?`,
		chatUUID, merchantID,
	).Scan(&ref.ID, &ref.MerchantID, &ref.VisitorID, &ref.AgentID, &ref.Status)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func GetChatV1Handler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		merchantID := c.MustGet("api_merchant_id").(int64)

		ref, err := apiChatAccess(conn, merchantID, c.Param("uuid"))
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

func SendMessageV1Handler(state *appstate.State, hub *ws.Hub, redisClient *redis.Client, limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		merchantID := c.MustGet("api_merchant_id").(int64)

		var req sendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		ref, err := apiChatAccess(conn, merchantID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status == "closed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chat_closed"})
			return
		}
		if !limiter.Allow("api-message-send:" + c.Param("uuid")) {
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
		webhook.Dispatch(conn, merchantID, "message.received", out)

		if ref.Status == "bot" {
			botengine.ContinueOnVisitorMessage(context.Background(), conn, hub, redisClient, ref.ID, req.Body)
		}

		c.JSON(http.StatusOK, out)
	}
}
