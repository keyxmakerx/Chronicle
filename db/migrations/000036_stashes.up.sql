-- Stashes and item/money moves for the armory plugin.
--
-- A stash is a named, campaign-wide holding place (party chest, hidden cache)
-- that the GM shows to chosen characters; a stash with no viewers is GM-only.
-- item_moves is both the history and the queue of requests waiting for the GM:
-- a move that needs approval is a row with status 'pending'. campaign_downtime
-- holds the switch that decides whether moves apply at once or wait; a missing
-- row means downtime is closed.
--
-- created_by / requested_by / decided_by are deliberately not foreign keys:
-- the history must outlive the account that made a move.
-- Core tables only (campaigns, entities); core migration.

CREATE TABLE IF NOT EXISTS stashes (
    id          INT           AUTO_INCREMENT PRIMARY KEY,
    campaign_id VARCHAR(36)   NOT NULL,
    name        VARCHAR(120)  NOT NULL,
    location    VARCHAR(200)  DEFAULT NULL,
    money       DECIMAL(14,2) NOT NULL DEFAULT 0,
    created_by  VARCHAR(36)   DEFAULT NULL,
    created_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uq_stashes_campaign_name (campaign_id, name),
    CONSTRAINT chk_stashes_money CHECK (money >= 0),
    CONSTRAINT fk_stashes_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS stash_viewers (
    stash_id            INT         NOT NULL,
    character_entity_id VARCHAR(36) NOT NULL,

    PRIMARY KEY (stash_id, character_entity_id),
    INDEX idx_stash_viewers_character (character_entity_id),
    CONSTRAINT fk_stash_viewers_stash FOREIGN KEY (stash_id)
        REFERENCES stashes(id) ON DELETE CASCADE,
    CONSTRAINT fk_stash_viewers_entity FOREIGN KEY (character_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS stash_items (
    stash_id       INT         NOT NULL,
    item_entity_id VARCHAR(36) NOT NULL,
    quantity       INT         NOT NULL,

    PRIMARY KEY (stash_id, item_entity_id),
    CONSTRAINT chk_stash_items_quantity CHECK (quantity > 0),
    CONSTRAINT fk_stash_items_stash FOREIGN KEY (stash_id)
        REFERENCES stashes(id) ON DELETE CASCADE,
    CONSTRAINT fk_stash_items_entity FOREIGN KEY (item_entity_id)
        REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS item_moves (
    id             BIGINT        AUTO_INCREMENT PRIMARY KEY,
    campaign_id    VARCHAR(36)   NOT NULL,
    kind           ENUM('item','money') NOT NULL,
    item_entity_id VARCHAR(36)   DEFAULT NULL,
    quantity       INT           DEFAULT NULL,
    amount         DECIMAL(14,2) DEFAULT NULL,
    from_kind      ENUM('character','stash') NOT NULL,
    from_id        VARCHAR(36)   NOT NULL,
    to_kind        ENUM('character','stash') NOT NULL,
    to_id          VARCHAR(36)   NOT NULL,
    status         ENUM('applied','pending','declined','failed') NOT NULL,
    reason         VARCHAR(255)  DEFAULT NULL,
    requested_by   VARCHAR(36)   NOT NULL,
    decided_by     VARCHAR(36)   DEFAULT NULL,
    created_at     TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at     TIMESTAMP     NULL DEFAULT NULL,

    INDEX idx_item_moves_campaign_created (campaign_id, created_at),
    INDEX idx_item_moves_campaign_status (campaign_id, status),
    CONSTRAINT fk_item_moves_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS campaign_downtime (
    campaign_id VARCHAR(36) NOT NULL PRIMARY KEY,
    is_open     BOOLEAN     NOT NULL DEFAULT FALSE,
    changed_by  VARCHAR(36) DEFAULT NULL,
    changed_at  TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_campaign_downtime_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
