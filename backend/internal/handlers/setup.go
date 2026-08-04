package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/config"
	appdb "livechat/backend/internal/db"
	"livechat/backend/internal/password"
	"livechat/backend/internal/redisclient"
)

// isSetupComplete checks the `setting` table for the setup_complete flag.
// A missing table (migrations not run yet) simply means "not complete".
func isSetupComplete(conn *sql.DB) bool {
	var value string
	err := conn.QueryRow(`SELECT value FROM setting WHERE merchant_id IS NULL AND ` + "`key`" + ` = 'setup_complete'`).Scan(&value)
	return err == nil && value == "true"
}

func StatusHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		complete := false
		if conn := state.DB(); conn != nil {
			complete = isSetupComplete(conn)
		}
		c.JSON(http.StatusOK, gin.H{"setupComplete": complete})
	}
}

type checkResult struct {
	Name string `json:"name"`
	Pass bool   `json:"pass"`
	Note string `json:"note"`
}

// ChecklistHandler runs the Setup Wizard's environment checklist
// (overview.md §5 step 1). No PHP check — the backend is Go, XAMPP only
// supplies Apache/MySQL locally.
func ChecklistHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		results := []checkResult{}

		if out, err := exec.Command("node", "--version").Output(); err == nil {
			results = append(results, checkResult{"Node.js", true, strings.TrimSpace(string(out))})
		} else {
			results = append(results, checkResult{"Node.js", false, "not found on PATH"})
		}

		results = append(results, checkResult{"Go backend", true, "running"})

		if conn, err := appdb.ConnectNoDB(cfg); err == nil {
			results = append(results, checkResult{"MySQL", true, "reachable"})
			conn.Close()
		} else {
			results = append(results, checkResult{"MySQL", false, err.Error()})
		}

		if client, err := redisclient.Connect(cfg); err == nil {
			results = append(results, checkResult{"Redis", true, "reachable"})
			client.Close()
		} else {
			results = append(results, checkResult{"Redis", false, "not reachable (optional, recommended) — " + err.Error()})
		}

		if err := os.MkdirAll(cfg.UploadsPath, 0755); err == nil {
			testFile := filepath.Join(cfg.UploadsPath, ".write_test")
			if err := os.WriteFile(testFile, []byte("ok"), 0644); err == nil {
				os.Remove(testFile)
				results = append(results, checkResult{"Uploads directory writable", true, cfg.UploadsPath})
			} else {
				results = append(results, checkResult{"Uploads directory writable", false, err.Error()})
			}
		} else {
			results = append(results, checkResult{"Uploads directory writable", false, err.Error()})
		}

		c.JSON(http.StatusOK, gin.H{"checks": results})
	}
}

type dbTestRequest struct {
	Host     string `json:"host" binding:"required"`
	Port     string `json:"port" binding:"required"`
	Name     string `json:"name" binding:"required"`
	User     string `json:"user" binding:"required"`
	Password string `json:"password"`
}

func DBTestHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req dbTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "message": "invalid_request"})
			return
		}

		testCfg := &config.Config{DBHost: req.Host, DBPort: req.Port, DBUser: req.User, DBPassword: req.Password}
		conn, err := appdb.ConnectNoDB(testCfg)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "message": err.Error()})
			return
		}
		defer conn.Close()
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "connected"})
	}
}

type finishRequest struct {
	DB struct {
		Host     string `json:"host" binding:"required"`
		Port     string `json:"port" binding:"required"`
		Name     string `json:"name" binding:"required"`
		User     string `json:"user" binding:"required"`
		Password string `json:"password"`
	} `json:"db" binding:"required"`
	Site struct {
		Title       string `json:"title" binding:"required"`
		Timezone    string `json:"timezone" binding:"required"`
		BaseURL     string `json:"baseUrl" binding:"required"`
		UploadsPath string `json:"uploadsPath" binding:"required"`
		AppPort     string `json:"appPort" binding:"required"`
		WSPort      string `json:"wsPort" binding:"required"`
	} `json:"site" binding:"required"`
	Admin struct {
		FullName        string `json:"fullName" binding:"required"`
		Email           string `json:"email" binding:"required"`
		Password        string `json:"password" binding:"required"`
		ConfirmPassword string `json:"confirmPassword" binding:"required"`
	} `json:"admin" binding:"required"`
}

