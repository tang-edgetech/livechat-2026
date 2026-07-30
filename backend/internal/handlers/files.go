package handlers

import (
	"database/sql"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
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
