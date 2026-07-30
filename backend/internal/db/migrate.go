package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrations executes every .sql file under migrationsDir, in filename
// order, statement by statement. Idempotent — every statement uses
// CREATE TABLE IF NOT EXISTS / INSERT ... ON DUPLICATE KEY, so re-running
// is safe (needed since the Setup Wizard's "Run Migration" step may be
// retried after a partial failure).
func RunMigrations(conn *sql.DB, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		for _, stmt := range splitStatements(string(content)) {
			if _, err := conn.Exec(stmt); err != nil {
				return fmt.Errorf("executing statement in %s: %w\n%s", name, err, stmt)
			}
		}
	}
	return nil
}

func splitStatements(sqlText string) []string {
	lines := strings.Split(sqlText, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || trimmed == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	joined := strings.Join(cleaned, "\n")

	raw := strings.Split(joined, ";")
	var stmts []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}
