-- 007_shop_counter_rolls: the owner's hidden notes on a shop's goods, and the
-- rolls players make at the counter (haggle, look closer).
--
-- shop_good_secrets holds what only a good "Look closer" roll may reveal. It
-- is a table of its own, never part of the room layout, because the layout is
-- served to every player who can see the shop.
--
-- shop_counter_rolls records one row per good, character, action and shelf
-- day. The unique key is the rule "one try a day": a second insert fails, so
-- two quick taps cannot both roll. A haggle row also carries the price change
-- the buy route applies that day; a look row's tier is what the character has
-- uncovered, and it is read across every day.
--
-- Core migrations run first, so the foreign keys are safe. Rows follow the
-- shop, the good (the relation), the character and the campaign out.
CREATE TABLE IF NOT EXISTS shop_good_secrets (
    campaign_id    VARCHAR(36)  NOT NULL,
    shop_entity_id VARCHAR(36)  NOT NULL,
    relation_id    INT          NOT NULL,
    note           VARCHAR(400) NOT NULL DEFAULT '',
    extra          VARCHAR(400) NOT NULL DEFAULT '',
    updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, shop_entity_id, relation_id),
    CONSTRAINT fk_shop_good_secrets_shop FOREIGN KEY (shop_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_good_secrets_relation FOREIGN KEY (relation_id)
        REFERENCES entity_relations(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_good_secrets_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS shop_counter_rolls (
    id             BIGINT       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id    VARCHAR(36)  NOT NULL,
    shop_entity_id VARCHAR(36)  NOT NULL,
    relation_id    INT          NOT NULL,
    character_id   VARCHAR(36)  NOT NULL,
    action         VARCHAR(10)  NOT NULL,
    shelf_day      BIGINT       NOT NULL,
    tier           TINYINT      NOT NULL,
    pct            SMALLINT     NOT NULL DEFAULT 0,
    roll_total     INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_shop_counter_rolls_try (campaign_id, shop_entity_id, relation_id, character_id, action, shelf_day),
    INDEX idx_shop_counter_rolls_character (campaign_id, shop_entity_id, character_id),
    CONSTRAINT fk_shop_counter_rolls_shop FOREIGN KEY (shop_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_counter_rolls_relation FOREIGN KEY (relation_id)
        REFERENCES entity_relations(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_counter_rolls_character FOREIGN KEY (character_id)
        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_shop_counter_rolls_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
