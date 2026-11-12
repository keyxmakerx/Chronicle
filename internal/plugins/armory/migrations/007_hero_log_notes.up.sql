-- 007_hero_log_notes: the lines a player (or the Director) writes in a hero's
-- log. What happened to the hero (items gained, given, crafted) is read from
-- item_moves, never copied here, so this table holds only the written words.
-- character_id is the hero's entity id; item_entity_id optionally names an item
-- the line is about and is cleared if that item page is deleted. author_id is
-- deliberately not a foreign key, as for storefronts. director_only lines are
-- shown only to viewers who see DM-only content.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS armory_hero_notes (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id    VARCHAR(36)  NOT NULL,
    character_id   VARCHAR(36)  NOT NULL,
    author_id      VARCHAR(36)  NOT NULL,
    body           VARCHAR(1000) NOT NULL,
    item_entity_id VARCHAR(36)  DEFAULT NULL,
    director_only  TINYINT(1)   NOT NULL DEFAULT 0,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_armory_hero_notes_character (character_id, created_at),
    CONSTRAINT fk_armory_hero_notes_character FOREIGN KEY (character_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_hero_notes_item FOREIGN KEY (item_entity_id)
        REFERENCES entities(id) ON DELETE SET NULL,
    CONSTRAINT fk_armory_hero_notes_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
