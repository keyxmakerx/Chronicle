-- Customize drafts: the owner's unsaved Customize work, kept on the server so
-- it survives a closed tab or a second device until Save or Discard.
-- One row per owner per campaign; version is the optimistic-concurrency
-- counter a writer must echo back so two tabs cannot overwrite each other.
-- Core tables, core migration: no plugin tables referenced.

CREATE TABLE IF NOT EXISTS customize_drafts (
    campaign_id CHAR(36)     NOT NULL,
    user_id     CHAR(36)     NOT NULL,
    draft_json  MEDIUMTEXT   NOT NULL,
    version     INT UNSIGNED NOT NULL DEFAULT 1,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, user_id),
    KEY idx_customize_drafts_user (user_id),
    CONSTRAINT fk_customize_drafts_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_customize_drafts_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
