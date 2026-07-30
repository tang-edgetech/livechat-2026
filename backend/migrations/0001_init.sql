-- Phase 0 core tables — overview.md §4 (Core, Sessions/Auth, Settings, Audit Log)

CREATE TABLE IF NOT EXISTS role (
  id TINYINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  slug VARCHAR(30) NOT NULL UNIQUE,
  name VARCHAR(50) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO role (id, slug, name) VALUES
  (1, 'super_admin', 'Super Admin'),
  (2, 'admin', 'Admin'),
  (3, 'agent', 'Agent')
ON DUPLICATE KEY UPDATE slug = VALUES(slug);

CREATE TABLE IF NOT EXISTS user (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  uuid CHAR(36) NOT NULL UNIQUE,
  role_id TINYINT UNSIGNED NOT NULL,
  display_name VARCHAR(80) NOT NULL,
  username VARCHAR(50) NOT NULL UNIQUE,
  email VARCHAR(120) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  status ENUM('active','inactive','suspended') NOT NULL DEFAULT 'active',
  items_per_page SMALLINT UNSIGNED NULL,
  last_login_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (role_id) REFERENCES role(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS merchant (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  uuid CHAR(36) NOT NULL UNIQUE,
  name VARCHAR(120) NOT NULL,
  code VARCHAR(50) NOT NULL UNIQUE,
  status ENUM('active','suspended') NOT NULL DEFAULT 'active',
  routing_mode ENUM('manual','round_robin') NOT NULL DEFAULT 'manual',
  last_routed_agent_id BIGINT UNSIGNED NULL,
  widget_config JSON NULL,
  inactivity_timeout_minutes SMALLINT UNSIGNED NOT NULL DEFAULT 30,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (last_routed_agent_id) REFERENCES user(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_merchant (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  merchant_id BIGINT UNSIGNED NOT NULL,
  assigned_by BIGINT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_user_merchant (user_id, merchant_id),
  FOREIGN KEY (user_id) REFERENCES user(id),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id),
  FOREIGN KEY (assigned_by) REFERENCES user(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS session (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  token_hash VARCHAR(255) NOT NULL,
  ip VARCHAR(45) NULL,
  user_agent VARCHAR(255) NULL,
  last_activity_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES user(id),
  INDEX idx_session_token (token_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- NOTE: MySQL unique indexes treat NULL as distinct per row, so the unique
-- key below does not stop duplicate *global* (merchant_id IS NULL) keys.
-- Same limitation as `visitor` in overview.md §4 — enforced at the app
-- layer (check-then-upsert) for the global case; the DB constraint still
-- catches duplicates within a single merchant.
CREATE TABLE IF NOT EXISTS setting (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  merchant_id BIGINT UNSIGNED NULL,
  `key` VARCHAR(100) NOT NULL,
  value TEXT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  updated_by BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_setting_scope_key (merchant_id, `key`),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id),
  FOREIGN KEY (updated_by) REFERENCES user(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_log (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  merchant_id BIGINT UNSIGNED NULL,
  user_id BIGINT UNSIGNED NULL,
  category VARCHAR(50) NOT NULL,
  message VARCHAR(255) NOT NULL,
  status_code SMALLINT NOT NULL,
  status_message VARCHAR(100) NULL,
  source VARCHAR(50) NOT NULL,
  ip_address VARCHAR(45) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_audit_category (category),
  INDEX idx_audit_status_code (status_code),
  INDEX idx_audit_user (user_id),
  INDEX idx_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
