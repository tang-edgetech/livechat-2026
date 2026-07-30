// Package ws is the real-time push layer described in overview.md §2:
// AJAX handles every action (send/save/delete/...); this hub only pushes
// new messages/status changes to the other party's already-open
// connection. Built on a Redis pub/sub abstraction from day one — even
// running a single Go instance today — so scaling to multiple instances
// later (§2's "millions of users" target) is a config change, not a
// rewrite: every instance subscribes to the same Redis channels, so any
// instance can deliver to a connection held by any other instance.
package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const channelPrefix = "ws:"

// Event is the envelope pushed to clients over the socket.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Hub struct {
	mu       sync.RWMutex
	subjects map[string]map[*Conn]bool

	redis *redis.Client // nil if Redis isn't reachable — falls back to local-only delivery
	ctx   context.Context
}

// NewHub subscribes to every "ws:*" channel so cross-instance delivery
// and same-instance delivery share one code path (see Publish).
func NewHub(ctx context.Context, redisClient *redis.Client) *Hub {
	h := &Hub{
		subjects: make(map[string]map[*Conn]bool),
		redis:    redisClient,
		ctx:      ctx,
	}
	if redisClient != nil {
		go h.subscribeLoop(redisClient)
	}
	return h
}

func (h *Hub) subscribeLoop(client *redis.Client) {
	sub := client.PSubscribe(h.ctx, channelPrefix+"*")
	defer sub.Close()

	for msg := range sub.Channel() {
		subject := msg.Channel[len(channelPrefix):]
		h.deliverLocal(subject, []byte(msg.Payload))
	}
}

func (h *Hub) Register(subject string, conn *Conn) {
	h.mu.Lock()
	if h.subjects[subject] == nil {
		h.subjects[subject] = make(map[*Conn]bool)
	}
	h.subjects[subject][conn] = true
	h.mu.Unlock()
}

func (h *Hub) Unregister(subject string, conn *Conn) {
	h.mu.Lock()
	if conns, ok := h.subjects[subject]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.subjects, subject)
		}
	}
	h.mu.Unlock()
}

// IsOnline reports whether `subject` has at least one live connection on
// THIS instance. Cross-instance presence goes through the `presence`
// package (backed by Redis), not this local map.
func (h *Hub) IsOnline(subject string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subjects[subject]) > 0
}

func (h *Hub) deliverLocal(subject string, payload []byte) {
	h.mu.RLock()
	conns := make([]*Conn, 0, len(h.subjects[subject]))
	for c := range h.subjects[subject] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.Send(payload)
	}
}

// Publish delivers an event to every connection registered under
// `subject`, on any instance. When Redis is reachable, this instance
// never delivers directly — it publishes, and its own subscribeLoop
// echoes the message back for local delivery, so there is exactly one
// delivery path regardless of which instance published. If Redis is
// down, it falls back to local-only delivery rather than dropping the
// event silently.
func (h *Hub) Publish(subject string, event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("ws: marshal event failed: %v", err)
		return
	}

	if h.redis != nil {
		ctx, cancel := context.WithTimeout(h.ctx, 2*time.Second)
		defer cancel()
		if err := h.redis.Publish(ctx, channelPrefix+subject, payload).Err(); err == nil {
			return
		}
		log.Printf("ws: redis publish failed, falling back to local delivery")
	}
	h.deliverLocal(subject, payload)
}

// Conn wraps one client's websocket with a buffered send channel so the
// hub never blocks (or panics on a concurrent write) while pushing to a
// slow or half-closed client.
type Conn struct {
	ws   *websocket.Conn
	send chan []byte
}

func NewConn(ws *websocket.Conn) *Conn {
	return &Conn{ws: ws, send: make(chan []byte, 16)}
}

func (c *Conn) Send(payload []byte) {
	select {
	case c.send <- payload:
	default:
		log.Printf("ws: client send buffer full, dropping message")
	}
}

// WritePump owns all writes to the underlying socket — the only
// goroutine allowed to call ws.Write* — and exits (closing the socket)
// when `send` is closed by ReadPump on disconnect.
func (c *Conn) WritePump() {
	defer c.ws.Close()
	for payload := range c.send {
		if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

// ReadPump's only job is noticing disconnection — overview.md §2 says the
// socket is push-only from the server, clients never send chat data over
// it (that's all AJAX). Reading is still required so gorilla answers
// ping/pong control frames and so we detect a closed connection promptly.
func (c *Conn) ReadPump(onClose func()) {
	defer func() {
		close(c.send)
		onClose()
	}()
	for {
		if _, _, err := c.ws.ReadMessage(); err != nil {
			return
		}
	}
}
