// Package passthrough verifies the signed payload a merchant's own
// backend produces for an already-logged-in website visitor (overview.md
// §10.2). "Never trust an unsigned passthrough payload — it's
// attacker-controlled JS on someone else's page": every field here is
// only used once HMAC verification against the merchant's own
// widget_identity secret succeeds.
package passthrough

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type Identity struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	ExternalID string `json:"externalId"`
	ExpiresAt  int64  `json:"expiresAt"`
	// Tier is the "standard" a merchant's own site follows to flag a
	// VIP customer (overview.md §6.9.1) — trusted only because it rides
	// inside this same HMAC-signed payload. Per the product decision:
	// the parked site sends "vip" for a VIP account and omits the field
	// entirely for a normal one; anything else is treated as no signal.
	Tier string `json:"tier,omitempty"`
}

var ErrInvalid = errors.New("invalid or expired passthrough token")

// Verify checks a "<base64url-payload>.<hex-hmac>" token against the
// merchant's widget_identity secret and expiry. Returns the embedded
// identity only on success.
func Verify(conn *sql.DB, merchantID int64, token string) (*Identity, error) {
	dotIdx := lastIndexByte(token, '.')
	if dotIdx < 0 {
		return nil, ErrInvalid
	}
	payloadB64, sigHex := token[:dotIdx], token[dotIdx+1:]

	var secret string
	err := conn.QueryRow(
		`SELECT i.secret_hash FROM integration i
		 JOIN integration_merchant im ON im.integration_id = i.id
		 WHERE im.merchant_id = ? AND i.type = 'widget_identity'
		 ORDER BY i.created_at DESC LIMIT 1`,
		merchantID,
	).Scan(&secret)
	if err != nil {
		return nil, ErrInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, ErrInvalid
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, ErrInvalid
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadBytes)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, sigBytes) {
		return nil, ErrInvalid
	}

	var identity Identity
	if err := json.Unmarshal(payloadBytes, &identity); err != nil {
		return nil, ErrInvalid
	}
	if identity.ExpiresAt == 0 || time.Now().Unix() > identity.ExpiresAt {
		return nil, ErrInvalid
	}

	return &identity, nil
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
