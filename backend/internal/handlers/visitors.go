package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

type visitorOut struct {
	UUID         string  `json:"uuid"`
	DisplayName  string  `json:"display_name"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
	Tier         string  `json:"tier"`
	MerchantName string  `json:"merchant_name"`
}

// ListVisitorsHandler backs the manual-merge tool's search (overview.md
// §10.3) — same merchant-scoping rule as every other staff-facing list.
func ListVisitorsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if len(merchantIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"visitors": []visitorOut{}})
			return
		}
		placeholders, args := int64SliceToPlaceholders(merchantIDs)

		query := `SELECT v.uuid, v.display_name, v.phone, v.email, v.tier, m.name
		          FROM visitor v JOIN merchant m ON m.id = v.merchant_id
		          WHERE v.merchant_id IN (` + placeholders + `) AND v.merged_into_id IS NULL`
		if search := c.Query("search"); search != "" {
			query += ` AND (v.display_name LIKE ? OR v.phone LIKE ? OR v.email LIKE ?)`
			like := "%" + search + "%"
			args = append(args, like, like, like)
		}
		query += ` ORDER BY v.created_at DESC LIMIT 50`

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		out := []visitorOut{}
		for rows.Next() {
			var v visitorOut
			if err := rows.Scan(&v.UUID, &v.DisplayName, &v.Phone, &v.Email, &v.Tier, &v.MerchantName); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, v)
		}
		c.JSON(http.StatusOK, gin.H{"visitors": out})
	}
}

type updateVisitorRequest struct {
	Email *string `json:"email"`
	// Tier is the manual-staff-tagging channel from overview.md §6.9.1 —
	// the always-available fallback alongside the trusted
	// passthrough/API-key signal handled in visitor.Resolve.
	Tier *string `json:"tier"`
}

// UpdateVisitorHandler: Admin/Super Admin force-correcting a Visitor's
// email (e.g. a typo) and/or setting their Normal/VIP tier — overview.md
// §10.3/§6.9.1, always audit-logged. Both fields are optional so this one
// endpoint serves either edit independently.
func UpdateVisitorHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)
		visitorUUID := c.Param("uuid")

		var req updateVisitorRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if req.Tier != nil && *req.Tier != "normal" && *req.Tier != "vip" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tier"})
			return
		}

		merchantID, err := visitorMerchantInScope(conn, role, userID, visitorUUID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		var messages []string
		if req.Email != nil {
			if _, err := conn.Exec(`UPDATE visitor SET email = ? WHERE uuid = ?`, *req.Email, visitorUUID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			messages = append(messages, "email corrected")
		}
		if req.Tier != nil {
			if _, err := conn.Exec(`UPDATE visitor SET tier = ? WHERE uuid = ?`, *req.Tier, visitorUUID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			messages = append(messages, "tier set to "+*req.Tier)
		}
		for _, msg := range messages {
			audit.Log(conn, audit.Entry{
				MerchantID: &merchantID, UserID: &userID, Category: "visitor",
				Message: "visitor " + msg, StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
			})
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func visitorMerchantInScope(conn *sql.DB, role string, userID int64, visitorUUID string) (int64, error) {
	var merchantID int64
	if err := conn.QueryRow(`SELECT merchant_id FROM visitor WHERE uuid = ?`, visitorUUID).Scan(&merchantID); err != nil {
		return 0, err
	}
	if role == "super_admin" {
		return merchantID, nil
	}
	var has int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, merchantID).Scan(&has); err != nil {
		return 0, err
	}
	if has == 0 {
		return 0, sql.ErrNoRows
	}
	return merchantID, nil
}

type mergeVisitorsRequest struct {
	SourceUUID string `json:"sourceUuid" binding:"required"`
	TargetUUID string `json:"targetUuid" binding:"required"`
}

// MergeVisitorsHandler implements the manual merge tool (overview.md
// §10.3): reassigns the source's chats and uploaded files to the target,
// marks the source merged (kept for history, excluded from future
// identity-resolution lookups), and audit-logs it.
func MergeVisitorsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var req mergeVisitorsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if req.SourceUUID == req.TargetUUID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source_and_target_must_differ"})
			return
		}

		sourceMerchant, err := visitorMerchantInScope(conn, role, userID, req.SourceUUID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "source_not_found"})
			return
		}
		targetMerchant, err := visitorMerchantInScope(conn, role, userID, req.TargetUUID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "target_not_found"})
			return
		}
		if sourceMerchant != targetMerchant {
			c.JSON(http.StatusBadRequest, gin.H{"error": "visitors_must_share_a_merchant"})
			return
		}

		var sourceID, targetID int64
		conn.QueryRow(`SELECT id FROM visitor WHERE uuid = ?`, req.SourceUUID).Scan(&sourceID)
		conn.QueryRow(`SELECT id FROM visitor WHERE uuid = ?`, req.TargetUUID).Scan(&targetID)

		tx, err := conn.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer tx.Rollback()

		if _, err := tx.Exec(`UPDATE chat SET visitor_id = ? WHERE visitor_id = ?`, targetID, sourceID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		if _, err := tx.Exec(`UPDATE file SET uploader_id = ? WHERE uploader_type = 'visitor' AND uploader_id = ?`, targetID, sourceID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		if _, err := tx.Exec(`UPDATE visitor SET merged_into_id = ? WHERE id = ?`, targetID, sourceID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &sourceMerchant, UserID: &userID, Category: "visitor_merge",
			Message:    "visitor " + req.SourceUUID + " merged into " + req.TargetUUID,
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
