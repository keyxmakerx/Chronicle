-- 002_item_looks: how an item shows its magic (the item_look widget).
--
-- One row per item: a small normalized JSON document naming a look from a
-- fixed gallery, an optional colour and sound, and the words shown when it is
-- used. updated_by is deliberately not a foreign key: the look must outlive
-- the account that last saved it. Deleting the item or the campaign removes
-- the row. Core migrations run before plugin ones, so the foreign keys to
-- campaigns and entities are safe; the collation matches those tables.
CREATE TABLE IF NOT EXISTS armory_item_looks (
    item_entity_id VARCHAR(36) NOT NULL PRIMARY KEY,
    campaign_id    VARCHAR(36) NOT NULL,
    look           TEXT        NOT NULL,
    updated_by     VARCHAR(36) DEFAULT NULL,
    updated_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_armory_item_looks_campaign (campaign_id),
    CONSTRAINT fk_armory_item_looks_item FOREIGN KEY (item_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_item_looks_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
