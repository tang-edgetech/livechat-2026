package handlers

import (
	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/storage"
	"livechat/backend/internal/ws"
)

type chatOut struct {
	UUID          string     `json:"uuid"`
	VisitorName   string     `json:"visitor_name"`
	VisitorUUID   string     `json:"visitor_uuid"`
	MerchantName  string     `json:"merchant_name"`
	MerchantUUID  string     `json:"merchant_uuid"`
	AgentName     *string    `json:"agent_name"`
	AgentEmail    *string    `json:"agent_email"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	LastMessageAt *time.Time `json:"last_message_at"`
}

type messageOut struct {
	UUID       string    `json:"uuid,omitempty"`
	ID         int64     `json:"id"`
	ChatUUID   string    `json:"chat_uuid,omitempty"`
	SenderType string    `json:"sender_type"`
	Body       string    `json:"body"`
	Type       string    `json:"type"`
	Metadata   *string   `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// scopedMerchantIDs mirrors the §3 scoping rule used everywhere else:
// Super Admin sees every merchant, Admin/Agent see only merchants they
// hold via user_merchant.
func scopedMerchantIDs(conn *sql.DB, role string, userID int64) ([]int64, error) {
	var rows *sql.Rows
	var err error
	if role == "super_admin" {
		rows, err = conn.Query(`SELECT id FROM merchant`)
	} else {
		rows, err = conn.Query(`SELECT merchant_id FROM user_merchant WHERE user_id = ?`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func ListChatsHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		merchantIDs, err := scopedMerchantIDs(conn, role, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if merchantUUID := c.Query("merchantUuid"); merchantUUID != "" {
			var id int64
			if err := conn.QueryRow(`SELECT id FROM merchant WHERE uuid = ?`, merchantUUID).Scan(&id); err == nil {
				merchantIDs = intersectInt64(merchantIDs, id)
			} else {
				merchantIDs = nil
			}
		}
		if len(merchantIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"chats": []chatOut{}, "total": 0})
			return
		}

		placeholders, args := int64SliceToPlaceholders(merchantIDs)

		query := `SELECT c.uuid, v.display_name, v.uuid, m.name, m.uuid,
		                 u.display_name, u.email, c.status, c.started_at, c.last_message_at
		          FROM chat c
		          JOIN visitor v ON v.id = c.visitor_id
		          JOIN merchant m ON m.id = c.merchant_id
		          LEFT JOIN user u ON u.id = c.agent_id
		          WHERE c.merchant_id IN (` + placeholders + `)`
		countQuery := `SELECT COUNT(*) FROM chat c WHERE c.merchant_id IN (` + placeholders + `)`

		if status := c.Query("status"); status != "" {
			query += " AND c.status = ?"
			countQuery += " AND c.status = ?"
			args = append(args, status)
		}
		if search := c.Query("search"); search != "" {
			query += " AND v.display_name LIKE ?"
			countQuery += " AND v.display_name LIKE ?"
			args = append(args, "%"+search+"%")
		}

		var total int
		if err := conn.QueryRow(countQuery, args...).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		sortCol := map[string]string{"started_at": "c.started_at", "last_message_at": "c.last_message_at"}[c.DefaultQuery("sortBy", "last_message_at")]
		if sortCol == "" {
			sortCol = "c.last_message_at"
		}
		sortDir := "DESC"
		if c.Query("sortDir") == "asc" {
			sortDir = "ASC"
		}
		query += " ORDER BY " + sortCol + " " + sortDir

		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 200 {
			pageSize = 20
		}
		query += " LIMIT ? OFFSET ?"
		args = append(args, pageSize, (page-1)*pageSize)

		rows, err := conn.Query(query, args...)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
			return
		}
		defer rows.Close()

		out := []chatOut{}
		for rows.Next() {
			var ch chatOut
			if err := rows.Scan(&ch.UUID, &ch.VisitorName, &ch.VisitorUUID, &ch.MerchantName, &ch.MerchantUUID,
				&ch.AgentName, &ch.AgentEmail, &ch.Status, &ch.StartedAt, &ch.LastMessageAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			out = append(out, ch)
		}
		c.JSON(http.StatusOK, gin.H{"chats": out, "total": total})
	}
}

// intersectInt64 narrows a scoped id list down to a single requested id,
// returning nil if that id wasn't in scope — used so a merchant filter
// can never leak a chat outside the requester's own merchant access.
func intersectInt64(ids []int64, want int64) []int64 {
	for _, id := range ids {
		if id == want {
			return []int64{id}
		}
	}
	return nil
}

func int64SliceToPlaceholders(ids []int64) (string, []any) {
	placeholders := ""
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = id
	}
	return placeholders, args
}

// chatAccess resolves a chat uuid to its internal id/merchant/agent/visitor
// ids and checks the requester's merchant is in scope — every chat
// endpoint below funnels through this so the scoping rule lives in one
// place (mirrors users.go's resolveTarget pattern).
type chatRef struct {
	ID         int64
	MerchantID int64
	VisitorID  int64
	AgentID    sql.NullInt64
	Status     string
}

