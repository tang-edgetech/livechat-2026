package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

type cannedMessageOut struct {
	ID           int64   `json:"id"`
	Title        string  `json:"title"`
	Body         string  `json:"body"`
	IsGlobal     bool    `json:"is_global"`
	MerchantUUID *string `json:"merchant_uuid"`
}

// ListCannedMessagesHandler: reachable by any staff role — Agents use
// these to quick-insert a reply (overview.md §6.3), only Admin/Super
// Admin can create/delete them.
func ListCannedMessagesHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		placeholders, args := int64SliceToPlaceholders(merchantIDs)
		query := `SELECT DISTINCT cm.id, cm.title, cm.body, cm.is_global, m.uuid
		          FROM canned_message cm
		          LEFT JOIN canned_message_merchant cmm ON cmm.canned_message_id = cm.id
		          LEFT JOIN merchant m ON m.id = cmm.merchant_id
		          WHERE cm.is_global = TRUE`
		if len(merchantIDs) > 0 {
			query += ` OR cmm.merchant_id IN (` + placeholders + `)`
		}
		query += ` ORDER BY cm.created_at DESC`

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		out := []cannedMessageOut{}
		for rows.Next() {
			var m cannedMessageOut
			if err := rows.Scan(&m.ID, &m.Title, &m.Body, &m.IsGlobal, &m.MerchantUUID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, m)
		}
		c.JSON(http.StatusOK, gin.H{"cannedMessages": out})
	}
}

type saveCannedMessageRequest struct {
	Title        string  `json:"title" binding:"required"`
	Body         string  `json:"body" binding:"required"`
	IsGlobal     bool    `json:"isGlobal"`
	MerchantUUID *string `json:"merchantUuid"`
}

func CreateCannedMessageHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var req saveCannedMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if req.IsGlobal && role != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only_super_admin_can_create_global"})
			return
		}

		var merchantID *int64
		if req.MerchantUUID != nil && *req.MerchantUUID != "" {
			id, err := resolveMerchantInScope(conn, role, userID, *req.MerchantUUID)
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "merchant_not_in_scope"})
				return
			}
			merchantID = &id
		} else if !req.IsGlobal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_required_unless_global"})
			return
		}

		result, err := conn.Exec(
			`INSERT INTO canned_message (title, body, is_global, created_by) VALUES (?, ?, ?, ?)`,
			req.Title, req.Body, req.IsGlobal, userID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		id, _ := result.LastInsertId()
		if merchantID != nil {
			conn.Exec(`INSERT INTO canned_message_merchant (canned_message_id, merchant_id) VALUES (?, ?)`, id, *merchantID)
		}

		audit.Log(conn, audit.Entry{MerchantID: merchantID, UserID: &userID, Category: "canned_message", Message: "canned message created: " + req.Title, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"id": id})
	}
}

func DeleteCannedMessageHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		conn.Exec(`DELETE FROM canned_message_merchant WHERE canned_message_id = ?`, id)
		if _, err := conn.Exec(`DELETE FROM canned_message WHERE id = ?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "canned_message", Message: "canned message deleted", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
