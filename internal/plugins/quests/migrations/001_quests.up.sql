-- 001_quests: quest sheets, and the notice boards pinned on place pages.
--
-- A quest sheet is one JSON document per page, validated by the service, so
-- its shape can grow without a schema change; version guards concurrent DM
-- edits. Board items keep ref_id without a foreign key: it points at a page,
-- a quest or a map, and a vanished target is simply dropped when the board is
-- read. Core tables run before plugin ones, and the collation matches them,
-- so the foreign keys are safe.
CREATE TABLE IF NOT EXISTS quests (
    entity_id    CHAR(36)  NOT NULL,
    campaign_id  CHAR(36)  NOT NULL,
    data         LONGTEXT  NOT NULL,
    version      INT       NOT NULL DEFAULT 1,
    updated_by   CHAR(36)  NULL,
    updated_at   DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (entity_id),
    CONSTRAINT fk_quests_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_quests_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS quest_board_pages (
    entity_id    CHAR(36)    NOT NULL,
    campaign_id  CHAR(36)    NOT NULL,
    board_look   VARCHAR(16) NOT NULL DEFAULT 'lit',
    ledger_look  VARCHAR(16) NOT NULL DEFAULT 'lit',
    PRIMARY KEY (entity_id),
    CONSTRAINT fk_quest_board_pages_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_quest_board_pages_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS quest_boards (
    id           CHAR(36)    NOT NULL,
    campaign_id  CHAR(36)    NOT NULL,
    entity_id    CHAR(36)    NOT NULL,
    name         VARCHAR(80) NOT NULL,
    who          ENUM('dm','scribe','all') NOT NULL DEFAULT 'dm',
    sort_order   INT         NOT NULL DEFAULT 0,
    created_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_quest_boards_entity (entity_id, sort_order),
    CONSTRAINT fk_quest_boards_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_quest_boards_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS quest_board_items (
    id             CHAR(36)     NOT NULL,
    board_id       CHAR(36)     NOT NULL,
    campaign_id    CHAR(36)     NOT NULL,
    kind           VARCHAR(16)  NOT NULL,
    x              DOUBLE       NOT NULL DEFAULT 0,
    y              DOUBLE       NOT NULL DEFAULT 0,
    w              DOUBLE       NOT NULL DEFAULT 20,
    r              DOUBLE       NOT NULL DEFAULT 0,
    owner_user_id  CHAR(36)     NULL,
    by_dm          TINYINT(1)   NOT NULL DEFAULT 0,
    hidden         TINYINT(1)   NOT NULL DEFAULT 0,
    text           VARCHAR(400) NULL,
    ref_id         CHAR(36)     NULL,
    from_item_id   CHAR(36)     NULL,
    to_item_id     CHAR(36)     NULL,
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_quest_board_items_board (board_id),
    CONSTRAINT fk_quest_board_items_board    FOREIGN KEY (board_id)    REFERENCES quest_boards(id) ON DELETE CASCADE,
    CONSTRAINT fk_quest_board_items_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
