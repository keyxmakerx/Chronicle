-- Foundry monster matches: what the DM decided about a creature page and the
-- monster Foundry has of the same name. The match itself is worked out from
-- the latest monster report; a row holds only the DM's choices: the monster
-- they matched by hand (empty follows the page's name), whose numbers play
-- ("foundry", "page", or empty for the default), and the same-named monsters
-- they said are a different creature (a JSON array of names).

CREATE TABLE IF NOT EXISTS foundry_monster_matches (
    campaign_id   CHAR(36)     NOT NULL,
    entity_id     CHAR(36)     NOT NULL,
    monster_name  VARCHAR(120) NOT NULL DEFAULT '',
    monster_pack  VARCHAR(120) NOT NULL DEFAULT '',
    use_numbers   VARCHAR(10)  NOT NULL DEFAULT '',
    not_same      TEXT         NOT NULL,
    updated_at    DATETIME(3)  NOT NULL,

    PRIMARY KEY (campaign_id, entity_id),
    KEY idx_foundry_monster_matches_entity (entity_id),
    CONSTRAINT fk_foundry_monster_matches_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_foundry_monster_matches_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
