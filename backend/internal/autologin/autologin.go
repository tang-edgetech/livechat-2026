// Package autologin verifies the signed deep-link token a B2B partner's
// own backend produces to drop one of their already-authenticated staff
// straight into the panel (overview.md §6.5/§9: "B2B auto-login — dev
// team issues a merchant a unique hash key; their system can deep-link a
// user into the panel pre-authenticated"). Deliberately mirrors
// internal/passthrough's shape exactly (same
// "<base64url-payload>.<hex-hmac>" token, same never-trust-until-verified
// posture) — the two are the same mechanism applied to two different
// privilege levels, which is exactly why they use separate `integration`
// secrets (`auto_login` here vs `widget_identity` there): a leaked
// visitor-passthrough secret should never be enough to sign into the
// staff panel.
package autologin

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Identity struct {
	UserUUID  string `json:"userUuid"`
	ExpiresAt int64  `json:"expiresAt"`
}

var ErrInvalid = errors.New("invalid or expired auto-login token")

// Verify checks a "<base64url-payload>.<hex-hmac>" token against the
// merchant's auto_login secret and expiry.
func Verify(conn *sql.DB, merchantID int64, token string) (*Identity, error) {
	dotIdx := strings.LastIndexByte(token, '.')
	if dotIdx < 0 {
		return nil, ErrInvalid
	}
	payloadB64, sigHex := token[:dotIdx], token[dotIdx+1:]

	var secret string
	err := conn.QueryRow(
		`SELECT i.secret_hash FROM integration i
		 JOIN integration_merchant im ON im.integration_id = i.id
		 WHERE im.merchant_id = ? AND i.type = 'auto_login'
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
