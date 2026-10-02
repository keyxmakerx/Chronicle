-- Site-wide log of changes made from the admin area: one row per change, who
-- made it and what it touched. security_events covers sign-in and session
-- security and campaign audit rows are per-campaign; neither answers "what did
-- the admins change on this site?", which the admin Home page shows.
-- actor_user_id is deliberately not a foreign key: the log must outlive the
-- account that made the change, and the page shows a neutral label for it.
-- Core table, core migration: no plugin tables referenced.

CREATE TABLE IF NOT EXISTS admin_activity (
    id            BIGINT       AUTO_INCREMENT PRIMARY KEY,
    actor_user_id CHAR(36)     DEFAULT NULL,
    action        VARCHAR(64)  NOT NULL,
    target_type   VARCHAR(32)  NOT NULL DEFAULT '',
    target_id     VARCHAR(64)  NOT NULL DEFAULT '',
    target_label  VARCHAR(255) NOT NULL DEFAULT '',
    detail        JSON         DEFAULT NULL,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_admin_activity_created (created_at DESC),
    INDEX idx_admin_activity_actor (actor_user_id, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
