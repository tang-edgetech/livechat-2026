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
