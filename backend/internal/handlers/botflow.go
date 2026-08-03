package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

type botFlowOut struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Trigger       string  `json:"trigger"`
	Flow          string  `json:"flow"`
	IntegrationID *int64  `json:"integration_id"`
	IsGlobal      bool    `json:"is_global"`
	IsActive      bool    `json:"is_active"`
	MerchantUUID  *string `json:"merchant_uuid"`
}

func ListBotFlowsHandler(state *appstate.State) gin.HandlerFunc {
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
		query := `SELECT DISTINCT bf.id, bf.name, bf.trigger_config, bf.flow, bf.integration_id, bf.is_global, bf.is_active, m.uuid
		          FROM bot_flow bf
		          LEFT JOIN bot_flow_merchant bfm ON bfm.bot_flow_id = bf.id
		          LEFT JOIN merchant m ON m.id = bfm.merchant_id
		          WHERE bf.is_global = TRUE`
		if len(merchantIDs) > 0 {
			query += ` OR bfm.merchant_id IN (` + placeholders + `)`
		}
		query += ` ORDER BY bf.created_at DESC`

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		defer rows.Close()

		out := []botFlowOut{}
		for rows.Next() {
			var f botFlowOut
			if err := rows.Scan(&f.ID, &f.Name, &f.Trigger, &f.Flow, &f.IntegrationID, &f.IsGlobal, &f.IsActive, &f.MerchantUUID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, f)
		}
		c.JSON(http.StatusOK, gin.H{"botFlows": out})
	}
}

type saveBotFlowRequest struct {
	Name          string  `json:"name" binding:"required"`
	Trigger       string  `json:"trigger" binding:"required"`
	Flow          string  `json:"flow" binding:"required"`
	IntegrationID *int64  `json:"integrationId"`
	IsGlobal      bool    `json:"isGlobal"`
	IsActive      bool    `json:"isActive"`
	MerchantUUID  *string `json:"merchantUuid"`
}

func CreateBotFlowHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var req saveBotFlowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
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
			`INSERT INTO bot_flow (name, trigger_config, flow, integration_id, is_global, is_active, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			req.Name, req.Trigger, req.Flow, req.IntegrationID, req.IsGlobal, req.IsActive, userID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		id, _ := result.LastInsertId()
		if merchantID != nil {
			conn.Exec(`INSERT INTO bot_flow_merchant (bot_flow_id, merchant_id) VALUES (?, ?)`, id, *merchantID)
		}

		audit.Log(conn, audit.Entry{MerchantID: merchantID, UserID: &userID, Category: "bot_flow", Message: "bot flow created: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"id": id})
	}
}

func UpdateBotFlowHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		var req saveBotFlowRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		if _, err := conn.Exec(
			`UPDATE bot_flow SET name = ?, trigger_config = ?, flow = ?, integration_id = ?, is_active = ? WHERE id = ?`,
			req.Name, req.Trigger, req.Flow, req.IntegrationID, req.IsActive, id,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "bot_flow", Message: "bot flow updated: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func DeleteBotFlowHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		conn.Exec(`DELETE FROM bot_flow_merchant WHERE bot_flow_id = ?`, id)
		if _, err := conn.Exec(`DELETE FROM bot_flow WHERE id = ?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "bot_flow", Message: "bot flow deleted", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
