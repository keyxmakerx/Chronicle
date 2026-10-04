-- Saved room layout for shop entities (the isometric shop room widget).
--
-- One row per shop: the layout is a normalized JSON document the Owner
-- arranges and every viewer of the shop reads. It only references shop goods
-- by relation id; the goods themselves stay in entity_relations.
--
-- updated_by is deliberately not a foreign key: the layout must outlive the
-- account that last saved it. Deleting the shop or the campaign removes the row.
-- Core tables only (campaigns, entities); core migration.

CREATE TABLE IF NOT EXISTS shop_rooms (
    shop_entity_id VARCHAR(36) NOT NULL PRIMARY KEY,
    campaign_id    VARCHAR(36) NOT NULL,
    layout         LONGTEXT    NOT NULL,
    updated_by     VARCHAR(36) DEFAULT NULL,
    updated_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_shop_rooms_campaign (campaign_id),
    CONSTRAINT fk_shop_rooms_entity FOREIGN KEY (shop_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_rooms_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
