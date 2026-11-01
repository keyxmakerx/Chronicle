-- 004_item_places: where a hero carries each item they hold.
--
-- The hero page draws a character's items in containers (in hand, worn, belt
-- pouch, backpack). The count stays on the Has Item line; this row only says
-- which container the item sits in, and an item with no row is in the
-- backpack. character_id is the character's entity id. A row whose item the
-- character no longer holds is never read.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS armory_item_places (
    campaign_id    VARCHAR(36) NOT NULL,
    character_id   VARCHAR(36) NOT NULL,
    item_entity_id VARCHAR(36) NOT NULL,
    place          VARCHAR(12) NOT NULL,
    updated_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, item_entity_id),
    INDEX idx_armory_item_places_campaign (campaign_id, character_id),
    CONSTRAINT fk_armory_item_places_character FOREIGN KEY (character_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_item_places_item FOREIGN KEY (item_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_item_places_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
