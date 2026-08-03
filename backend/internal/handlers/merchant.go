package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	Name             string `json:"name" binding:"required"`
	Code             string `json:"code" binding:"required"`
	InitialAdminUUID string `json:"initialAdminUuid"`
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

type merchantDetailOut struct {
	UUID                     string  `json:"uuid"`
	Name                     string  `json:"name"`
	Code                     string  `json:"code"`
	Status                   string  `json:"status"`
	RoutingMode              string  `json:"routing_mode"`
	WidgetConfig             *string `json:"widget_config"`
	InactivityTimeoutMinutes int     `json:"inactivity_timeout_minutes"`
	HasWidgetIdentity        bool    `json:"has_widget_identity"`
	HasAutoLogin             bool    `json:"has_auto_login"`
}

// GetMerchantHandler backs the branding/routing/timeout edit screen
// (overview.md §6.10/§10.4 — "full branding/routing config UI comes in
// Phase 3/4, this phase just needs the record + assignment working").
func GetMerchantHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)
		merchantUUID := c.Param("uuid")

		var out merchantDetailOut
		var merchantID int64
		err := conn.QueryRow(
			`SELECT id, uuid, name, code, status, routing_mode, widget_config, inactivity_timeout_minutes
			 FROM merchant WHERE uuid = ?`,
			merchantUUID,
		).Scan(&merchantID, &out.UUID, &out.Name, &out.Code, &out.Status, &out.RoutingMode, &out.WidgetConfig, &out.InactivityTimeoutMinutes)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		if role != "super_admin" {
			var has int
			conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, merchantID).Scan(&has)
			if has == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		conn.QueryRow(
			`SELECT COUNT(*) > 0 FROM integration_merchant im
			 JOIN integration i ON i.id = im.integration_id
			 WHERE im.merchant_id = ? AND i.type = 'widget_identity'`,
			merchantID,
		).Scan(&out.HasWidgetIdentity)

		conn.QueryRow(
			`SELECT COUNT(*) > 0 FROM integration_merchant im
			 JOIN integration i ON i.id = im.integration_id
			 WHERE im.merchant_id = ? AND i.type = 'auto_login'`,
			merchantID,
		).Scan(&out.HasAutoLogin)

		c.JSON(http.StatusOK, out)
	}
}

type updateMerchantRequest struct {
	RoutingMode              *string `json:"routingMode"`
	WidgetConfig             *string `json:"widgetConfig"`
	InactivityTimeoutMinutes *int    `json:"inactivityTimeoutMinutes"`
}

// UpdateMerchantHandler: Admin can edit their own held merchant's
// operational config; Super Admin can edit any (overview.md §6.10).
func UpdateMerchantHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)
		merchantUUID := c.Param("uuid")

		var req updateMerchantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if role != "super_admin" {
			var has int
			conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, merchantID).Scan(&has)
			if has == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		if req.RoutingMode != nil {
			if *req.RoutingMode != "manual" && *req.RoutingMode != "round_robin" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_routing_mode"})
				return
			}
			conn.Exec(`UPDATE merchant SET routing_mode = ? WHERE id = ?`, *req.RoutingMode, merchantID)
		}
		if req.WidgetConfig != nil {
			conn.Exec(`UPDATE merchant SET widget_config = ? WHERE id = ?`, *req.WidgetConfig, merchantID)
		}
		if req.InactivityTimeoutMinutes != nil {
			if *req.InactivityTimeoutMinutes < 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_inactivity_timeout"})
				return
			}
			conn.Exec(`UPDATE merchant SET inactivity_timeout_minutes = ? WHERE id = ?`, *req.InactivityTimeoutMinutes, merchantID)
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, UserID: &userID, Category: "merchant",
			Message: "merchant config updated", StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// GenerateWidgetIdentityHandler (Super Admin only, overview.md §10.2):
// mints the HMAC secret a merchant's own backend uses to sign a
// logged-in visitor's passthrough payload. Shown to the caller exactly
// once — only ever stored server-side from here on. Rotating replaces
// the previous secret outright (old signed payloads stop verifying).
func GenerateWidgetIdentityHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		merchantUUID := c.Param("uuid")

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		secret := hex.EncodeToString(secretBytes)

		tx, err := conn.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer tx.Rollback()

		tx.Exec(
			`DELETE i FROM integration i JOIN integration_merchant im ON im.integration_id = i.id
			 WHERE im.merchant_id = ? AND i.type = 'widget_identity'`,
			merchantID,
		)
		result, err := tx.Exec(`INSERT INTO integration (type, secret_hash, is_global) VALUES ('widget_identity', ?, FALSE)`, secret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		integrationID, _ := result.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO integration_merchant (integration_id, merchant_id) VALUES (?, ?)`, integrationID, merchantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, UserID: &userID, Category: "integration",
			Message: "widget identity secret (re)generated", StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"secret": secret})
	}
}

// GenerateAutoLoginHandler (Super Admin only, overview.md §6.5/§9): mints
// the shared secret a B2B partner's own backend uses to HMAC-sign an
// auto-login deep link (see internal/autologin) — same mint-and-replace
// shape as GenerateWidgetIdentityHandler above, just a separate secret
// since this one grants panel access rather than identifying a Visitor.
func GenerateAutoLoginHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		merchantUUID := c.Param("uuid")

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		secret := hex.EncodeToString(secretBytes)

		tx, err := conn.Begin()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer tx.Rollback()

		tx.Exec(
			`DELETE i FROM integration i JOIN integration_merchant im ON im.integration_id = i.id
			 WHERE im.merchant_id = ? AND i.type = 'auto_login'`,
			merchantID,
		)
		result, err := tx.Exec(`INSERT INTO integration (type, secret_hash, is_global) VALUES ('auto_login', ?, FALSE)`, secret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		integrationID, _ := result.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO integration_merchant (integration_id, merchant_id) VALUES (?, ?)`, integrationID, merchantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, UserID: &userID, Category: "integration",
			Message: "auto-login secret (re)generated", StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"secret": secret})
	}
}
