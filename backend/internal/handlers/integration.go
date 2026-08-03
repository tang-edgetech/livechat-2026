package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
)

// Webhook integrations back the Bot flow's "Connect to another system"
// step (overview.md §6.4/§6.5) — the one place a real endpoint URL and
// secret are unavoidable. Super Admin/dev team sets these up; the Admin
// building a flow only ever picks one by name (ListIntegrationsHandler
// deliberately omits the URL/secret from the response).

type webhookConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type integrationOut struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func ListIntegrationsHandler(state *appstate.State) gin.HandlerFunc {
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
		query := `SELECT DISTINCT i.id, i.config FROM integration i
		          LEFT JOIN integration_merchant im ON im.integration_id = i.id
		          WHERE i.type = 'webhook' AND (i.is_global = TRUE`
		if len(merchantIDs) > 0 {
			query += ` OR im.merchant_id IN (` + placeholders + `)`
		}
		query += `)`

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		out := []integrationOut{}
		for rows.Next() {
			var id int64
			var configRaw string
			if err := rows.Scan(&id, &configRaw); err != nil {
				continue
			}
			var cfg webhookConfig
			json.Unmarshal([]byte(configRaw), &cfg)
			out = append(out, integrationOut{ID: id, Name: cfg.Name})
		}
		c.JSON(http.StatusOK, gin.H{"integrations": out})
	}
}

type createIntegrationRequest struct {
	Name         string  `json:"name" binding:"required"`
	URL          string  `json:"url" binding:"required"`
	Secret       string  `json:"secret"`
	IsGlobal     bool    `json:"isGlobal"`
	MerchantUUID *string `json:"merchantUuid"`
}

// CreateIntegrationHandler is Super Admin only — see the package comment.
func CreateIntegrationHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var req createIntegrationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		var merchantID *int64
		if req.MerchantUUID != nil && *req.MerchantUUID != "" {
			var id int64
			if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, *req.MerchantUUID).Scan(&id); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_not_found"})
				return
			}
			merchantID = &id
		} else if !req.IsGlobal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "merchant_required_unless_global"})
			return
		}

		secret := req.Secret
		if secret == "" {
			secretBytes := make([]byte, 24)
			rand.Read(secretBytes)
			secret = hex.EncodeToString(secretBytes)
		}

		configBytes, _ := json.Marshal(webhookConfig{Name: req.Name, URL: req.URL})
		result, err := conn.Exec(
			`INSERT INTO integration (type, config, secret_hash, is_global) VALUES ('webhook', ?, ?, ?)`,
			string(configBytes), secret, req.IsGlobal,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		id, _ := result.LastInsertId()
		if merchantID != nil {
			conn.Exec(`INSERT INTO integration_merchant (integration_id, merchant_id) VALUES (?, ?)`, id, *merchantID)
		}

		audit.Log(conn, audit.Entry{MerchantID: merchantID, UserID: &userID, Category: "integration", Message: "webhook integration created: " + req.Name, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"id": id})
	}
}

func DeleteIntegrationHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)
		id := c.Param("id")

		conn.Exec(`DELETE FROM integration_merchant WHERE integration_id = ?`, id)
		if _, err := conn.Exec(`DELETE FROM integration WHERE id = ?`, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "integration", Message: "webhook integration deleted", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// TestIntegrationHandler gives the technical setup step a plain
// pass/fail (overview.md §6.5) rather than requiring protocol knowledge
// to interpret a raw error.
func TestIntegrationHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		var configRaw string
		if err := conn.QueryRow(`SELECT config FROM integration WHERE id = ?`, c.Param("id")).Scan(&configRaw); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "message": "not found"})
			return
		}
		var cfg webhookConfig
		json.Unmarshal([]byte(configRaw), &cfg)

		client := http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(cfg.URL)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "message": err.Error()})
			return
		}
		defer resp.Body.Close()
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "reachable (status " + resp.Status + ")"})
	}
}
