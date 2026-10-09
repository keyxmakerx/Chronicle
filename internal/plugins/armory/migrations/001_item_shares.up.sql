-- 001_item_shares: which view grants on a hidden item came from a player
-- sharing it with the rest of the party.
--
-- A row means the holder (character_id) shared the item with user_id. The
-- grant itself lives on the entity's allow list like any other; this table
-- only remembers that sharing asked for it, so un-sharing can take back
-- exactly what sharing added and never a grant the GM set by hand or one a
-- give added. made_grant is 1 on the row whose share added the grant (it
-- was not there before); when that row goes and no other holder still shares
-- with the user, the grant is removed.
--
-- Core migrations run before plugin ones, so the foreign keys to campaigns
-- and entities are safe; the collation matches those tables.
CREATE TABLE IF NOT EXISTS armory_item_shares (
    campaign_id    VARCHAR(36) NOT NULL,
    character_id   VARCHAR(36) NOT NULL,
    item_entity_id VARCHAR(36) NOT NULL,
    user_id        VARCHAR(36) NOT NULL,
    made_grant     TINYINT(1)  NOT NULL DEFAULT 0,
    shared_by      VARCHAR(36) NOT NULL,
    created_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (character_id, item_entity_id, user_id),
    INDEX idx_armory_item_shares_item_user (item_entity_id, user_id),
    INDEX idx_armory_item_shares_campaign (campaign_id),
    CONSTRAINT fk_armory_item_shares_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_item_shares_character FOREIGN KEY (character_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_item_shares_item FOREIGN KEY (item_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
