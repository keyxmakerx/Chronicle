-- Foundry scenes: the scenes in the campaign's Foundry world, as the GM's
-- Foundry client last reported them, so the owner (and the owner's AI) can
-- see which maps already exist in Foundry. Each report replaces the
-- campaign's scene rows and its report row. The report row is kept apart so
-- a world with no scenes reads differently from one that never reported.
-- Names are what the GM typed in Foundry: display text only, never trusted.

CREATE TABLE IF NOT EXISTS foundry_scene_reports (
    campaign_id VARCHAR(36) NOT NULL PRIMARY KEY,
    scene_count INT         NOT NULL DEFAULT 0,
    truncated   TINYINT(1)  NOT NULL DEFAULT 0,
    reported_at DATETIME(3) NOT NULL,

    CONSTRAINT fk_foundry_scene_reports_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS foundry_scenes (
    campaign_id   VARCHAR(36)  NOT NULL,
    scene_id      VARCHAR(64)  NOT NULL,
    name          VARCHAR(255) NOT NULL DEFAULT '',
    folder        VARCHAR(255) NOT NULL DEFAULT '',
    is_active     TINYINT(1)   NOT NULL DEFAULT 0,
    in_navigation TINYINT(1)   NOT NULL DEFAULT 0,

    PRIMARY KEY (campaign_id, scene_id),
    KEY idx_foundry_scenes_name (campaign_id, name),
    CONSTRAINT fk_foundry_scenes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
