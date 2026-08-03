package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

// API keys back the inbound half of REST API v1 (overview.md §6.5) — a
// server-to-server credential scoped to exactly one merchant, unlike a
// webhook integration which can be global. Super Admin only, same as
// every other raw-secret-bearing row under Settings > Integration.

type apiKeyConfig struct {
	Name string `json:"name"`
}

type apiKeyOut struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	MerchantName string `json:"merchant_name"`
	CreatedAt    string `json:"created_at"`
}

func ListAPIKeysHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		query := `SELECT i.id, i.config, m.name, i.created_at FROM integration i
		          JOIN integration_merchant im ON im.integration_id = i.id
		          JOIN merchant m ON m.id = im.merchant_id
		          WHERE i.type = 'api_key'`
		var args []any
		if role != "super_admin" {
			if len(merchantIDs) == 0 {
				c.JSON(http.StatusOK, gin.H{"apiKeys": []apiKeyOut{}})
				return
			}
			placeholders, mArgs := int64SliceToPlaceholders(merchantIDs)
			query += ` AND im.merchant_id IN (` + placeholders + `)`
			args = append(args, mArgs...)
		}

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		out := []apiKeyOut{}
		for rows.Next() {
			var id int64
			var configRaw, merchantName, createdAt string
			if err := rows.Scan(&id, &configRaw, &merchantName, &createdAt); err != nil {
				continue
			}
			var cfg apiKeyConfig
			json.Unmarshal([]byte(configRaw), &cfg)
			out = append(out, apiKeyOut{ID: id, Name: cfg.Name, MerchantName: merchantName, CreatedAt: createdAt})
		}
		c.JSON(http.StatusOK, gin.H{"apiKeys": out})
	}
}

type createAPIKeyRequest struct {
	Name         string `json:"name" binding:"required"`
	MerchantUUID string `json:"merchantUuid" binding:"required"`
}

// CreateAPIKeyHandler is Super Admin only. The raw key is returned exactly
// once — only its SHA-256 hash is persisted (RequireAPIKey compares
// against that hash on every call), same discipline as `session`.
func CreateAPIKeyHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var req createAPIKeyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, req.MerchantUUID).Scan(&merchantID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_not_found"})
			return
		}

		keyBytes := make([]byte, 24)
		if _, err := rand.Read(keyBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		rawKey := "lc_" + hex.EncodeToString(keyBytes)
		sum := sha256.Sum256([]byte(rawKey))
		hash := hex.EncodeToString(sum[:])

		configBytes, _ := json.Marshal(apiKeyConfig{Name: req.Name})
		result, err := conn.Exec(`INSERT INTO integration (type, config, secret_hash, is_global) VALUES ('api_key', ?, ?, FALSE)`, string(configBytes), hash)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		id, _ := result.LastInsertId()
		if _, err := conn.Exec(`INSERT INTO integration_merchant (integration_id, merchant_id) VALUES (?, ?)`, id, merchantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{MerchantID: &merchantID, UserID: &userID, Category: "integration", Message: "API key created: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"id": id, "apiKey": rawKey})
	}
}

func RevokeAPIKeyHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		conn.Exec(`DELETE FROM integration_merchant WHERE integration_id = ?`, id)
		if _, err := conn.Exec(`DELETE FROM integration WHERE id = ? AND type = 'api_key'`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "integration", Message: "API key revoked", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
