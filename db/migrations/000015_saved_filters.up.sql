-- Per-user saved filter presets for sidebar tag filtering.
-- Allows users to save tag combos as named presets for quick switching.
CREATE TABLE saved_filters (
  id             CHAR(36)     NOT NULL PRIMARY KEY,
  user_id        CHAR(36)     NOT NULL,
  campaign_id    CHAR(36)     NOT NULL,
  entity_type_id INT          NULL,
  name           VARCHAR(100) NOT NULL,
  tag_slugs      JSON         NOT NULL,
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
  INDEX idx_saved_filters (user_id, campaign_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
