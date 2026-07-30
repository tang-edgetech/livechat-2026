package session

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// IdleTimeout is the 2-hour idle-logout window from overview.md §6.0.
const IdleTimeout = 2 * time.Hour

// bumpThrottle avoids write amplification on last_activity_at (§4 `session`
// note: "bumped ... throttled, e.g. at most once/minute").
const bumpThrottle = 1 * time.Minute

var ErrNotFound = errors.New("session not found or expired")

type Session struct {
	ID             int64
	UserID         int64
	LastActivityAt time.Time
	ExpiresAt      time.Time
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create issues a new session for userID and returns the raw token — the
// only time the raw value exists; only its hash is ever persisted.
func Create(conn *sql.DB, userID int64, ip, userAgent string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	_, err = conn.Exec(
		`INSERT INTO session (user_id, token_hash, ip, user_agent, last_activity_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, hashToken(token), ip, userAgent, now, now.Add(IdleTimeout),
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

// Validate looks up the session by raw token. If it has gone past its
// idle window it is treated as gone (ErrNotFound) — the client-side timer
// is cosmetic, this check is what actually enforces the 2-hour idle logout.
// On success it slides the expiry forward, throttled to at most once/minute.
func Validate(conn *sql.DB, token string) (*Session, error) {
	var s Session
	row := conn.QueryRow(
		`SELECT id, user_id, last_activity_at, expires_at FROM session WHERE token_hash = ?`,
		hashToken(token),
	)
	if err := row.Scan(&s.ID, &s.UserID, &s.LastActivityAt, &s.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	now := time.Now()
	if now.After(s.ExpiresAt) {
		_, _ = conn.Exec(`DELETE FROM session WHERE id = ?`, s.ID)
		return nil, ErrNotFound
	}

	if now.Sub(s.LastActivityAt) >= bumpThrottle {
		newExpiry := now.Add(IdleTimeout)
		_, err := conn.Exec(
			`UPDATE session SET last_activity_at = ?, expires_at = ? WHERE id = ?`,
			now, newExpiry, s.ID,
		)
		if err == nil {
			s.LastActivityAt = now
			s.ExpiresAt = newExpiry
		}
	}

	return &s, nil
}

// Delete removes a session — logout.
func Delete(conn *sql.DB, token string) error {
	_, err := conn.Exec(`DELETE FROM session WHERE token_hash = ?`, hashToken(token))
	return err
}
