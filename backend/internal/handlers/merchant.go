package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

type merchantOut struct {
	UUID   string `json:"uuid"`
	ID     int64  `json:"-"`
	Name   string `json:"name"`
	Code   string `json:"code"`
	Status string `json:"status"`
}

// ListMerchantsHandler: Super Admin sees every merchant; Admin sees only
// the merchant(s) already granted to them via user_merchant (overview.md
// §3/§6.10). Not reachable by Agent — gated by RequireRole in main.go.
func ListMerchantsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var rows *sql.Rows
		var err error
		if role == "super_admin" {
			rows, err = conn.Query(`SELECT id, uuid, name, code, status FROM merchant ORDER BY created_at DESC`)
		} else {
			rows, err = conn.Query(
				`SELECT m.id, m.uuid, m.name, m.code, m.status FROM merchant m
				 JOIN user_merchant um ON um.merchant_id = m.id
				 WHERE um.user_id = ? ORDER BY m.created_at DESC`,
				userID,
			)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		out := []merchantOut{}
		for rows.Next() {
			var m merchantOut
			if err := rows.Scan(&m.ID, &m.UUID, &m.Name, &m.Code, &m.Status); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, m)
		}
		c.JSON(http.StatusOK, gin.H{"merchants": out})
	}
}

type createMerchantRequest struct {
	Name              string `json:"name" binding:"required"`
	Code              string `json:"code" binding:"required"`
	InitialAdminUUID  string `json:"initialAdminUuid"`
}

// CreateMerchantHandler is Super Admin only (overview.md §6.10) — creates
// the merchant record and, if an initial Admin is given, grants them
// access in the same request (the actual "onboard a new customer" flow).
func CreateMerchantHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorID := c.MustGet("user_id").(int64)

		var req createMerchantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		merchantUUID := uuid.New().String()
		result, err := conn.Exec(
			`INSERT INTO merchant (uuid, name, code, status) VALUES (?, ?, ?, 'active')`,
			merchantUUID, req.Name, req.Code,
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "create_failed", "detail": err.Error()})
			return
		}
		merchantID, _ := result.LastInsertId()

		if req.InitialAdminUUID != "" {
			var adminID int64
			var roleSlug string
			err := conn.QueryRow(
				`SELECT u.id, r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.uuid = ?`,
				req.InitialAdminUUID,
			).Scan(&adminID, &roleSlug)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "initial_admin_not_found"})
				return
			}
			if roleSlug != "admin" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "initial_admin_must_be_admin_role"})
				return
			}
			if _, err := conn.Exec(
				`INSERT INTO user_merchant (user_id, merchant_id, assigned_by) VALUES (?, ?, ?)`,
				adminID, merchantID, actorID,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "assign_admin_failed", "detail": err.Error()})
				return
			}
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, UserID: &actorID, Category: "merchant",
			Message: "merchant created: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})

		c.JSON(http.StatusOK, gin.H{"uuid": merchantUUID})
	}
}

type merchantStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active suspended"`
}

// SetMerchantStatusHandler (suspend/reactivate) is Super Admin only.
func SetMerchantStatusHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorID := c.MustGet("user_id").(int64)
		merchantUUID := c.Param("uuid")

		var req merchantStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		if _, err := conn.Exec(`UPDATE merchant SET status = ? WHERE id = ?`, req.Status, merchantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, UserID: &actorID, Category: "merchant",
			Message: "merchant status set to " + req.Status, StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type assignAdminRequest struct {
	UserUUID string `json:"userUuid" binding:"required"`
}

// AssignMerchantAdminHandler grants an existing Admin access to a
// merchant they don't already hold. Super Admin only — an Admin can
// never hand out access to a merchant they don't have (overview.md §3),
// and granting the *first* Admin is the one action reserved for Super
// Admin regardless.
func AssignMerchantAdminHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorID := c.MustGet("user_id").(int64)
		merchantUUID := c.Param("uuid")

		var req assignAdminRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "merchant_not_found"})
			return
		}

		var userID int64
		var roleSlug string
		if err := conn.QueryRow(
			`SELECT u.id, r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.uuid = ?`,
			req.UserUUID,
		).Scan(&userID, &roleSlug); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
			return
		}
		if roleSlug != "admin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_must_be_admin_role"})
			return
		}

		if _, err := conn.Exec(
			`INSERT INTO user_merchant (user_id, merchant_id, assigned_by) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE assigned_by = assigned_by`,
			userID, merchantID, actorID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, UserID: &actorID, Category: "merchant",
			Message: "admin assigned to merchant", StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
