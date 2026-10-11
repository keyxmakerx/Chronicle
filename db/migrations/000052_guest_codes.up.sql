-- Guest codes: an owner hands a player a one-use code, and the player joins
-- one campaign with only a name. The guest's account is fenced to that
-- campaign by users.guest_campaign_id until they add an email and password
-- or merge into an existing account.
-- Only the code's hash is stored; the owner sees the code once.
-- Core tables, core migration: no plugin tables referenced.

ALTER TABLE users ADD COLUMN IF NOT EXISTS guest_campaign_id CHAR(36) NULL;
CREATE INDEX IF NOT EXISTS idx_users_guest_campaign ON users (guest_campaign_id);

CREATE TABLE IF NOT EXISTS campaign_guest_codes (
    id          CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    code_hash   CHAR(64)     NOT NULL,
    note        VARCHAR(100) NOT NULL DEFAULT '',
    created_by  CHAR(36)     NOT NULL,
    expires_at  DATETIME     NOT NULL,
    used_by     CHAR(36)     NULL,
    used_at     DATETIME     NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_campaign_guest_codes_hash (code_hash),
    KEY idx_campaign_guest_codes_campaign (campaign_id),
    CONSTRAINT fk_campaign_guest_codes_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_guest_codes_creator
        FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_campaign_guest_codes_user
        FOREIGN KEY (used_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