func chatAccess(conn *sql.DB, role string, userID int64, chatUUID string) (*chatRef, error) {
	var ref chatRef
	err := conn.QueryRow(
		`SELECT id, merchant_id, visitor_id, agent_id, status FROM chat WHERE uuid = ?`, chatUUID,
	).Scan(&ref.ID, &ref.MerchantID, &ref.VisitorID, &ref.AgentID, &ref.Status)
	if err != nil {
		return nil, err
	}

	if role == "super_admin" {
		return &ref, nil
	}
	var has int
	err = conn.QueryRow(
		`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, userID, ref.MerchantID,
	).Scan(&has)
	if err != nil {
		return nil, err
	}
	if has == 0 {
		return nil, sql.ErrNoRows
	}
	return &ref, nil
}

func GetChatHandler(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		var summary chatOut
		if err := conn.QueryRow(
			`SELECT c.uuid, v.display_name, v.uuid, m.name, m.uuid, u.display_name, u.email, c.status, c.started_at, c.last_message_at
			 FROM chat c
			 JOIN visitor v ON v.id = c.visitor_id
			 JOIN merchant m ON m.id = c.merchant_id
			 LEFT JOIN user u ON u.id = c.agent_id
			 WHERE c.id = ?`,
			ref.ID,
		).Scan(&summary.UUID, &summary.VisitorName, &summary.VisitorUUID, &summary.MerchantName, &summary.MerchantUUID,
			&summary.AgentName, &summary.AgentEmail, &summary.Status, &summary.StartedAt, &summary.LastMessageAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		rows, err := conn.Query(
			`SELECT id, sender_type, body, type, metadata, created_at FROM message WHERE chat_id = ? ORDER BY created_at ASC`,
			ref.ID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		defer rows.Close()

		messages := []messageOut{}
		for rows.Next() {
			var m messageOut
			if err := rows.Scan(&m.ID, &m.SenderType, &m.Body, &m.Type, &m.Metadata, &m.CreatedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			messages = append(messages, m)
		}

		c.JSON(http.StatusOK, gin.H{
			"chat":     summary,
			"messages": messages,
		})
	}
}

// ClaimChatHandler: an Agent self-assigns a pending chat (overview.md
// §6.9 manual mode). Admin/Super Admin can claim too (acts as the manual
// reassignment overview.md always allows).
func ClaimChatHandler(state *appstate.State, hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status == "closed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chat_closed"})
			return
		}

		if _, err := conn.Exec(`UPDATE chat SET agent_id = ?, status = 'active' WHERE id = ?`, userID, ref.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{MerchantID: &ref.MerchantID, UserID: &userID, Category: "chat", Message: "chat claimed", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type assignChatRequest struct {
	AgentUUID string `json:"agentUuid" binding:"required"`
}

// AssignChatHandler: Admin/Super Admin manually (re)assign a chat's PIC —
// overview.md §6.9: "an Admin (or Super Admin) can always manually
// reassign a chat's PIC — routing decides the default assignment, not a
// hard restriction."
func AssignChatHandler(state *appstate.State, hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var req assignChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		var agentID int64
		var agentRole string
		if err := conn.QueryRow(
			`SELECT u.id, r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.uuid = ?`, req.AgentUUID,
		).Scan(&agentID, &agentRole); err != nil || agentRole != "agent" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_agent"})
			return
		}
		var inMerchant int
		conn.QueryRow(`SELECT COUNT(*) FROM user_merchant WHERE user_id = ? AND merchant_id = ?`, agentID, ref.MerchantID).Scan(&inMerchant)
		if inMerchant == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent_not_in_merchant"})
			return
		}

		if _, err := conn.Exec(`UPDATE chat SET agent_id = ?, status = 'active' WHERE id = ?`, agentID, ref.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{MerchantID: &ref.MerchantID, UserID: &userID, Category: "chat", Message: "chat reassigned", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func CloseChatHandler(state *appstate.State, hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}

		if _, err := conn.Exec(`UPDATE chat SET status = 'closed', closed_at = NOW() WHERE id = ?`, ref.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{MerchantID: &ref.MerchantID, UserID: &userID, Category: "chat", Message: "chat closed", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		hub.Publish(ws.VisitorSubject(ref.VisitorID), ws.Event{Type: "chat_closed", Data: gin.H{"chatUuid": c.Param("uuid")}})
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type sendMessageRequest struct {
	Body string `json:"body" binding:"required"`
}

// SendMessageHandler: the Agent side of "AJAX POST is broadcast to the
// other party over WebSocket" (overview.md §2). Requires the chat be
// active and — if the actor is a plain Agent, not Admin/Super Admin —
// that they're the assigned PIC, so replying still routes through claim.
func SendMessageHandler(state *appstate.State, hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		var req sendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status == "closed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chat_closed"})
			return
		}
		if role == "agent" && (!ref.AgentID.Valid || ref.AgentID.Int64 != userID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "not_the_assigned_agent"})
			return
		}

		msgID, createdAt, err := insertMessage(conn, ref.ID, "agent", &userID, req.Body, "text", nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		out := messageOut{ID: msgID, ChatUUID: c.Param("uuid"), SenderType: "agent", Body: req.Body, Type: "text", CreatedAt: createdAt}
		hub.Publish(ws.VisitorSubject(ref.VisitorID), ws.Event{Type: "message", Data: out})
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))

		c.JSON(http.StatusOK, out)
	}
}

func insertMessage(conn *sql.DB, chatID int64, senderType string, senderID *int64, body, msgType string, metadata *string) (int64, time.Time, error) {
	now := time.Now()
	result, err := conn.Exec(
		`INSERT INTO message (chat_id, sender_type, sender_id, body, type, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		chatID, senderType, senderID, body, msgType, metadata, now,
	)
	if err != nil {
		return 0, time.Time{}, err
	}
	id, _ := result.LastInsertId()
	_, _ = conn.Exec(`UPDATE chat SET last_message_at = ? WHERE id = ?`, now, chatID)
	return id, now, nil
}

// notifyChatUpdated pushes a fresh chat summary to every online staff
// member watching this merchant's dashboard/Chat List — see
// ws.DashboardSubject. Best-effort: a failed re-fetch here just means a
// slightly stale row until the next event, never a fatal error for the
// request that triggered it.
func notifyChatUpdated(conn *sql.DB, hub *ws.Hub, merchantID int64, chatUUID string) {
	var out chatOut
	err := conn.QueryRow(
		`SELECT c.uuid, v.display_name, v.uuid, m.name, m.uuid, u.display_name, u.email, c.status, c.started_at, c.last_message_at
		 FROM chat c
		 JOIN visitor v ON v.id = c.visitor_id
		 JOIN merchant m ON m.id = c.merchant_id
		 LEFT JOIN user u ON u.id = c.agent_id
		 WHERE c.uuid = ?`,
		chatUUID,
	).Scan(&out.UUID, &out.VisitorName, &out.VisitorUUID, &out.MerchantName, &out.MerchantUUID,
		&out.AgentName, &out.AgentEmail, &out.Status, &out.StartedAt, &out.LastMessageAt)
	if err != nil {
		return
	}
	hub.Publish(ws.DashboardSubject(merchantID), ws.Event{Type: "chat_updated", Data: out})
}

// UploadFileHandler handles an Agent's file attachment on a chat
// (overview.md §6.8 category 1). Stored via the storage.Driver interface
// so swapping local disk for S3 later is a config change only (§7).
func UploadFileHandler(state *appstate.State, hub *ws.Hub, driver storage.Driver) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status == "closed" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chat_closed"})
			return
		}

		fh, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no_file"})
			return
		}

		out, err := storeChatFile(conn, driver, ref, "user", &userID, fh)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload_failed", "detail": err.Error()})
			return
		}
		out.ChatUUID = c.Param("uuid")

		hub.Publish(ws.VisitorSubject(ref.VisitorID), ws.Event{Type: "message", Data: out})
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		c.JSON(http.StatusOK, out)
	}
}

