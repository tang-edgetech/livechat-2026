package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

type merchantMini struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type userOut struct {
	UUID        string         `json:"uuid"`
	DisplayName string         `json:"display_name"`
	Username    string         `json:"username"`
	Email       string         `json:"email"`
	Role        string         `json:"role"`
	Status      string         `json:"status"`
	Merchants   []merchantMini `json:"merchants"`
}

// merchantsByUserID batches the user_merchant lookup for a set of users —
// one extra query for the whole list rather than one per row.
func merchantsByUserID(conn *sql.DB, userIDs []int64) (map[int64][]merchantMini, error) {
	result := map[int64][]merchantMini{}
	if len(userIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT um.user_id, m.uuid, m.name FROM user_merchant um
	          JOIN merchant m ON m.id = um.merchant_id
	          WHERE um.user_id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID int64
		var m merchantMini
		if err := rows.Scan(&userID, &m.UUID, &m.Name); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], m)
	}
	return result, nil
}

// actorMerchantIDs returns the set of merchant ids a non-super-admin actor
// already holds — the ceiling on what they're allowed to grant to anyone
// else (overview.md §3: "an Admin can never hand out access to a merchant
// they don't hold").
func actorMerchantIDs(conn *sql.DB, actorID int64) (map[int64]bool, error) {
	rows, err := conn.Query(`SELECT merchant_id FROM user_merchant WHERE user_id = ?`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = true
	}
	return set, nil
}

// ListUsersHandler: Super Admin sees every account; Admin sees only
// Agents that share at least one of the Admin's own merchants
// (overview.md §6.6 — Super Admin is never visible to an Admin).
func ListUsersHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		actorID := c.MustGet("user_id").(int64)

		var rows *sql.Rows
		var err error
		if role == "super_admin" {
			rows, err = conn.Query(
				`SELECT u.id, u.uuid, u.display_name, u.username, u.email, r.slug, u.status
				 FROM user u JOIN role r ON r.id = u.role_id ORDER BY u.created_at DESC`,
			)
		} else {
			rows, err = conn.Query(
				`SELECT DISTINCT u.id, u.uuid, u.display_name, u.username, u.email, r.slug, u.status
				 FROM user u
				 JOIN role r ON r.id = u.role_id
				 JOIN user_merchant um ON um.user_id = u.id
				 WHERE r.slug = 'agent' AND um.merchant_id IN (
				   SELECT merchant_id FROM user_merchant WHERE user_id = ?
				 )
				 ORDER BY u.created_at DESC`,
				actorID,
			)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		type row struct {
			id int64
			u  userOut
		}
		var scanned []row
		var ids []int64
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.u.UUID, &r.u.DisplayName, &r.u.Username, &r.u.Email, &r.u.Role, &r.u.Status); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			scanned = append(scanned, r)
			ids = append(ids, r.id)
		}
		rows.Close()

		merchantMap, err := merchantsByUserID(conn, ids)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		out := []userOut{}
		for _, r := range scanned {
			r.u.Merchants = merchantMap[r.id]
			if r.u.Merchants == nil {
				r.u.Merchants = []merchantMini{}
			}
			out = append(out, r.u)
		}
		c.JSON(http.StatusOK, gin.H{"users": out})
	}
}

type createUserRequest struct {
	Username      string   `json:"username" binding:"required"`
	Email         string   `json:"email" binding:"required"`
	DisplayName   string   `json:"displayName" binding:"required"`
	Password      string   `json:"password" binding:"required"`
	Role          string   `json:"role"`
	MerchantUUIDs []string `json:"merchantUuids"`
}

// CreateUserHandler enforces overview.md §3's assignment rules:
//   - Super Admin creates any account, any role, any merchant(s).
//   - Admin can only create Agents, and only assign merchants the Admin
//     itself already holds.
func CreateUserHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorRole := c.MustGet("role").(string)
		actorID := c.MustGet("user_id").(int64)

		var req createUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if len(req.Password) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_too_short", "detail": "minimum 10 characters"})
			return
		}

		targetRole := req.Role
		if actorRole == "admin" {
			targetRole = "agent" // an Admin can only ever create Agents
		}
		if targetRole != "super_admin" && targetRole != "admin" && targetRole != "agent" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_role"})
			return
		}

		var roleID int64
		if err := conn.QueryRow(`SELECT id FROM role WHERE slug = ?`, targetRole).Scan(&roleID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_role"})
			return
		}

		// Resolve requested merchant uuids to ids, and enforce the Admin
		// ceiling before creating anything.
		var merchantIDs []int64
		if targetRole != "super_admin" {
			var allowed map[int64]bool
			if actorRole == "admin" {
				var err error
				allowed, err = actorMerchantIDs(conn, actorID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
					return
				}
			}
			for _, mUUID := range req.MerchantUUIDs {
				var mID int64
				if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, mUUID).Scan(&mID); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_not_found", "detail": mUUID})
					return
				}
				if actorRole == "admin" && !allowed[mID] {
					c.JSON(http.StatusForbidden, gin.H{"error": "merchant_not_held_by_actor", "detail": mUUID})
					return
				}
				merchantIDs = append(merchantIDs, mID)
			}
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		newUUID := uuid.New().String()
		result, err := conn.Exec(
			`INSERT INTO user (uuid, role_id, display_name, username, email, password_hash, status)
			 VALUES (?, ?, ?, ?, ?, ?, 'active')`,
			newUUID, roleID, req.DisplayName, req.Username, req.Email, string(passwordHash),
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "create_failed", "detail": err.Error()})
			return
		}
		newUserID, _ := result.LastInsertId()

		for _, mID := range merchantIDs {
			if _, err := conn.Exec(
				`INSERT INTO user_merchant (user_id, merchant_id, assigned_by) VALUES (?, ?, ?)`,
				newUserID, mID, actorID,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
		}

		audit.Log(conn, audit.Entry{
			UserID: &actorID, Category: "user", Message: "user created: " + req.Username + " (" + targetRole + ")",
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"uuid": newUUID})
	}
}

