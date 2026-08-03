package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/retention"
	"livechat/backend/internal/settings"
	"livechat/backend/internal/storage"
)

// GetSettingsHandler backs the General/System/Files settings tabs — any
// authed user can view (System/Files tabs are staff-only at the route
// group level; General is visible to everyone), only Super Admin can
// change values (UpdateSettingsHandler).
func GetSettingsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := settings.GetAll(state.DB())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"settings": out})
	}
}

// UpdateSettingsHandler — Super Admin only (overview.md §9: these are
// global, not per-merchant, values).
func UpdateSettingsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var req map[string]string
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		for key, value := range req {
			if _, ok := settings.Defaults[key]; !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "unknown_setting", "detail": key})
				return
			}
			if err := settings.Set(conn, key, value, userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "settings", Message: "settings updated", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})

		out, _ := settings.GetAll(conn)
		c.JSON(http.StatusOK, gin.H{"settings": out})
	}
}

type fileRulesOut struct {
	AllowedExtensions string `json:"allowedExtensions"`
	MaxSizeMB         string `json:"maxSizeMb"`
	HasOverride       bool   `json:"hasOverride"`
}

// GetFileRulesHandler returns this merchant's file-rule override if one
// exists, else the global default with hasOverride=false so the UI can
// show "using global default" (overview.md §6.8 — Super Admin can exempt
// or tighten a specific merchant rather than only a single global rule).
func GetFileRulesHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, c.Param("uuid")).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		ext, hasExt, err := settings.GetMerchant(conn, merchantID, "file_allowed_extensions")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		maxMB, hasMax, err := settings.GetMerchant(conn, merchantID, "file_max_size_mb")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		c.JSON(http.StatusOK, fileRulesOut{AllowedExtensions: ext, MaxSizeMB: maxMB, HasOverride: hasExt || hasMax})
	}
}

type updateFileRulesRequest struct {
	AllowedExtensions *string `json:"allowedExtensions"`
	MaxSizeMB         *string `json:"maxSizeMb"`
	ClearOverride     bool    `json:"clearOverride"`
}

// UpdateFileRulesHandler — Super Admin only. ClearOverride removes the
// merchant-scoped rows entirely (reverting to the global default);
// otherwise any provided field is upserted as this merchant's override.
// An override value of "" (extensions) or "0" (size) is a deliberate
// "don't limit this merchant" switch — the existing rule-checking code in
// validateFileRules already treats empty/<=0 as unlimited.
func UpdateFileRulesHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var merchantID int64
		if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, c.Param("uuid")).Scan(&merchantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		var req updateFileRulesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		if req.ClearOverride {
			settings.ClearMerchant(conn, merchantID, "file_allowed_extensions")
			settings.ClearMerchant(conn, merchantID, "file_max_size_mb")
		} else {
			if req.AllowedExtensions != nil {
				if err := settings.SetMerchant(conn, merchantID, "file_allowed_extensions", *req.AllowedExtensions, userID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
					return
				}
			}
			if req.MaxSizeMB != nil {
				if err := settings.SetMerchant(conn, merchantID, "file_max_size_mb", *req.MaxSizeMB, userID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
					return
				}
			}
		}

		audit.Log(conn, audit.Entry{MerchantID: &merchantID, UserID: &userID, Category: "settings", Message: "merchant file rules updated", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})

		ext, hasExt, _ := settings.GetMerchant(conn, merchantID, "file_allowed_extensions")
		maxMB, hasMax, _ := settings.GetMerchant(conn, merchantID, "file_max_size_mb")
		c.JSON(http.StatusOK, fileRulesOut{AllowedExtensions: ext, MaxSizeMB: maxMB, HasOverride: hasExt || hasMax})
	}
}

// PurgeNowHandler runs the retention sweep immediately on demand
// (overview.md §9's "manual purge now action"), Super Admin only.
func PurgeNowHandler(state *appstate.State, driver storage.Driver) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		report, err := retention.Sweep(conn, driver)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		audit.Log(conn, audit.Entry{UserID: &userID, Category: "settings", Message: "manual retention purge run", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"report": report})
	}
}
