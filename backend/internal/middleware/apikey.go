package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
)

// RequireAPIKey backs the inbound REST API v1 (overview.md §6.5/§9: "REST
// API — inbound... Bearer API-key auth for server-to-server calls"). The
// raw key is only ever shown once at creation (ApiKeysHandler); every
// call after that only ever compares its SHA-256 hash, the same
// never-store-the-secret pattern as `session.token_hash`.
func RequireAPIKey(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_api_key"})
			return
		}
		rawKey := strings.TrimPrefix(auth, "Bearer ")
		sum := sha256.Sum256([]byte(rawKey))
		hash := hex.EncodeToString(sum[:])

		var (
			keyID      int64
			merchantID int64
		)
		err := state.DB().QueryRow(
			`SELECT i.id, im.merchant_id FROM integration i
			 JOIN integration_merchant im ON im.integration_id = i.id
			 WHERE i.type = 'api_key' AND i.secret_hash = ?`,
			hash,
		).Scan(&keyID, &merchantID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_key"})
			return
		}

		c.Set("api_key_id", keyID)
		c.Set("api_merchant_id", merchantID)
		c.Next()
	}
}
