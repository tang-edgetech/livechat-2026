package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/botengine"
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

type botFlowAnalyticsOut struct {
	BotFlowID       int64                   `json:"bot_flow_id"`
	Name            string                  `json:"name"`
	Mode            string                  `json:"mode"`
	TotalRuns       int                     `json:"total_runs"`
	ActiveRuns      int                     `json:"active_runs"`
	ResolvedRuns    int                     `json:"resolved_runs"`
	CompletedRuns   int                     `json:"completed_runs"`
	HandoffRuns     int                     `json:"handoff_runs"`
	ClosedRuns      int                     `json:"closed_runs"`
	AbandonedRuns   int                     `json:"abandoned_runs"`
	CompletionRate  *float64                `json:"completion_rate"`
	HandoffRate     *float64                `json:"handoff_rate"`
	AbandonmentRate *float64                `json:"abandonment_rate"`
	DropOffNodes    []botFlowDropOffNodeOut `json:"drop_off_nodes,omitempty"`
}

type botFlowDropOffNodeOut struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
}

// BotFlowAnalyticsHandler is Bot Analytics' read side (bot_flow_run,
// written by botengine/flowrun.go's helpers) — completion/handoff/
// abandonment rate per flow, plus a drop-off-by-node breakdown for
// "steps" flows, so an Admin can see whether a flow is actually working
// instead of building it blind.
func BotFlowAnalyticsHandler(state *appstate.State) gin.HandlerFunc {
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
		args = append([]any{c.Param("id")}, args...)
		query := `SELECT DISTINCT bf.id, bf.name, bf.flow FROM bot_flow bf
		          LEFT JOIN bot_flow_merchant bfm ON bfm.bot_flow_id = bf.id
		          WHERE bf.id = ? AND (bf.is_global = TRUE`
		if len(merchantIDs) > 0 {
			query += ` OR bfm.merchant_id IN (` + placeholders + `)`
		}
		query += `)`

		var flowID int64
		var name, flowRaw string
		if err := conn.QueryRow(query, args...).Scan(&flowID, &name, &flowRaw); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		var flow botengine.FlowDef
		json.Unmarshal([]byte(flowRaw), &flow)
		mode := flow.Mode
		if mode == "" {
			mode = "steps"
		}

		out := botFlowAnalyticsOut{BotFlowID: flowID, Name: name, Mode: mode}
		rows, err := conn.Query(`SELECT outcome, COUNT(*) FROM bot_flow_run WHERE bot_flow_id = ? GROUP BY outcome`, flowID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		for rows.Next() {
			var outcome string
			var n int
			if err := rows.Scan(&outcome, &n); err != nil {
				rows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			switch outcome {
			case "active":
				out.ActiveRuns = n
			case "completed":
				out.CompletedRuns = n
			case "handoff":
				out.HandoffRuns = n
			case "closed":
				out.ClosedRuns = n
			case "abandoned":
				out.AbandonedRuns = n
			}
		}
		rows.Close()

		out.TotalRuns = out.ActiveRuns + out.CompletedRuns + out.HandoffRuns + out.ClosedRuns + out.AbandonedRuns
		out.ResolvedRuns = out.TotalRuns - out.ActiveRuns
		if out.ResolvedRuns > 0 {
			completion := float64(out.CompletedRuns) / float64(out.ResolvedRuns)
			handoff := float64(out.HandoffRuns) / float64(out.ResolvedRuns)
			abandonment := float64(out.AbandonedRuns) / float64(out.ResolvedRuns)
			out.CompletionRate, out.HandoffRate, out.AbandonmentRate = &completion, &handoff, &abandonment
		}

		// Drop-off-by-node is only meaningful for a node graph — an
		// ai_passthrough flow has none to attribute abandonment to.
		if mode != "ai_passthrough" {
			nodeRows, err := conn.Query(
				`SELECT last_node_id, COUNT(*) AS cnt FROM bot_flow_run
				 WHERE bot_flow_id = ? AND outcome = 'abandoned' AND last_node_id IS NOT NULL
				 GROUP BY last_node_id ORDER BY cnt DESC LIMIT 10`, flowID,
			)
			if err == nil {
				labels := botFlowNodeLabels(flow)
				out.DropOffNodes = []botFlowDropOffNodeOut{}
				for nodeRows.Next() {
					var nodeID string
					var n int
					if err := nodeRows.Scan(&nodeID, &n); err != nil {
						continue
					}
					label, ok := labels[nodeID]
					if !ok {
						label = nodeID + " (node removed since)"
					}
					out.DropOffNodes = append(out.DropOffNodes, botFlowDropOffNodeOut{NodeID: nodeID, Label: label, Count: n})
				}
				nodeRows.Close()
			}
		}

		c.JSON(http.StatusOK, out)
	}
}

// botFlowNodeLabels builds a node id -> short human label map from the
// flow's current JSON — a chat's last_node_id can point at a node the
// flow author has since deleted/renamed, so callers must handle a miss.
func botFlowNodeLabels(flow botengine.FlowDef) map[string]string {
	labels := make(map[string]string, len(flow.Nodes))
	for _, n := range flow.Nodes {
		label := n.Type
		if msg, ok := n.Config["message"].(string); ok && msg != "" {
			if len(msg) > 40 {
				msg = msg[:40] + "…"
			}
			label = n.Type + ": " + msg
		}
		labels[n.ID] = label
	}
	return labels
}
