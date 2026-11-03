-- 006_shop_shelves: how many places on a shop's "today's shelf" change each
-- day. Which goods can turn up there is a flag on each "sells" listing; the
-- day's pick is worked out from the calendar, never stored. A shop with no
-- row changes the default number of places. updated_by is deliberately not
-- a foreign key, as for storefronts.
CREATE TABLE IF NOT EXISTS armory_shop_shelves (
    shop_entity_id VARCHAR(36) NOT NULL PRIMARY KEY,
    campaign_id    VARCHAR(36) NOT NULL,
    today_spots    TINYINT UNSIGNED NOT NULL DEFAULT 2,
    updated_by     VARCHAR(36) DEFAULT NULL,
    updated_at     TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_armory_shop_shelves_campaign (campaign_id),
    CONSTRAINT fk_armory_shop_shelves_shop FOREIGN KEY (shop_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_armory_shop_shelves_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
