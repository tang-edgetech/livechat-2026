-- Per-user Appearance preference (Profile tab), account-persisted so it
-- follows the user across devices rather than living in localStorage.
ALTER TABLE user ADD COLUMN IF NOT EXISTS theme_preference VARCHAR(20) NOT NULL DEFAULT 'light';
