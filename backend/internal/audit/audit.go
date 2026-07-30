package audit

import (
	"database/sql"
	"log"
)

// Entry mirrors the `audit_log` table (overview.md §4). MerchantID/UserID
// are pointers because both columns are nullable (system-level events,
// pre-auth actions).
type Entry struct {
	MerchantID    *int64
	UserID        *int64
	Category      string
	Message       string
	StatusCode    int
	StatusMessage string
	Source        string
	IPAddress     string
}

// Log writes one audit_log row. Failure to write is logged to stderr but
// never returned as an error to the caller — an audit-logging outage must
// not take down the feature that triggered it.
func Log(conn *sql.DB, e Entry) {
	_, err := conn.Exec(
		`INSERT INTO audit_log (merchant_id, user_id, category, message, status_code, status_message, source, ip_address)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.MerchantID, e.UserID, e.Category, e.Message, e.StatusCode, e.StatusMessage, e.Source, e.IPAddress,
	)
	if err != nil {
		log.Printf("audit log write failed: %v (category=%s message=%q)", err, e.Category, e.Message)
	}
}
