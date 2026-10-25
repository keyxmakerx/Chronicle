-- Foundry monsters: the actors a campaign's Foundry world can place, as the
-- GM's Foundry client last reported them (its world's NPC actors and its
-- Actor compendiums' entries). The AI connection reads it to build fights
-- from monsters Foundry already has. One row per campaign holds the whole
-- list, so a report replaces it in one write; rows not reported for a while
-- are pruned. Everything in it is Foundry's word: display text only.

CREATE TABLE IF NOT EXISTS foundry_monster_lists (
    campaign_id  VARCHAR(36) NOT NULL,
    monsters     MEDIUMTEXT  NOT NULL,
    truncated    TINYINT(1)  NOT NULL DEFAULT 0,
    reported_at  DATETIME(3) NOT NULL,

    PRIMARY KEY (campaign_id),
    KEY idx_foundry_monster_lists_reported (reported_at),
    CONSTRAINT fk_foundry_monster_lists_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
