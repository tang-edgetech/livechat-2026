package visitor

import (
	"database/sql"
	"regexp"

	"github.com/google/uuid"

	"livechat/backend/internal/audit"
)

type Visitor struct {
	ID          int64
	UUID        string
	MerchantID  int64
	DisplayName string
	Phone       sql.NullString
	Email       sql.NullString
}

var nonDigits = regexp.MustCompile(`\D`)

// NormalizePhone strips everything but digits. Full E.164 validation
// belongs to the real pre-chat form (Phase 3); the internal test harness
// (Phase 2) is expected to supply already-sane numbers.
func NormalizePhone(raw string) string {
	return nonDigits.ReplaceAllString(raw, "")
}

// Resolve implements the identity-resolution order from overview.md §4:
//  1. normalize phone, look up a non-merged visitor by (merchant, phone) — phone is the primary key
//  2. if no phone match, fall back to (merchant, email)
//  3. if neither matches, create a new visitor
//  4. if phone matches one row and email matches a *different* row, don't
//     auto-merge — proceed with the phone-matched row and flag the
//     conflict in audit_log for manual review (§10.3's merge tool).
func Resolve(conn *sql.DB, merchantID int64, phoneRaw, email, displayName, fingerprint string) (*Visitor, error) {
	phone := NormalizePhone(phoneRaw)

	var byPhone, byEmail *Visitor
	var err error

	if phone != "" {
		byPhone, err = findActive(conn, merchantID, "phone", phone)
		if err != nil {
			return nil, err
		}
	}
	if email != "" {
		byEmail, err = findActive(conn, merchantID, "email", email)
		if err != nil {
			return nil, err
		}
	}

	if byPhone != nil && byEmail != nil && byPhone.ID != byEmail.ID {
		audit.Log(conn, audit.Entry{
			MerchantID: &merchantID, Category: "visitor_merge",
			Message:    "possible duplicate: phone and email resolved to different visitors — proceeding with the phone match",
			StatusCode: 200, Source: "system",
		})
		return byPhone, nil
	}
	if byPhone != nil {
		return byPhone, nil
	}
	if byEmail != nil {
		return byEmail, nil
	}

	return create(conn, merchantID, phone, email, displayName, fingerprint)
}

func findActive(conn *sql.DB, merchantID int64, column, value string) (*Visitor, error) {
	query := `SELECT id, uuid, merchant_id, display_name, phone, email FROM visitor
	          WHERE merchant_id = ? AND ` + column + ` = ? AND merged_into_id IS NULL
	          ORDER BY created_at ASC LIMIT 1`
	var v Visitor
	err := conn.QueryRow(query, merchantID, value).Scan(&v.ID, &v.UUID, &v.MerchantID, &v.DisplayName, &v.Phone, &v.Email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func create(conn *sql.DB, merchantID int64, phone, email, displayName, fingerprint string) (*Visitor, error) {
	if displayName == "" {
		displayName = "Visitor"
	}
	newUUID := uuid.New().String()
	result, err := conn.Exec(
		`INSERT INTO visitor (uuid, merchant_id, display_name, phone, email, fingerprint) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`,
		newUUID, merchantID, displayName, phone, email, fingerprint,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &Visitor{
		ID: id, UUID: newUUID, MerchantID: merchantID, DisplayName: displayName,
		Phone: sql.NullString{String: phone, Valid: phone != ""},
		Email: sql.NullString{String: email, Valid: email != ""},
	}, nil
}
