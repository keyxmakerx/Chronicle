-- 002_dm_screen_prefs: each person's arrangement and style for a docked
-- screen, so their layouts follow them from one device to the next.
--
-- One row per campaign, user and screen ("dm" for the DM Screen, "player" for
-- the players' screen). prefs is a JSON document the service validates and
-- caps at 16 KB before every write: the saved layouts, the arrangement the
-- screen shows now, and the surface and density picks. The kit pick is not
-- here; it stays with the member look picks so the owner's "who can change"
-- setting still governs it. A row goes with its campaign or its user.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS dm_screen_prefs (
    campaign_id CHAR(36)    NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    screen      VARCHAR(16) NOT NULL,
    prefs       TEXT        NOT NULL,
    updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (campaign_id, user_id, screen),
    KEY idx_dm_screen_prefs_user (user_id),
    CONSTRAINT fk_dm_screen_prefs_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_dm_screen_prefs_user     FOREIGN KEY (user_id)     REFERENCES users(id)     ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