// resolveTarget loads a target user's id+role, and — for a non-super-admin
// actor — checks the target is an Agent sharing a merchant with the actor.
// Every write endpoint below (status, password, merchant grant/revoke)
// funnels through this so the scoping rule lives in one place.
func resolveTarget(conn *sql.DB, actorRole string, actorID int64, targetUUID string) (int64, string, error) {
	var targetID int64
	var targetRole string
	err := conn.QueryRow(
		`SELECT u.id, r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.uuid = ?`,
		targetUUID,
	).Scan(&targetID, &targetRole)
	if err != nil {
		return 0, "", err
	}
	if actorRole == "super_admin" {
		return targetID, targetRole, nil
	}
	if targetRole != "agent" {
		return 0, "", sql.ErrNoRows
	}
	var shared int
	err = conn.QueryRow(
		`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id IN (
		   SELECT merchant_id FROM user_merchant WHERE user_id = ?
		 )`,
		targetID, actorID,
	).Scan(&shared)
	if err != nil || shared == 0 {
		return 0, "", sql.ErrNoRows
	}
	return targetID, targetRole, nil
}

type setStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active inactive suspended"`
}

// SetUserStatusHandler (overview.md §6.2: Admin sets Agent account
// status; Super Admin does this for every account).
func SetUserStatusHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorRole := c.MustGet("role").(string)
		actorID := c.MustGet("user_id").(int64)

		var req setStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		targetID, _, err := resolveTarget(conn, actorRole, actorID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		if _, err := conn.Exec(`UPDATE user SET status = ? WHERE id = ?`, req.Status, targetID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{
			UserID: &actorID, Category: "user", Message: "user status set to " + req.Status,
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type forcePasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required"`
}

// ForcePasswordHandler (overview.md §6.2: Admin can force-change Agent
// passwords; Super Admin can for every account).
func ForcePasswordHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorRole := c.MustGet("role").(string)
		actorID := c.MustGet("user_id").(int64)

		var req forcePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if len(req.NewPassword) < 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_too_short", "detail": "minimum 10 characters"})
			return
		}

		targetID, _, err := resolveTarget(conn, actorRole, actorID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if _, err := conn.Exec(`UPDATE user SET password_hash = ? WHERE id = ?`, string(hash), targetID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{
			UserID: &actorID, Category: "user", Message: "password force-reset",
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type merchantGrantRequest struct {
	MerchantUUID string `json:"merchantUuid" binding:"required"`
}

// GrantUserMerchantHandler / RevokeUserMerchantHandler let an Admin
// grant/revoke merchant access for their own Agents, limited to merchants
// the Admin itself already holds (overview.md §3). Super Admin is
// unrestricted, same as everywhere else.
func GrantUserMerchantHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorRole := c.MustGet("role").(string)
		actorID := c.MustGet("user_id").(int64)

		var req merchantGrantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		targetID, targetRole, err := resolveTarget(conn, actorRole, actorID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if targetRole == "super_admin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "super_admin_has_no_merchants"})
			return
		}

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, req.MerchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "merchant_not_found"})
			return
		}

		if actorRole == "admin" {
			allowed, err := actorMerchantIDs(conn, actorID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			if !allowed[merchantID] {
				c.JSON(http.StatusForbidden, gin.H{"error": "merchant_not_held_by_actor"})
				return
			}
		}

		if _, err := conn.Exec(
			`INSERT INTO user_merchant (user_id, merchant_id, assigned_by) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE assigned_by = assigned_by`,
			targetID, merchantID, actorID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{
			UserID: &actorID, Category: "user", Message: "merchant access granted",
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func RevokeUserMerchantHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		actorRole := c.MustGet("role").(string)
		actorID := c.MustGet("user_id").(int64)

		targetID, targetRole, err := resolveTarget(conn, actorRole, actorID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if targetRole == "super_admin" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "super_admin_has_no_merchants"})
			return
		}

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, c.Param("merchantUuid")).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "merchant_not_found"})
			return
		}

		if actorRole == "admin" {
			allowed, err := actorMerchantIDs(conn, actorID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			if !allowed[merchantID] {
				c.JSON(http.StatusForbidden, gin.H{"error": "merchant_not_held_by_actor"})
				return
			}
		}

		if _, err := conn.Exec(
			`DELETE FROM user_merchant WHERE user_id = ? AND merchant_id = ?`,
			targetID, merchantID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{
			UserID: &actorID, Category: "user", Message: "merchant access revoked",
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
