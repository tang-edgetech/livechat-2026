package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/middleware"
	"livechat/backend/internal/session"
)

type loginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginHandler authenticates by username OR email + password, per
// overview.md §4.1 ("Login is username/email + password only").
func LoginHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var (
			id           int64
			passwordHash string
			status       string
		)
		err := conn.QueryRow(
			`SELECT id, password_hash, status FROM user WHERE username = ? OR email = ?`,
			req.Login, req.Login,
		).Scan(&id, &passwordHash, &status)

		if err == sql.ErrNoRows {
			audit.Log(conn, audit.Entry{Category: "auth", Message: "login failed: unknown login " + req.Login, StatusCode: 401, Source: "web", IPAddress: c.ClientIP()})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
			audit.Log(conn, audit.Entry{UserID: &id, Category: "auth", Message: "login failed: bad password", StatusCode: 401, Source: "web", IPAddress: c.ClientIP()})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
			return
		}

		if status != "active" {
			audit.Log(conn, audit.Entry{UserID: &id, Category: "auth", Message: "login blocked: account " + status, StatusCode: 403, Source: "web", IPAddress: c.ClientIP()})
			c.JSON(http.StatusForbidden, gin.H{"error": "account_" + status})
			return
		}

		token, err := session.Create(conn, id, c.ClientIP(), c.Request.UserAgent())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		_, _ = conn.Exec(`UPDATE user SET last_login_at = ? WHERE id = ?`, time.Now(), id)
		audit.Log(conn, audit.Entry{UserID: &id, Category: "auth", Message: "login success", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})

		secure := c.Request.TLS != nil
		c.SetCookie(middleware.SessionCookieName, token, int(session.IdleTimeout.Seconds()), "/", "", secure, true)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func LogoutHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		token, err := c.Cookie(middleware.SessionCookieName)
		if err == nil && token != "" {
			_ = session.Delete(conn, token)
		}
		c.SetCookie(middleware.SessionCookieName, "", -1, "/", "", false, true)

		if uid, ok := c.Get("user_id"); ok {
			id := uid.(int64)
			audit.Log(conn, audit.Entry{UserID: &id, Category: "auth", Message: "logout", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func MeHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var (
			uuid, displayName, email, roleSlug string
		)
		err := conn.QueryRow(
			`SELECT u.uuid, u.display_name, u.email, r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.id = ?`,
			userID,
		).Scan(&uuid, &displayName, &email, &roleSlug)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"uuid":         uuid,
			"display_name": displayName,
			"email":        email,
			"role":         roleSlug,
		})
	}
}
