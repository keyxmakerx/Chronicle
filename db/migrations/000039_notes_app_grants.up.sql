-- A player's permission for an app outside Chronicle (the Foundry notebook)
-- to use that player's notes in one campaign. Only the SHA-256 of the token is
-- stored; the token itself is shown once, to the window that asked for it.
-- origin is the web address of that window, checked against the allowed
-- origins when the grant is made and shown back to the player.
-- Core tables, core migration: only campaigns and users are referenced.

CREATE TABLE IF NOT EXISTS notes_app_grants (
    id           CHAR(36)     NOT NULL PRIMARY KEY,
    campaign_id  CHAR(36)     NOT NULL,
    user_id      CHAR(36)     NOT NULL,
    token_hash   CHAR(64)     NOT NULL,
    origin       VARCHAR(255) NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME     DEFAULT NULL,
    revoked_at   DATETIME     DEFAULT NULL,

    UNIQUE KEY uq_notes_app_grants_token (token_hash),
    INDEX idx_notes_app_grants_user (campaign_id, user_id),
    CONSTRAINT fk_notes_app_grants_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_notes_app_grants_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
