// Package settings wraps the generic `setting` key/value table
// (overview.md §9) for global (merchant_id IS NULL) values: site config,
// per-data-type retention windows, and file format/size rules. Also
// supports a per-merchant override of the same keys (currently used for
// file format/size rules — overview.md §6.8) via GetMerchant/SetMerchant.
package settings

import "database/sql"

var Defaults = map[string]string{
	"site_title":               "LiveChat",
	"timezone":                 "UTC",
	"items_per_page_default":   "20",
	"retention_audit_log_days": "365",
	"retention_message_days":   "365",
	"retention_file_days":      "365",
	"file_allowed_extensions":  "",
	"file_max_size_mb":         "20",
}

func GetAll(conn *sql.DB) (map[string]string, error) {
	out := make(map[string]string, len(Defaults))
	for k, v := range Defaults {
		out[k] = v
	}
	// ORDER BY + last-write-wins in the loop below is defense in depth
	// against any leftover duplicate rows from before Set() was fixed to
	// check-then-upsert instead of relying on MySQL's unique index (which
	// treats every merchant_id IS NULL row as distinct, never NULL-equal
	// to another — see the migration's own note on `setting`).
	rows, err := conn.Query("SELECT `key`, value FROM setting WHERE merchant_id IS NULL ORDER BY updated_at ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		if v.Valid {
			out[k] = v.String
		}
	}
	return out, rows.Err()
}

func Get(conn *sql.DB, key string) (string, error) {
	var v sql.NullString
	err := conn.QueryRow("SELECT value FROM setting WHERE merchant_id IS NULL AND `key` = ? ORDER BY updated_at DESC LIMIT 1", key).Scan(&v)
	if err == sql.ErrNoRows {
		return Defaults[key], nil
	}
	if err != nil {
		return "", err
	}
	if !v.Valid {
		return Defaults[key], nil
	}
	return v.String, nil
}

// Set is a check-then-upsert rather than INSERT ... ON DUPLICATE KEY
// UPDATE — MySQL never treats one merchant_id IS NULL row as a duplicate
// of another, so ON DUPLICATE KEY UPDATE silently INSERTs a fresh row
// every call instead of updating the existing one (this was a real bug:
// every global settings save accumulated a new row instead of replacing
// the old one).
func Set(conn *sql.DB, key, value string, updatedBy int64) error {
	res, err := conn.Exec(
		"UPDATE setting SET value = ?, updated_by = ? WHERE merchant_id IS NULL AND `key` = ?",
		value, updatedBy, key,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = conn.Exec(
		"INSERT INTO setting (merchant_id, `key`, value, updated_by) VALUES (NULL, ?, ?, ?)",
		key, value, updatedBy,
	)
	return err
}

// GetMerchant returns the merchant-scoped override for key if one has
// been set, falling back to the global value (Get) otherwise. The second
// return value reports whether an override actually exists, so a caller
// (the Files settings UI) can show "using global default" vs "custom for
// this merchant" rather than just a value with no provenance.
func GetMerchant(conn *sql.DB, merchantID int64, key string) (string, bool, error) {
	var v sql.NullString
	err := conn.QueryRow("SELECT value FROM setting WHERE merchant_id = ? AND `key` = ?", merchantID, key).Scan(&v)
	if err == sql.ErrNoRows {
		global, gerr := Get(conn, key)
		return global, false, gerr
	}
	if err != nil {
		return "", false, err
	}
	if !v.Valid {
		global, gerr := Get(conn, key)
		return global, false, gerr
	}
	return v.String, true, nil
}

// SetMerchant upserts a merchant-scoped override. Unlike the global case,
// merchant_id here is a concrete value, so the table's unique key
// (merchant_id, `key`) genuinely enforces one row per pair and ON
// DUPLICATE KEY UPDATE works correctly.
func SetMerchant(conn *sql.DB, merchantID int64, key, value string, updatedBy int64) error {
	_, err := conn.Exec(
		"INSERT INTO setting (merchant_id, `key`, value, updated_by) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value), updated_by = VALUES(updated_by)",
		merchantID, key, value, updatedBy,
	)
	return err
}

// ClearMerchant removes a merchant's override, reverting it back to
// inheriting the global default.
func ClearMerchant(conn *sql.DB, merchantID int64, key string) error {
	_, err := conn.Exec("DELETE FROM setting WHERE merchant_id = ? AND `key` = ?", merchantID, key)
	return err
}
