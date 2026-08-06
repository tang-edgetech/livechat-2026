package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/password"
)

var validThemes = map[string]bool{"light": true, "dark": true, "violet": true}
var validManualStatuses = map[string]bool{"online": true, "offline": true}

type updateProfileRequest struct {
	DisplayName     *string `json:"displayName"`
	ThemePreference *string `json:"themePreference"`
	ManualStatus    *string `json:"manualStatus"`
}

// UpdateProfileHandler: any user, self-service display name, Appearance
// preference, and availability status (overview.md §6.2/item 4) — all
// optional so e.g. the Sidebar's status toggle can update just that
// field without resending name/theme.
func UpdateProfileHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var req updateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if req.ThemePreference != nil && !validThemes[*req.ThemePreference] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_theme"})
			return
		}
		if req.ManualStatus != nil && !validManualStatuses[*req.ManualStatus] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_status"})
			return
		}

		if req.DisplayName != nil {
			if _, err := conn.Exec(`UPDATE user SET display_name = ? WHERE id = ?`, *req.DisplayName, userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
		}
		if req.ThemePreference != nil {
			if _, err := conn.Exec(`UPDATE user SET theme_preference = ? WHERE id = ?`, *req.ThemePreference, userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
		}
		if req.ManualStatus != nil {
			if _, err := conn.Exec(`UPDATE user SET manual_status = ? WHERE id = ?`, *req.ManualStatus, userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required"`
}

// ChangePasswordHandler: any user, self-service password change — requires
// the current password, unlike ForcePasswordHandler which an Admin/Super
// Admin uses on someone else's account (overview.md §6.2).
func ChangePasswordHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		userID := c.MustGet("user_id").(int64)

		var req changePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if err := password.Validate(req.NewPassword); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_invalid", "detail": "8-16 characters, at least one uppercase letter and one digit"})
			return
		}

		var currentHash string
		if err := conn.QueryRow(`SELECT password_hash FROM user WHERE id = ?`, userID).Scan(&currentHash); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "current_password_incorrect"})
			return
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if _, err := conn.Exec(`UPDATE user SET password_hash = ? WHERE id = ?`, string(newHash), userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{
			UserID: &userID, Category: "auth", Message: "self-service password change",
			StatusCode: 200, Source: "web", IPAddress: c.ClientIP(),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
