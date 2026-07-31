package handlers

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/passthrough"
	"livechat/backend/internal/ratelimit"
	"livechat/backend/internal/routing"
	"livechat/backend/internal/storage"
	"livechat/backend/internal/visitor"
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

		agentID, status, err := routing.Route(context.Background(), conn, redisClient, merchantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		chatUUID := uuid.New().String()
		_, err = conn.Exec(
			`INSERT INTO chat (uuid, merchant_id, visitor_id, agent_id, status) VALUES (?, ?, ?, ?, ?)`,
			chatUUID, merchantID, v.ID, agentID, status,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		notifyChatUpdated(conn, hub, merchantID, chatUUID)

		c.JSON(http.StatusOK, gin.H{
			"chatUuid":    chatUUID,
			"visitorUuid": v.UUID,
			"status":      status,
		})
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

func SendVisitorMessageHandler(state *appstate.State, hub *ws.Hub, limiter *ratelimit.Limiter) gin.HandlerFunc {
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
