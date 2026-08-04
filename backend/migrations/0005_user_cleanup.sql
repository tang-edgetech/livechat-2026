-- Username is being dropped entirely (login becomes email + password
-- only); created_by adds provenance to the Users list; is_html lets a
-- canned message / automation rule opt into sanitized-HTML rendering
-- instead of the default plain-text escaping every message gets.
ALTER TABLE user DROP COLUMN IF EXISTS username;

-- No named FK constraint here deliberately: RunMigrations executes each
-- statement via a pooled *sql.DB one at a time (see db/migrate.go),
-- which offers no guarantee that a guard-then-add pattern (SET a session
-- variable, then a dynamic ALTER) lands on the same underlying
-- connection — the exact kind of fragility this file's IF NOT EXISTS
-- guards elsewhere are trying to avoid. created_by is enforced at the
-- application layer instead (same pragmatic tradeoff already made for
-- `setting`'s merchant_id NULL-uniqueness elsewhere in this schema).
ALTER TABLE user ADD COLUMN IF NOT EXISTS created_by BIGINT UNSIGNED NULL AFTER status;

ALTER TABLE canned_message ADD COLUMN IF NOT EXISTS is_html TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE automation_rule ADD COLUMN IF NOT EXISTS is_html TINYINT(1) NOT NULL DEFAULT 0;
