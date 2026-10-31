-- 003_shop_fronts: how a shop's storefront is drawn on the Shops street.
--
-- One row per shop: a small normalized JSON document (building style,
-- colours, awning, sign, window wares, and the look it takes when closed).
-- updated_by is deliberately not a foreign key: the storefront must outlive
-- the account that last saved it. Deleting the shop or the campaign removes
-- the row. Core migrations run first, so the foreign keys are safe.
CREATE TABLE IF NOT EXISTS armory_shop_fronts (
    shop_entity_id VARCHAR(36) NOT NULL PRIMARY KEY,
    campaign_id    VARCHAR(36) NOT NULL,
    front          TEXT        NOT NULL,
    updated_by     VARCHAR(36) DEFAULT NULL,
    updated_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_armory_shop_fronts_campaign (campaign_id),
    CONSTRAINT fk_armory_shop_fronts_shop FOREIGN KEY (shop_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_shop_fronts_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
