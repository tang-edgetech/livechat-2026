// Package webhook is the outbound notification half of overview.md §6.5
// ("REST API — inbound and outbound (webhooks out)") — distinct from a
// Bot flow's synchronous call_integration (which posts and waits for a
// reply inline). This is fire-and-forget: platform events (a chat
// starting, a visitor message arriving, a chat closing) get POSTed to any
// `webhook`-type integration configured to receive that event, best
// effort, same fail-soft discipline as audit.Log — a slow or broken
// receiving endpoint must never block or fail the request that triggered
// the event.
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Config struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// Dispatch looks up every webhook integration in scope for merchantID
// (its own merchant, or global) that lists `event` in its config, and
// POSTs an HMAC-signed JSON payload to each in its own goroutine.
func Dispatch(conn *sql.DB, merchantID int64, event string, payload any) {
	rows, err := conn.Query(
		`SELECT DISTINCT i.id, i.config, i.secret_hash FROM integration i
		 LEFT JOIN integration_merchant im ON im.integration_id = i.id
		 WHERE i.type = 'webhook' AND (i.is_global = TRUE OR im.merchant_id = ?)`,
		merchantID,
	)
	if err != nil {
		log.Printf("webhook dispatch lookup failed: %v", err)
		return
	}
	defer rows.Close()

	type target struct {
		url    string
		secret string
	}
	var targets []target
	for rows.Next() {
		var id int64
		var configRaw, secret string
		if err := rows.Scan(&id, &configRaw, &secret); err != nil {
			continue
		}
		var cfg Config
		json.Unmarshal([]byte(configRaw), &cfg)
		if cfg.URL == "" || !contains(cfg.Events, event) {
			continue
		}
		targets = append(targets, target{url: cfg.URL, secret: secret})
	}

	if len(targets) == 0 {
		return
	}

	body, err := json.Marshal(map[string]any{"event": event, "data": payload})
	if err != nil {
		return
	}

	for _, t := range targets {
		go post(t.url, t.secret, body)
	}
}

func post(url, secret string, body []byte) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook request build failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-LiveChat-Signature", signature)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook delivery failed (%s): %v", url, err)
		return
	}
	defer resp.Body.Close()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
