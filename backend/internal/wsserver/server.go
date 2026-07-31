// Package wsserver hosts the WebSocket upgrade endpoints on their own
// port (cfg.WSPort) — separate from the main REST API port, matching the
// port fields the Setup Wizard already collects (overview.md §5). Two
// entry points: /ws for authenticated staff (Agent/Admin/Super Admin),
// /ws/visitor for the widget side of a chat.
package wsserver

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/config"
	"livechat/backend/internal/presence"
	"livechat/backend/internal/session"
	"livechat/backend/internal/ws"
)

func Start(cfg *config.Config, state *appstate.State, hub *ws.Hub, redisClient *redis.Client) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" || origin == cfg.FrontendOrigin
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn := state.DB()
		if conn == nil {
			http.Error(w, "not configured", http.StatusServiceUnavailable)
			return
		}

		cookie, err := r.Cookie("session_token")
		if err != nil {
			http.Error(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		sess, err := session.Validate(conn, cookie.Value)
		if err != nil {
			http.Error(w, "session expired", http.StatusUnauthorized)
			return
		}

		merchantIDs, err := staffMerchantIDs(conn, sess.UserID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws: staff upgrade failed: %v", err)
			return
		}

		serveStaff(r.Context(), hub, redisClient, sess.UserID, merchantIDs, wsConn)
	})

	mux.HandleFunc("/ws/visitor", func(w http.ResponseWriter, r *http.Request) {
		conn := state.DB()
		if conn == nil {
			http.Error(w, "not configured", http.StatusServiceUnavailable)
			return
		}

		visitorUUID := r.URL.Query().Get("visitor")
		chatUUID := r.URL.Query().Get("chat")
		visitorID, err := validateVisitorChat(conn, visitorUUID, chatUUID)
		if err != nil {
			http.Error(w, "invalid visitor/chat", http.StatusUnauthorized)
			return
		}

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws: visitor upgrade failed: %v", err)
			return
		}

		subject := ws.VisitorSubject(visitorID)
		serveConn(hub, subject, wsConn)
	})

	go func() {
		log.Println("WebSocket server listening on :" + cfg.WSPort)
		if err := http.ListenAndServe(":"+cfg.WSPort, mux); err != nil {
			log.Fatal("wsserver: ", err)
		}
	}()
}

// serveStaff registers the connection under the Agent's own subject plus
// every merchant dashboard subject they can see, and tracks presence
// (overview.md §6.9 round-robin routing needs to know who's online).
func serveStaff(ctx context.Context, hub *ws.Hub, redisClient *redis.Client, userID int64, merchantIDs []int64, wsConn *websocket.Conn) {
	conn := ws.NewConn(wsConn)
	subjects := []string{ws.AgentSubject(userID)}
	for _, mID := range merchantIDs {
		subjects = append(subjects, ws.DashboardSubject(mID))
	}
	for _, s := range subjects {
		hub.Register(s, conn)
	}

	if err := presence.Connect(ctx, redisClient, userID); err != nil {
		log.Printf("ws: presence connect failed: %v", err)
	}
	for _, mID := range merchantIDs {
		hub.Publish(ws.DashboardSubject(mID), ws.Event{Type: "presence_changed"})
	}

	go conn.WritePump()
	conn.ReadPump(func() {
		for _, s := range subjects {
			hub.Unregister(s, conn)
		}
		if err := presence.Disconnect(ctx, redisClient, userID); err != nil {
			log.Printf("ws: presence disconnect failed: %v", err)
		}
		for _, mID := range merchantIDs {
			hub.Publish(ws.DashboardSubject(mID), ws.Event{Type: "presence_changed"})
		}
	})
}

func serveConn(hub *ws.Hub, subject string, wsConn *websocket.Conn) {
	conn := ws.NewConn(wsConn)
	hub.Register(subject, conn)
	go conn.WritePump()
	conn.ReadPump(func() {
		hub.Unregister(subject, conn)
	})
}

func staffMerchantIDs(conn *sql.DB, userID int64) ([]int64, error) {
	var role string
	if err := conn.QueryRow(
		`SELECT r.slug FROM user u JOIN role r ON r.id = u.role_id WHERE u.id = ?`, userID,
	).Scan(&role); err != nil {
		return nil, err
	}

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

func validateVisitorChat(conn *sql.DB, visitorUUID, chatUUID string) (int64, error) {
	var visitorID int64
	err := conn.QueryRow(
		`SELECT v.id FROM visitor v
		 JOIN chat c ON c.visitor_id = v.id
		 WHERE v.uuid = ? AND c.uuid = ?`,
		visitorUUID, chatUUID,
	).Scan(&visitorID)
	return visitorID, err
}
