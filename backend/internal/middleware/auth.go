package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/session"
)

const SessionCookieName = "session_token"

// RequireAuth validates the session cookie on every protected route. The
// 2-hour idle logout (overview.md §6.0) is enforced here server-side via
// session.Validate — a tampered client-side timer cannot bypass it.
func RequireAuth(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookieName)
		if err != nil || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not_authenticated"})
			return
		}

		s, err := session.Validate(state.DB(), token)
		if err != nil {
			c.SetCookie(SessionCookieName, "", -1, "/", "", false, true)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session_expired"})
			return
		}

		var roleSlug string
		if err := state.DB().QueryRow(
			`SELECT r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.id = ?`,
			s.UserID,
		).Scan(&roleSlug); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		c.Set("user_id", s.UserID)
		c.Set("role", roleSlug)
		c.Next()
	}
}

// RequireRole must run after RequireAuth. It enforces RBAC server-side —
// overview.md §3/Phase 1: "RBAC enforced on every route from here on, not
// bolted on later."
func RequireRole(allowed ...string) gin.HandlerFunc {
	set := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		set[r] = true
	}
	return func(c *gin.Context) {
		role := c.MustGet("role").(string)
		if !set[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}
