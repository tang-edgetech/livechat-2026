// Package retention implements the auto-purge job from overview.md §9:
// audit_log/message/file rows past their `setting`-defined retention
// window (default 365 days each) age out on their own, plus a manual
// "purge now" action. In-process daily ticker per the Decisions Log —
// no separate worker process for v1.
package retention

import (
	"database/sql"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/lock"
	"livechat/backend/internal/settings"
	"livechat/backend/internal/storage"
)

type Report struct {
	AuditLogsDeleted int64 `json:"audit_logs_deleted"`
	MessagesDeleted  int64 `json:"messages_deleted"`
	FilesDeleted     int64 `json:"files_deleted"`
}

// Sweep deletes expired rows for all three data types. File rows also
// have their on-disk blob removed via the storage driver — best-effort
// per row so one bad path doesn't abort the rest of the sweep.
func Sweep(conn *sql.DB, driver storage.Driver) (Report, error) {
	var report Report

	if res, err := conn.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoff(conn, "retention_audit_log_days")); err == nil {
		report.AuditLogsDeleted, _ = res.RowsAffected()
	} else {
		return report, err
	}

	if res, err := conn.Exec(`DELETE FROM message WHERE created_at < ?`, cutoff(conn, "retention_message_days")); err == nil {
		report.MessagesDeleted, _ = res.RowsAffected()
	} else {
		return report, err
	}

	rows, err := conn.Query(`SELECT id, disk_path FROM file WHERE created_at < ?`, cutoff(conn, "retention_file_days"))
	if err != nil {
		return report, err
	}
	var ids []int64
	var paths []string
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			rows.Close()
			return report, err
		}
		ids = append(ids, id)
		paths = append(paths, path)
	}
	rows.Close()

	for i, id := range ids {
		_ = driver.Delete(paths[i])
		if _, err := conn.Exec(`DELETE FROM file WHERE id = ?`, id); err == nil {
			report.FilesDeleted++
		}
	}

	return report, nil
}

func cutoff(conn *sql.DB, key string) time.Time {
	val, err := settings.Get(conn, key)
	if err != nil || val == "" {
		val = settings.Defaults[key]
	}
	days, err := strconv.Atoi(val)
	if err != nil || days <= 0 {
		days = 365
	}
	return time.Now().AddDate(0, 0, -days)
}

// StartScheduler runs the sweep once a day. It no-ops while the DB isn't
// configured yet (pre-Setup-Wizard) rather than erroring. redisClient may
// be nil; the lock only matters once more than one Go instance is
// running (overview.md §11 Phase 7) so they don't all purge the same
// rows redundantly — harmless either way since every delete is
// idempotent, just wasted work without it.
func StartScheduler(state *appstate.State, driver storage.Driver, redisClient *redis.Client) {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			if !lock.TryAcquire(redisClient, "retention-sweep", 23*time.Hour) {
				continue
			}
			conn := state.DB()
			if conn == nil {
				continue
			}
			if _, err := Sweep(conn, driver); err != nil {
				log.Printf("retention sweep failed: %v", err)
			}
		}
	}()
}
