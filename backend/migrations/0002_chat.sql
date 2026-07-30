-- Phase 2 core tables — overview.md §4 (Chat, Files)

CREATE TABLE IF NOT EXISTS visitor (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  uuid CHAR(36) NOT NULL UNIQUE,
  merchant_id BIGINT UNSIGNED NOT NULL,
  display_name VARCHAR(80) NOT NULL DEFAULT 'Visitor',
  phone VARCHAR(20) NULL,
  email VARCHAR(120) NULL,
  fingerprint VARCHAR(100) NULL,
  merged_into_id BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (merchant_id) REFERENCES merchant(id),
  FOREIGN KEY (merged_into_id) REFERENCES visitor(id),
  INDEX idx_visitor_merchant_phone (merchant_id, phone),
  INDEX idx_visitor_merchant_email (merchant_id, email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS chat (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  uuid CHAR(36) NOT NULL UNIQUE,
  merchant_id BIGINT UNSIGNED NOT NULL,
  visitor_id BIGINT UNSIGNED NOT NULL,
  agent_id BIGINT UNSIGNED NULL,
  status ENUM('active','pending','closed','bot') NOT NULL DEFAULT 'pending',
  started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  closed_at DATETIME NULL,
  last_message_at DATETIME NULL,
  FOREIGN KEY (merchant_id) REFERENCES merchant(id),
  FOREIGN KEY (visitor_id) REFERENCES visitor(id),
  FOREIGN KEY (agent_id) REFERENCES user(id),
  INDEX idx_chat_merchant_status (merchant_id, status),
  INDEX idx_chat_agent (agent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS message (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  chat_id BIGINT UNSIGNED NOT NULL,
  sender_type ENUM('visitor','agent','bot','system') NOT NULL,
  sender_id BIGINT UNSIGNED NULL,
  body TEXT NOT NULL,
  type ENUM('text','file','system','quick_reply') NOT NULL DEFAULT 'text',
  metadata JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (chat_id) REFERENCES chat(id),
  INDEX idx_message_chat_created (chat_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS file (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  uuid CHAR(36) NOT NULL UNIQUE,
  merchant_id BIGINT UNSIGNED NOT NULL,
  chat_id BIGINT UNSIGNED NULL,
  uploader_type ENUM('visitor','user','system') NOT NULL,
  uploader_id BIGINT UNSIGNED NULL,
  purpose ENUM('chat','automation','bot','branding') NOT NULL,
  disk_path VARCHAR(255) NOT NULL,
  original_name VARCHAR(255) NOT NULL,
  mime_type VARCHAR(100) NOT NULL,
  size_bytes INT UNSIGNED NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (merchant_id) REFERENCES merchant(id),
  FOREIGN KEY (chat_id) REFERENCES chat(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
