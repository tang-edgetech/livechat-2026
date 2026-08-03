package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

type auditLogOut struct {
	ID            int64   `json:"id"`
	MerchantName  *string `json:"merchant_name"`
	UserName      *string `json:"user_name"`
	Category      string  `json:"category"`
	Message       string  `json:"message"`
	StatusCode    int     `json:"status_code"`
	StatusMessage *string `json:"status_message"`
	Source        string  `json:"source"`
	IPAddress     *string `json:"ip_address"`
	CreatedAt     string  `json:"created_at"`
}

// ListAuditLogsHandler backs the Audit Logs tab (overview.md §6.7):
// filters, keyword search, sorting. Super Admin sees everything
// including system-level rows (merchant_id NULL); Admin is scoped to
// their own merchants only — a global/system event is Super-Admin-only
// by nature (setup, merchant creation, etc.), same sensitivity tier as
// everywhere else Super Admin outranks Admin.
func ListAuditLogsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		query := `SELECT a.id, m.name, u.display_name, a.category, a.message, a.status_code, a.status_message, a.source, a.ip_address, a.created_at
		          FROM audit_log a
		          LEFT JOIN merchant m ON m.id = a.merchant_id
		          LEFT JOIN user u ON u.id = a.user_id
		          WHERE 1=1`
		countQuery := `SELECT COUNT(*) FROM audit_log a WHERE 1=1`
		var args []any

		if role == "super_admin" {
			// no merchant restriction
		} else {
			if len(merchantIDs) == 0 {
				c.JSON(http.StatusOK, gin.H{"logs": []auditLogOut{}, "total": 0})
				return
			}
			placeholders, mArgs := int64SliceToPlaceholders(merchantIDs)
			query += ` AND a.merchant_id IN (` + placeholders + `)`
			countQuery += ` AND a.merchant_id IN (` + placeholders + `)`
			args = append(args, mArgs...)
		}

		if category := c.Query("category"); category != "" {
			query += ` AND a.category = ?`
			countQuery += ` AND a.category = ?`
			args = append(args, category)
		}
		if userUUID := c.Query("userUuid"); userUUID != "" {
			// Users are only ever addressed by UUID across the trust
			// boundary; resolve to the internal id server-side.
			query += ` AND a.user_id = (SELECT id FROM user WHERE uuid = ?)`
			countQuery += ` AND a.user_id = (SELECT id FROM user WHERE uuid = ?)`
			args = append(args, userUUID)
		}
		if source := c.Query("source"); source != "" {
			query += ` AND a.source = ?`
			countQuery += ` AND a.source = ?`
			args = append(args, source)
		}
		if statusCode := c.Query("statusCode"); statusCode != "" {
			query += ` AND a.status_code = ?`
			countQuery += ` AND a.status_code = ?`
			args = append(args, statusCode)
		}
		if from := c.Query("from"); from != "" {
			query += ` AND a.created_at >= ?`
			countQuery += ` AND a.created_at >= ?`
			args = append(args, from)
		}
		if to := c.Query("to"); to != "" {
			query += ` AND a.created_at <= ?`
			countQuery += ` AND a.created_at <= ?`
			args = append(args, to)
		}
		if search := c.Query("search"); search != "" {
			query += ` AND a.message LIKE ?`
			countQuery += ` AND a.message LIKE ?`
			args = append(args, "%"+search+"%")
		}

		var total int
		if err := conn.QueryRow(countQuery, args...).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}

		sortColumns := map[string]string{
			"created_at":  "a.created_at",
			"category":    "a.category",
			"status_code": "a.status_code",
			"source":      "a.source",
		}
		sortCol, ok := sortColumns[c.Query("sortBy")]
		if !ok {
			sortCol = "a.created_at"
		}
		sortDir := "DESC"
		if c.Query("sortDir") == "asc" {
			sortDir = "ASC"
		}
		query += ` ORDER BY ` + sortCol + ` ` + sortDir
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 500 {
			pageSize = 20
		}
		query += ` LIMIT ? OFFSET ?`
		args = append(args, pageSize, (page-1)*pageSize)

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		defer rows.Close()

		out := []auditLogOut{}
		for rows.Next() {
			var l auditLogOut
			if err := rows.Scan(&l.ID, &l.MerchantName, &l.UserName, &l.Category, &l.Message, &l.StatusCode, &l.StatusMessage, &l.Source, &l.IPAddress, &l.CreatedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, l)
		}
		c.JSON(http.StatusOK, gin.H{"logs": out, "total": total})
	}
}

// DeleteAuditLogHandler — Super Admin only (overview.md §6.7/§1.1: "only
// role that can delete Audit Logs").
func DeleteAuditLogHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		if _, err := conn.Exec(`DELETE FROM audit_log WHERE id = ?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "audit_log", Message: "audit log entry deleted (id " + id + ")", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
