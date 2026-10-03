-- Purchase requests: a player's basket from a shop, held for the GM while
-- downtime is closed.
--
-- The basket stores only listing ids and quantities, never per-item prices:
-- coins, stock and prices are all read again when the GM approves or downtime
-- opens. quoted_total / quoted_currency are the basket's total when it was
-- asked for; a request is refused at apply time if the price has gone up past
-- it, so a player is never charged more than they saw.
-- status 'applied' is written before the basket runs and 'failed' replaces it
-- if the basket cannot go through, so a crash can never leave a request that
-- charges twice.
--
-- requested_by / decided_by are deliberately not foreign keys, as in
-- item_moves: the history must outlive the account.
-- Core tables only (campaigns, entities); core migration.

CREATE TABLE IF NOT EXISTS shop_purchase_requests (
    id              BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id     VARCHAR(36)  NOT NULL,
    shop_entity_id  VARCHAR(36)  NOT NULL,
    buyer_entity_id VARCHAR(36)  NOT NULL,
    requested_by    VARCHAR(36)  NOT NULL,
    basket          JSON         NOT NULL,
    quoted_total    DECIMAL(14,2) NOT NULL,
    quoted_currency VARCHAR(255) NOT NULL,
    status          ENUM('pending','applied','declined','failed') NOT NULL DEFAULT 'pending',
    reason          VARCHAR(255) DEFAULT NULL,
    decided_by      VARCHAR(36)  DEFAULT NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at      TIMESTAMP    NULL DEFAULT NULL,

    INDEX idx_shop_purchase_requests_campaign_status (campaign_id, status, created_at),
    INDEX idx_shop_purchase_requests_requester (campaign_id, requested_by, status),
    INDEX idx_shop_purchase_requests_buyer (buyer_entity_id, created_at),
    INDEX idx_shop_purchase_requests_shop (shop_entity_id),
    CONSTRAINT fk_shop_purchase_requests_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_purchase_requests_shop FOREIGN KEY (shop_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_purchase_requests_buyer FOREIGN KEY (buyer_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