// FinishHandler is Setup Wizard step 5 (overview.md §5): writes .env, runs
// migrations, creates the Super Admin, marks setup_complete. Cannot be
// re-run once setup_complete is set.
func FinishHandler(cfg *config.Config, envPath string, state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req finishRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
			return
		}

		if req.Admin.Password != req.Admin.ConfirmPassword {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_mismatch"})
			return
		}
		if err := password.Validate(req.Admin.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password_invalid", "detail": "8-16 characters, at least one uppercase letter and one digit"})
			return
		}

		serverConn, err := appdb.ConnectNoDB(&config.Config{
			DBHost: req.DB.Host, DBPort: req.DB.Port, DBUser: req.DB.User, DBPassword: req.DB.Password,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "db_unreachable", "detail": err.Error()})
			return
		}
		defer serverConn.Close()

		if _, err := serverConn.Exec("CREATE DATABASE IF NOT EXISTS `" + req.DB.Name + "` DEFAULT CHARACTER SET utf8mb4"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create_database_failed", "detail": err.Error()})
			return
		}

		targetCfg := &config.Config{
			DBHost: req.DB.Host, DBPort: req.DB.Port, DBName: req.DB.Name,
			DBUser: req.DB.User, DBPassword: req.DB.Password,
		}
		conn, err := appdb.Connect(targetCfg)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db_connect_failed", "detail": err.Error()})
			return
		}

		if err := appdb.RunMigrations(conn, cfg.MigrationsPath); err != nil {
			conn.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "migration_failed", "detail": err.Error()})
			return
		}

		if isSetupComplete(conn) {
			conn.Close()
			c.JSON(http.StatusConflict, gin.H{"error": "already_complete"})
			return
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Admin.Password), bcrypt.DefaultCost)
		if err != nil {
			conn.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		adminUUID := uuid.New().String()
		_, err = conn.Exec(
			`INSERT INTO user (uuid, role_id, display_name, email, password_hash, status)
			 VALUES (?, 1, ?, ?, ?, 'active')`,
			adminUUID, req.Admin.FullName, req.Admin.Email, string(passwordHash),
		)
		if err != nil {
			conn.Close()
			if strings.Contains(err.Error(), "Duplicate entry") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "email_taken"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create_super_admin_failed", "detail": err.Error()})
			return
		}

		for key, val := range map[string]string{
			"site_title": req.Site.Title,
			"timezone":   req.Site.Timezone,
		} {
			_, _ = conn.Exec("INSERT INTO setting (merchant_id, `key`, value) VALUES (NULL, ?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)", key, val)
		}
		_, err = conn.Exec("INSERT INTO setting (merchant_id, `key`, value) VALUES (NULL, 'setup_complete', 'true') ON DUPLICATE KEY UPDATE value = 'true'")
		if err != nil {
			conn.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		envContent := fmt.Sprintf(
			"APP_PORT=%s\nWS_PORT=%s\nBASE_URL=%s\nDB_HOST=%s\nDB_PORT=%s\nDB_NAME=%s\nDB_USER=%s\nDB_PASSWORD=%s\nUPLOADS_PATH=%s\n",
			req.Site.AppPort, req.Site.WSPort, req.Site.BaseURL,
			req.DB.Host, req.DB.Port, req.DB.Name, req.DB.User, req.DB.Password,
			req.Site.UploadsPath,
		)
		if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
			conn.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "env_write_failed", "detail": err.Error()})
			return
		}

		audit.Log(conn, audit.Entry{Category: "setup", Message: "setup wizard completed, super admin created: " + req.Admin.FullName, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})

		// Publish the now-configured connection so /api/auth/* works
		// immediately, with no process restart required.
		if old := state.DB(); old != nil {
			old.Close()
		}
		state.SetDB(conn)

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
