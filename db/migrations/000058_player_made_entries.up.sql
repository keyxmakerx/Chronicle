-- Player-made entries. A Director can let one player write one entry of
-- their own for a pick list (a homebrew ancestry, say): that permission is a
-- row in campaign_entry_grants, spent when the player saves the entry
-- (entry_id set). budget is the points total the player's entry gets; 0
-- means the list has no budget. A player's entry is a draft ('draft' while
-- they write, 'sent' once they hand it in) that only they and the Directors
-- see until a Director accepts it; review_note is the Director's "what to
-- change" when they hand it back. Every existing entry is a Director's, so
-- the default is 'accepted'.
-- entry_id is deliberately not a foreign key: deleting the entry leaves the
-- permission spent, and a Director gives it again by choice.
-- Core tables, core migration: only campaigns and users are referenced.

ALTER TABLE campaign_system_entries
    ADD COLUMN IF NOT EXISTS status ENUM('accepted','draft','sent') NOT NULL DEFAULT 'accepted' AFTER for_entity_id;
ALTER TABLE campaign_system_entries
    ADD COLUMN IF NOT EXISTS review_note VARCHAR(500) NOT NULL DEFAULT '' AFTER status;

CREATE TABLE IF NOT EXISTS campaign_entry_grants (
    id          BIGINT      AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36)    NOT NULL,
    system_id   VARCHAR(64) NOT NULL,
    field_key   VARCHAR(64) NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    budget      INT         NOT NULL DEFAULT 0,
    entry_id    BIGINT      NULL DEFAULT NULL,
    granted_by  CHAR(36)    DEFAULT NULL,
    created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_campaign_entry_grants (campaign_id, system_id, field_key, user_id),
    KEY idx_campaign_entry_grants_user (user_id),
    CONSTRAINT fk_campaign_entry_grants_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_entry_grants_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
