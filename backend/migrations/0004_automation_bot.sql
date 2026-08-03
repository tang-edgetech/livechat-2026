-- Phase 4 — overview.md §4 (Automation / Bot)

CREATE TABLE IF NOT EXISTS automation_rule (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(120) NOT NULL,
  trigger_type VARCHAR(50) NOT NULL DEFAULT 'chat_start',
  `condition` JSON NULL,
  message TEXT NOT NULL,
  is_global BOOLEAN NOT NULL DEFAULT FALSE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS automation_rule_merchant (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  automation_rule_id BIGINT UNSIGNED NOT NULL,
  merchant_id BIGINT UNSIGNED NOT NULL,
  UNIQUE KEY uq_automation_rule_merchant (automation_rule_id, merchant_id),
  FOREIGN KEY (automation_rule_id) REFERENCES automation_rule(id),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS canned_message (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  title VARCHAR(120) NOT NULL,
  body TEXT NOT NULL,
  is_global BOOLEAN NOT NULL DEFAULT FALSE,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS canned_message_merchant (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  canned_message_id BIGINT UNSIGNED NOT NULL,
  merchant_id BIGINT UNSIGNED NOT NULL,
  UNIQUE KEY uq_canned_message_merchant (canned_message_id, merchant_id),
  FOREIGN KEY (canned_message_id) REFERENCES canned_message(id),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bot_flow (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(120) NOT NULL,
  trigger_config JSON NOT NULL,
  flow JSON NOT NULL,
  integration_id BIGINT UNSIGNED NULL,
  is_global BOOLEAN NOT NULL DEFAULT FALSE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (integration_id) REFERENCES integration(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bot_flow_merchant (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  bot_flow_id BIGINT UNSIGNED NOT NULL,
  merchant_id BIGINT UNSIGNED NOT NULL,
  UNIQUE KEY uq_bot_flow_merchant (bot_flow_id, merchant_id),
  FOREIGN KEY (bot_flow_id) REFERENCES bot_flow(id),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- A chat currently being driven by a bot flow (status='bot') needs to
-- remember where it is. No formal FK on bot_flow_id — same deliberate
-- simplification as message.sender_id's polymorphic reference (§4) —
-- kept a plain indexed column instead so this ALTER stays idempotent.
ALTER TABLE chat ADD COLUMN IF NOT EXISTS bot_flow_id BIGINT UNSIGNED NULL;
ALTER TABLE chat ADD COLUMN IF NOT EXISTS bot_node_id VARCHAR(64) NULL;
ALTER TABLE chat ADD COLUMN IF NOT EXISTS bot_variables JSON NULL;
CREATE INDEX IF NOT EXISTS idx_chat_bot_flow ON chat(bot_flow_id);
