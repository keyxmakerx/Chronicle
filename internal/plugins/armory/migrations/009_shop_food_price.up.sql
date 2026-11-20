-- 009_shop_food_price: what a day's food costs in the campaign's main coin.
-- The everyday goods list is priced relative to one day's food, so this one
-- number rescales every everyday price the stock fill drafts. Prices a
-- Director has already set on a listing are never rescaled. A campaign with
-- no row uses 1. updated_by is deliberately not a foreign key.
CREATE TABLE IF NOT EXISTS armory_campaign_prices (
    campaign_id VARCHAR(36)   NOT NULL PRIMARY KEY,
    food_price  DECIMAL(10,2) NOT NULL DEFAULT 1.00,
    updated_by  VARCHAR(36)   DEFAULT NULL,
    updated_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_armory_campaign_prices_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
