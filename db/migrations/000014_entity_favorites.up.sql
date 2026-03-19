-- Per-user entity favorites for sidebar bookmarks.
-- Replaces the localStorage-based favorites with DB-backed persistence
-- so favorites sync across devices and sessions.
CREATE TABLE entity_favorites (
  user_id     CHAR(36)  NOT NULL,
  entity_id   CHAR(36)  NOT NULL,
  campaign_id CHAR(36)  NOT NULL,
  created_at  DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, entity_id),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
  FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
  INDEX idx_favorites_campaign (user_id, campaign_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
