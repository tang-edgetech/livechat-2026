-- Bot Analytics: one row per chat ever bot-driven, so an Admin can see
-- completion/handoff/abandonment rates and per-node drop-off instead of
-- building a flow blind. No FK on bot_flow_id — same deliberate
-- simplification as chat.bot_flow_id itself (0004_automation_bot.sql):
-- DeleteBotFlowHandler unconditionally deletes a bot_flow row today, and a
-- real FK would turn that into a 500 the moment any chat had ever run it.
-- chat_id DOES get a FK — chat rows are never hard-deleted (retention only
-- prunes `message` by age), matching message/file's own chat_id FK.
CREATE TABLE IF NOT EXISTS bot_flow_run (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  bot_flow_id BIGINT UNSIGNED NOT NULL,
  chat_id BIGINT UNSIGNED NOT NULL,
  merchant_id BIGINT UNSIGNED NOT NULL,
  last_node_id VARCHAR(64) NULL,
  outcome ENUM('active','completed','handoff','closed','abandoned') NOT NULL DEFAULT 'active',
  resolved_by ENUM('engine','staff','sweep') NOT NULL DEFAULT 'engine',
  started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at DATETIME NULL,
  UNIQUE KEY uq_bot_flow_run_chat (chat_id),
  INDEX idx_bot_flow_run_flow (bot_flow_id),
  FOREIGN KEY (chat_id) REFERENCES chat(id),
  FOREIGN KEY (merchant_id) REFERENCES merchant(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
