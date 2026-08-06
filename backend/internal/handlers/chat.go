package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/audit"
	"livechat/backend/internal/htmlguard"
	"livechat/backend/internal/settings"
	"livechat/backend/internal/storage"
	"livechat/backend/internal/webhook"
	"livechat/backend/internal/ws"
)

type chatOut struct {
	UUID          string     `json:"uuid"`
	VisitorName   string     `json:"visitor_name"`
	VisitorUUID   string     `json:"visitor_uuid"`
	VisitorTier   string     `json:"visitor_tier"`
	MerchantName  string     `json:"merchant_name"`
	MerchantUUID  string     `json:"merchant_uuid"`
	AgentName     *string    `json:"agent_name"`
	AgentEmail    *string    `json:"agent_email"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	LastMessageAt *time.Time `json:"last_message_at"`
	// LastMessageSenderType lets the frontend tell "a customer just wrote
	// in" apart from "an agent just replied" on the Chats list without a
	// second round trip — the sound-alert feature (Telegram request)
	// needs exactly that distinction to avoid dinging on an agent's own
	// outgoing message.
	LastMessageSenderType *string `json:"last_message_sender_type"`
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

		query := `SELECT c.uuid, v.display_name, v.uuid, v.tier, m.name, m.uuid,
		                 u.display_name, u.email, c.status, c.started_at, c.last_message_at,
		                 (SELECT lm.sender_type FROM message lm WHERE lm.chat_id = c.id ORDER BY lm.created_at DESC LIMIT 1)
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
			if err := rows.Scan(&ch.UUID, &ch.VisitorName, &ch.VisitorUUID, &ch.VisitorTier, &ch.MerchantName, &ch.MerchantUUID,
				&ch.AgentName, &ch.AgentEmail, &ch.Status, &ch.StartedAt, &ch.LastMessageAt, &ch.LastMessageSenderType); err != nil {
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
			`SELECT c.uuid, v.display_name, v.uuid, v.tier, m.name, m.uuid, u.display_name, u.email, c.status, c.started_at, c.last_message_at,
			        (SELECT lm.sender_type FROM message lm WHERE lm.chat_id = c.id ORDER BY lm.created_at DESC LIMIT 1)
			 FROM chat c
			 JOIN visitor v ON v.id = c.visitor_id
			 JOIN merchant m ON m.id = c.merchant_id
			 LEFT JOIN user u ON u.id = c.agent_id
			 WHERE c.id = ?`,
			ref.ID,
		).Scan(&summary.UUID, &summary.VisitorName, &summary.VisitorUUID, &summary.VisitorTier, &summary.MerchantName, &summary.MerchantUUID,
			&summary.AgentName, &summary.AgentEmail, &summary.Status, &summary.StartedAt, &summary.LastMessageAt, &summary.LastMessageSenderType); err != nil {
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

// InviteChatHandler: an operator promotes an 'enquiry' (nobody was
// reachable when it landed — overview.md item 4) into a live chat,
// assigning themselves as PIC and nudging the visitor's widget out of its
// waiting screen. Mirrors ClaimChatHandler's shape but only applies to
// the enquiry status, and additionally has to notify the visitor side
// directly since — unlike a claim — the visitor's widget has no assigned
// agent subject to already be listening for a "message" push.
func InviteChatHandler(state *appstate.State, hub *ws.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn := state.DB()
		role := c.MustGet("role").(string)
		userID := c.MustGet("user_id").(int64)

		ref, err := chatAccess(conn, role, userID, c.Param("uuid"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if ref.Status != "enquiry" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "not_an_enquiry"})
			return
		}

		if _, err := conn.Exec(`UPDATE chat SET agent_id = ?, status = 'active' WHERE id = ?`, userID, ref.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		audit.Log(conn, audit.Entry{MerchantID: &ref.MerchantID, UserID: &userID, Category: "chat", Message: "enquiry invited to live chat", StatusCode: 200, Source: "web", IPAddress: c.ClientIP()})
		hub.Publish(ws.VisitorSubject(ref.VisitorID), ws.Event{Type: "chat_invited", Data: gin.H{"chatUuid": c.Param("uuid")}})
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
		webhook.Dispatch(conn, ref.MerchantID, "chat.closed", gin.H{"chatUuid": c.Param("uuid")})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

type sendMessageRequest struct {
	Body   string `json:"body" binding:"required"`
	IsHtml bool   `json:"isHtml"`
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
		if req.IsHtml && htmlguard.IsDangerous(req.Body) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "html_not_allowed"})
			return
		}

		var metadata *string
		if req.IsHtml {
			raw, _ := json.Marshal(map[string]bool{"isHtml": true})
			s := string(raw)
			metadata = &s
		}

		msgID, createdAt, err := insertMessage(conn, ref.ID, "agent", &userID, req.Body, "text", metadata)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		out := messageOut{ID: msgID, ChatUUID: c.Param("uuid"), SenderType: "agent", Body: req.Body, Type: "text", CreatedAt: createdAt, Metadata: metadata}
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
		`SELECT c.uuid, v.display_name, v.uuid, m.name, m.uuid, u.display_name, u.email, c.status, c.started_at, c.last_message_at,
		        (SELECT lm.sender_type FROM message lm WHERE lm.chat_id = c.id ORDER BY lm.created_at DESC LIMIT 1)
		 FROM chat c
		 JOIN visitor v ON v.id = c.visitor_id
		 JOIN merchant m ON m.id = c.merchant_id
		 LEFT JOIN user u ON u.id = c.agent_id
		 WHERE c.uuid = ?`,
		chatUUID,
	).Scan(&out.UUID, &out.VisitorName, &out.VisitorUUID, &out.MerchantName, &out.MerchantUUID,
		&out.AgentName, &out.AgentEmail, &out.Status, &out.StartedAt, &out.LastMessageAt, &out.LastMessageSenderType)
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
			var ruleErr *fileRuleError
			if errors.As(err, &ruleErr) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "file_rejected", "detail": ruleErr.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload_failed", "detail": err.Error()})
			return
		}
		out.ChatUUID = c.Param("uuid")

		hub.Publish(ws.VisitorSubject(ref.VisitorID), ws.Event{Type: "message", Data: out})
		notifyChatUpdated(conn, hub, ref.MerchantID, c.Param("uuid"))
		c.JSON(http.StatusOK, out)
	}
}

