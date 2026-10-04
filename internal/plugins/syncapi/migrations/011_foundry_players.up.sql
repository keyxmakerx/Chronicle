-- Foundry players: who is in the campaign's Foundry world and which member
-- each is linked to, as the GM's Foundry client last reported. Chronicle
-- cannot see this itself (players' changes arrive on the GM's connection),
-- so the owner's Foundry page reads it from here. Each report replaces the
-- campaign's rows; rows not reported for a while are pruned. Names are what
-- Foundry users call themselves: display text only, never trusted.

CREATE TABLE IF NOT EXISTS foundry_players (
    campaign_id     VARCHAR(36)  NOT NULL,
    foundry_user_id VARCHAR(64)  NOT NULL,
    foundry_name    VARCHAR(100) NOT NULL DEFAULT '',
    member_user_id  VARCHAR(36)  NULL DEFAULT NULL,
    is_online       TINYINT(1)   NOT NULL DEFAULT 0,
    last_change_at  DATETIME(3)  NULL DEFAULT NULL,
    last_failed_at  DATETIME(3)  NULL DEFAULT NULL,
    last_failure    VARCHAR(200) NOT NULL DEFAULT '',
    failed_count    INT          NOT NULL DEFAULT 0,
    reported_at     DATETIME(3)  NOT NULL,

    PRIMARY KEY (campaign_id, foundry_user_id),
    KEY idx_foundry_players_reported (reported_at),
    CONSTRAINT fk_foundry_players_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