// storeChatFile is shared by the staff and visitor upload handlers: save
// to disk, insert the `file` row, then insert a `type=file` message
// pointing at it — metadata carries what the UI needs to render an
// attachment link without a second round trip.
func storeChatFile(conn *sql.DB, driver storage.Driver, ref *chatRef, uploaderType string, uploaderID *int64, fh *multipart.FileHeader) (messageOut, error) {
	src, err := fh.Open()
	if err != nil {
		return messageOut{}, err
	}
	defer src.Close()

	fileUUID := uuid.New().String()
	diskPath, err := driver.Put(fileUUID, src)
	if err != nil {
		return messageOut{}, err
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if _, err := conn.Exec(
		`INSERT INTO file (uuid, merchant_id, chat_id, uploader_type, uploader_id, purpose, disk_path, original_name, mime_type, size_bytes)
		 VALUES (?, ?, ?, ?, ?, 'chat', ?, ?, ?, ?)`,
		fileUUID, ref.MerchantID, ref.ID, uploaderType, uploaderID, diskPath, fh.Filename, mimeType, fh.Size,
	); err != nil {
		return messageOut{}, err
	}

	metadata := fmt.Sprintf(`{"fileUuid":%q,"originalName":%q,"mimeType":%q,"sizeBytes":%d}`,
		fileUUID, fh.Filename, mimeType, fh.Size)
	senderType := "agent"
	if uploaderType == "visitor" {
		senderType = "visitor"
	}
	msgID, createdAt, err := insertMessage(conn, ref.ID, senderType, uploaderID, fh.Filename, "file", &metadata)
	if err != nil {
		return messageOut{}, err
	}

	return messageOut{ID: msgID, SenderType: senderType, Body: fh.Filename, Type: "file", Metadata: &metadata, CreatedAt: createdAt}, nil
}