// fileRuleError marks an upload rejected by the Files settings tab's
// format/size rules (overview.md §6.8) — handlers use errors.As to
// return 400 instead of the generic 500 upload_failed.
type fileRuleError struct{ msg string }

func (e *fileRuleError) Error() string { return e.msg }

// validateFileRules checks the merchant's own override first (Super
// Admin can exempt or tighten a specific merchant — overview.md §6.8),
// falling back to the global default when no override is set.
func validateFileRules(conn *sql.DB, merchantID int64, fh *multipart.FileHeader) error {
	maxMBStr, _, err := settings.GetMerchant(conn, merchantID, "file_max_size_mb")
	if err != nil {
		maxMBStr = settings.Defaults["file_max_size_mb"]
	}
	if maxMB, _ := strconv.Atoi(maxMBStr); maxMB > 0 && fh.Size > int64(maxMB)*1024*1024 {
		return &fileRuleError{fmt.Sprintf("file exceeds the %d MB limit", maxMB)}
	}

	allowedStr, _, err := settings.GetMerchant(conn, merchantID, "file_allowed_extensions")
	if err != nil {
		allowedStr = settings.Defaults["file_allowed_extensions"]
	}
	if allowed := strings.TrimSpace(allowedStr); allowed != "" {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(fh.Filename), "."))
		ok := false
		for _, a := range strings.Split(allowed, ",") {
			if strings.ToLower(strings.TrimSpace(a)) == ext {
				ok = true
				break
			}
		}
		if !ok {
			return &fileRuleError{fmt.Sprintf("file type .%s is not allowed", ext)}
		}
	}
	return nil
}

// storeChatFile is shared by the staff and visitor upload handlers: save
// to disk, insert the `file` row, then insert a `type=file` message
// pointing at it — metadata carries what the UI needs to render an
// attachment link without a second round trip.
func storeChatFile(conn *sql.DB, driver storage.Driver, ref *chatRef, uploaderType string, uploaderID *int64, fh *multipart.FileHeader) (messageOut, error) {
	if err := validateFileRules(conn, ref.MerchantID, fh); err != nil {
		return messageOut{}, err
	}

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
