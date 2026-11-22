-- 001_hero_change_state: what decides when a player may change their own
-- hero's picks, beyond the campaign's rule (campaigns.settings hero_changes).
--
-- One row per hero page, written only when there is something to keep:
-- level and level_at are the level the hero last reached and when, recorded
-- as the level field goes up, so "for N days after a new level" can be told;
-- reopened_until is a Director's exception for this one hero, and
-- reopened_by who gave it (no foreign key: it only records who acted). A
-- hero with no row has made no level-up since this table existed and has no
-- exception.
CREATE TABLE IF NOT EXISTS hero_change_state (
    entity_id       CHAR(36)     NOT NULL,
    campaign_id     CHAR(36)     NOT NULL,
    level           INT          NOT NULL DEFAULT 0,
    level_at        DATETIME(3)  NULL,
    reopened_until  DATETIME(3)  NULL,
    reopened_by     CHAR(36)     NULL,
    updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (entity_id),
    KEY idx_hero_change_state_campaign (campaign_id),
    CONSTRAINT fk_hero_change_state_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_hero_change_state_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
