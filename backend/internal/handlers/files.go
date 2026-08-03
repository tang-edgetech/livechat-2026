package handlers

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/storage"
)

type fileRef struct {
	DiskPath   string
	MimeType   string
	OrigName   string
	MerchantID int64
	ChatID     sql.NullInt64
}

func lookupFile(conn *sql.DB, fileUUID string) (*fileRef, error) {
	var f fileRef
	err := conn.QueryRow(
		`SELECT disk_path, mime_type, original_name, merchant_id, chat_id FROM file WHERE uuid = ?`, fileUUID,
	).Scan(&f.DiskPath, &f.MimeType, &f.OrigName, &f.MerchantID, &f.ChatID)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func streamFile(c *gin.Context, driver storage.Driver, f *fileRef) {
	reader, err := driver.Get(f.DiskPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file_not_found"})
		return
	}
	defer reader.Close()

	c.Header("Content-Disposition", `inline; filename="`+f.OrigName+`"`)
	c.Header("Content-Type", f.MimeType)
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}

// DownloadFileHandler (staff): the file's merchant must be in the
// requester's scope — same rule as every other chat-scoped resource.
func DownloadFileHandler(state *appstate.State, driver storage.Driver) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		f, err := lookupFile(conn, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if role != "super_admin" {
			var has int
			conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, f.MerchantID).Scan(&has)
			if has == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}
		streamFile(c, driver, f)
	}
}

type fileListOut struct {
	UUID         string  `json:"uuid"`
	OriginalName string  `json:"original_name"`
	MimeType     string  `json:"mime_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Purpose      string  `json:"purpose"`
	MerchantName string  `json:"merchant_name"`
	UploaderType string  `json:"uploader_type"`
	UploaderName *string `json:"uploader_name"`
	CreatedAt    string  `json:"created_at"`
}

// ListFilesHandler backs the Files "Library" view (overview.md §6.8) — a
// browsable list/grid of what's actually been uploaded, scoped the same
// way as every other staff-facing list (scopedMerchantIDs). Filters:
// merchant, a mime-type prefix (e.g. "image/" for the grid view's
// image-only filter), date range, keyword on the original filename.
func ListFilesHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		query := `SELECT f.uuid, f.original_name, f.mime_type, f.size_bytes, f.purpose, f.created_at,
		                 m.name,
		                 f.uploader_type,
		                 CASE f.uploader_type WHEN 'user' THEN u.display_name WHEN 'visitor' THEN v.display_name ELSE NULL END
		          FROM file f
		          JOIN merchant m ON m.id = f.merchant_id
		          LEFT JOIN user u ON f.uploader_type = 'user' AND u.id = f.uploader_id
		          LEFT JOIN visitor v ON f.uploader_type = 'visitor' AND v.id = f.uploader_id
		          WHERE 1=1`
		countQuery := `SELECT COUNT(*) FROM file f WHERE 1=1`
		var args []any

		if role != "super_admin" {
			if len(merchantIDs) == 0 {
				c.JSON(http.StatusOK, gin.H{"files": []fileListOut{}, "total": 0})
				return
			}
			placeholders, mArgs := int64SliceToPlaceholders(merchantIDs)
			query += ` AND f.merchant_id IN (` + placeholders + `)`
			countQuery += ` AND f.merchant_id IN (` + placeholders + `)`
			args = append(args, mArgs...)
		}
		if merchantUUID := c.Query("merchantUuid"); merchantUUID != "" {
			query += ` AND f.merchant_id = (SELECT id FROM merchant WHERE uuid = ?)`
			countQuery += ` AND f.merchant_id = (SELECT id FROM merchant WHERE uuid = ?)`
			args = append(args, merchantUUID)
		}
		if mimePrefix := c.Query("mimePrefix"); mimePrefix != "" {
			query += ` AND f.mime_type LIKE ?`
			countQuery += ` AND f.mime_type LIKE ?`
			args = append(args, mimePrefix+"%")
		}
		if from := c.Query("from"); from != "" {
			query += ` AND f.created_at >= ?`
			countQuery += ` AND f.created_at >= ?`
			args = append(args, from)
		}
		if to := c.Query("to"); to != "" {
			query += ` AND f.created_at <= ?`
			countQuery += ` AND f.created_at <= ?`
			args = append(args, to)
		}
		if search := c.Query("search"); search != "" {
			query += ` AND f.original_name LIKE ?`
			countQuery += ` AND f.original_name LIKE ?`
			args = append(args, "%"+search+"%")
		}

		var total int
		if err := conn.QueryRow(countQuery, args...).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "24"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 200 {
			pageSize = 24
		}
		query += ` ORDER BY f.created_at DESC LIMIT ? OFFSET ?`
		args = append(args, pageSize, (page-1)*pageSize)

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		defer rows.Close()

		out := []fileListOut{}
		for rows.Next() {
			var f fileListOut
			if err := rows.Scan(&f.UUID, &f.OriginalName, &f.MimeType, &f.SizeBytes, &f.Purpose, &f.CreatedAt, &f.MerchantName, &f.UploaderType, &f.UploaderName); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, f)
		}
		c.JSON(http.StatusOK, gin.H{"files": out, "total": total})
	}
}

type renameFileRequest struct {
	OriginalName string `json:"originalName" binding:"required"`
}

// RenameFileHandler — staffOnly, scoped the same way as every other
// merchant-owned resource (an Admin can only touch a file under a
// merchant they hold).
func RenameFileHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		f, err := lookupFile(conn, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if role != "super_admin" {
			var has int
			conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, f.MerchantID).Scan(&has)
			if has == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		var req renameFileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		if _, err := conn.Exec(`UPDATE file SET original_name = ? WHERE uuid = ?`, req.OriginalName, c.Param("uuid")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		audit.Log(conn, audit.Entry{MerchantID: &f.MerchantID, UserID: &userID, Category: "file", Message: "file renamed to " + req.OriginalName, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// DeleteFileHandler — staffOnly, same merchant scoping as RenameFileHandler.
// Removes the DB row and the on-disk blob; best-effort on the disk side so
// a missing/already-gone file doesn't block clearing the record.
func DeleteFileHandler(state *appstate.State, driver storage.Driver) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		f, err := lookupFile(conn, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if role != "super_admin" {
			var has int
			conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, f.MerchantID).Scan(&has)
			if has == 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		if _, err := conn.Exec(`DELETE FROM file WHERE uuid = ?`, c.Param("uuid")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		_ = driver.Delete(f.DiskPath)
		audit.Log(conn, audit.Entry{MerchantID: &f.MerchantID, UserID: &userID, Category: "file", Message: "file deleted: " + f.OrigName, StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// DownloadVisitorFileHandler (visitor): valid only for the visitor who
// owns the chat the file was attached to.
func DownloadVisitorFileHandler(state *appstate.State, driver storage.Driver) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		f, err := lookupFile(conn, c.Param("uuid"))
		if err != nil || !f.ChatID.Valid {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		var visitorUUID string
		if err := conn.QueryRow(
			`SELECT v.uuid FROM chat c JOIN visitor v ON v.id = c.visitor_id WHERE c.id = ?`, f.ChatID.Int64,
		).Scan(&visitorUUID); err != nil || visitorUUID != c.Query("visitor") {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		streamFile(c, driver, f)
	}
}
