-- Update health: per changed feature, when this server first ran with it and
-- how often it has been used and failed since. feature_id names an entry of
-- the code's changed-features list; rows for entries since removed are
-- ignored on read. Counts only: no user, campaign, address or request data.
-- Core table, core migration: no plugin tables referenced.

CREATE TABLE IF NOT EXISTS update_health (
    feature_id     VARCHAR(64)  NOT NULL PRIMARY KEY,
    first_seen_at  DATETIME     NOT NULL,
    uses           INT UNSIGNED NOT NULL DEFAULT 0,
    errors         INT UNSIGNED NOT NULL DEFAULT 0,
    last_error_at  DATETIME     NULL DEFAULT NULL,
    last_error     VARCHAR(300) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
