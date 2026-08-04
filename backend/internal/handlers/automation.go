package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/htmlguard"
)

type automationRuleOut struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Condition    *string `json:"condition"`
	Message      string  `json:"message"`
	IsGlobal     bool    `json:"is_global"`
	IsActive     bool    `json:"is_active"`
	IsHtml       bool    `json:"is_html"`
	MerchantUUID *string `json:"merchant_uuid"`
}

// ListAutomationRulesHandler returns every rule visible to the requester:
// global rules, plus rules scoped to any merchant they hold (overview.md
// §6.3). Super Admin's scope already covers every merchant, so this one
// query serves everyone.
func ListAutomationRulesHandler(state *appstate.State) gin.HandlerFunc {
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
		query := `SELECT DISTINCT r.id, r.name, r.condition, r.message, r.is_global, r.is_active, r.is_html, m.uuid
		          FROM automation_rule r
		          LEFT JOIN automation_rule_merchant arm ON arm.automation_rule_id = r.id
		          LEFT JOIN merchant m ON m.id = arm.merchant_id
		          WHERE r.is_global = TRUE`
		if len(merchantIDs) > 0 {
			query += ` OR arm.merchant_id IN (` + placeholders + `)`
		}
		query += ` ORDER BY r.created_at DESC`

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		defer rows.Close()

		out := []automationRuleOut{}
		for rows.Next() {
			var r automationRuleOut
			if err := rows.Scan(&r.ID, &r.Name, &r.Condition, &r.Message, &r.IsGlobal, &r.IsActive, &r.IsHtml, &r.MerchantUUID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, r)
		}
		c.JSON(http.StatusOK, gin.H{"rules": out})
	}
}

type saveAutomationRuleRequest struct {
	Name         string  `json:"name" binding:"required"`
	Condition    string  `json:"condition"`
	Message      string  `json:"message" binding:"required"`
	IsGlobal     bool    `json:"isGlobal"`
	IsActive     bool    `json:"isActive"`
	IsHtml       bool    `json:"isHtml"`
	MerchantUUID *string `json:"merchantUuid"`
}

// CreateAutomationRuleHandler: Admin can only create merchant-scoped
// rules for a merchant they hold; Super Admin can also create global
// ones (overview.md §3's general scoping rule, applied here).
func CreateAutomationRuleHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var req saveAutomationRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if req.IsGlobal && role != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only_super_admin_can_create_global_rules"})
			return
		}
		if req.IsHtml && htmlguard.IsDangerous(req.Message) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "html_not_allowed"})
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
			`INSERT INTO automation_rule (name, `+"`condition`"+`, message, is_global, is_active, is_html, created_by) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			req.Name, nullIfEmpty(req.Condition), req.Message, req.IsGlobal, req.IsActive, req.IsHtml, userID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		ruleID, _ := result.LastInsertId()

		if merchantID != nil {
			conn.Exec(`INSERT INTO automation_rule_merchant (automation_rule_id, merchant_id) VALUES (?, ?)`, ruleID, *merchantID)
		}

		audit.Log(conn, audit.Entry{MerchantID: merchantID, UserID: &userID, Category: "automation", Message: "automation rule created: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"id": ruleID})
	}
}

// UpdateAutomationRuleHandler lets Admin/Super Admin edit an existing
// rule in place instead of only create/delete.
func UpdateAutomationRuleHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		var req saveAutomationRuleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if req.IsGlobal && role != "super_admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "only_super_admin_can_create_global_rules"})
			return
		}
		if req.IsHtml && htmlguard.IsDangerous(req.Message) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "html_not_allowed"})
			return
		}

		var merchantID *int64
		if req.MerchantUUID != nil && *req.MerchantUUID != "" {
			mid, err := resolveMerchantInScope(conn, role, userID, *req.MerchantUUID)
			if err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "merchant_not_in_scope"})
				return
			}
			merchantID = &mid
		} else if !req.IsGlobal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_required_unless_global"})
			return
		}

		if _, err := conn.Exec(
			`UPDATE automation_rule SET name = ?, `+"`condition`"+` = ?, message = ?, is_global = ?, is_active = ?, is_html = ? WHERE id = ?`,
			req.Name, nullIfEmpty(req.Condition), req.Message, req.IsGlobal, req.IsActive, req.IsHtml, id,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		conn.Exec(`DELETE FROM automation_rule_merchant WHERE automation_rule_id = ?`, id)
		if merchantID != nil {
			conn.Exec(`INSERT INTO automation_rule_merchant (automation_rule_id, merchant_id) VALUES (?, ?)`, id, *merchantID)
		}

		audit.Log(conn, audit.Entry{MerchantID: merchantID, UserID: &userID, Category: "automation", Message: "automation rule updated: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func DeleteAutomationRuleHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		conn.Exec(`DELETE FROM automation_rule_merchant WHERE automation_rule_id = ?`, id)
		if _, err := conn.Exec(`DELETE FROM automation_rule WHERE id = ?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "automation", Message: "automation rule deleted", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// resolveMerchantInScope resolves a merchant uuid to its internal id,
// only if the requester (Admin scoped to their own, Super Admin any) is
// allowed to use it.
func resolveMerchantInScope(conn *sql.DB, role string, userID int64, merchantUUID string) (int64, error) {
	var id int64
	if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&id); err != nil {
		return 0, err
	}
	if role == "super_admin" {
		return id, nil
	}
	var has int
	conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, id).Scan(&has)
	if has == 0 {
		return 0, sql.ErrNoRows
	}
	return id, nil
}
