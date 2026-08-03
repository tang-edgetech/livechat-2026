// Package settings wraps the generic `setting` key/value table
// (overview.md §9) for global (merchant_id IS NULL) values: site config,
// per-data-type retention windows, and file format/size rules.
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
	rows, err := conn.Query("SELECT `key`, value FROM setting WHERE merchant_id IS NULL")
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
	err := conn.QueryRow("SELECT value FROM setting WHERE merchant_id IS NULL AND `key` = ?", key).Scan(&v)
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

func Set(conn *sql.DB, key, value string, updatedBy int64) error {
	_, err := conn.Exec(
		"INSERT INTO setting (merchant_id, `key`, value, updated_by) VALUES (NULL, ?, ?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value), updated_by = VALUES(updated_by)",
		key, value, updatedBy,
	)
	return err
}
