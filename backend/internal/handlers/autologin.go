package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/autologin"
	"livechat/backend/internal/config"
	"livechat/backend/internal/middleware"
	"livechat/backend/internal/session"
)

// AutoLoginHandler is the B2B deep-link entry point (overview.md §6.5/§9)
// — unauthenticated by design, the same way the widget's passthrough
// endpoint is: trust nothing until the HMAC signature verifies against
// this merchant's own `auto_login` secret (internal/autologin.Verify).
// On success it mints a REAL session exactly like LoginHandler does, just
// without a password, then redirects into the panel — a partner's
// end user never sees or touches a LiveChat credential.
func AutoLoginHandler(state *appstate.State, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		merchantCode := c.Query("merchant")
		token := c.Query("token")

		var merchantID int64
		var merchantStatus string
		if err := conn.QueryRow(`SELECT id, status FROM merchant WHERE code = ?`, merchantCode).Scan(&merchantID, &merchantStatus); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "merchant_not_found"})
			return
		}
		if merchantStatus != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "merchant_suspended"})
			return
		}

		identity, err := autologin.Verify(conn, merchantID, token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_auto_login_token"})
			return
		}

		var (
			userID   int64
			status   string
			roleSlug string
		)
		err = conn.QueryRow(
			`SELECT u.id, u.status, r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.uuid = ?`,
			identity.UserUUID,
		).Scan(&userID, &status, &roleSlug)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
			return
		}
		if status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "account_" + status})
			return
		}
		if roleSlug != "super_admin" {
			var has int
			conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, merchantID).Scan(&has)
			if has == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		tok, err := session.Create(conn, userID, c.ClientIP(), c.Request.UserAgent())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{MerchantID: &merchantID, UserID: &userID, Category: "auth", Message: "B2B auto-login", StatusCode: 200, Source: "api", IPAddress: c.ClientIP()})

		secure := c.Request.TLS != nil
		c.SetCookie(middleware.SessionCookieName, tok, int(session.IdleTimeout.Seconds()), "/", "", secure, true)
		c.Redirect(http.StatusFound, cfg.FrontendOrigin+"/dashboard")
	}
}
