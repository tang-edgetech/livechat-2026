-- Phase 3 needs `integration` for widget_identity (passthrough auth,
-- overview.md §10.2). The other types (api_key, webhook, auto_login) sit
-- unused in the enum until Phases 4/6 actually create rows of those
-- types — same table, no schema change needed later.

CREATE TABLE IF NOT EXISTS integration (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  type ENUM('api_key','auto_login','webhook','widget_identity') NOT NULL,
  config JSON NULL,
  secret_hash VARCHAR(255) NOT NULL,
  is_global BOOLEAN NOT NULL DEFAULT FALSE,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS integration_merchant (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  integration_id BIGINT UNSIGNED NOT NULL,
  merchant_id BIGINT UNSIGNED NOT NULL,
  UNIQUE KEY uq_integration_merchant (integration_id, merchant_id),
  FOREIGN KEY (integration_id) REFERENCES integration(id),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
