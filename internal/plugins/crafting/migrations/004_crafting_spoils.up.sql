-- 004_crafting_spoils: what pages drop, and the party's spoils tray.
--
-- A drop list belongs to one page: a creature, a Remnant, a place, a ruined
-- item. It may name one other page whose list drops too (a Remnant's
-- original), one step only. Its lines are JSON, as a recipe's materials are.
CREATE TABLE IF NOT EXISTS crafting_drops (
    campaign_id  CHAR(36)  NOT NULL,
    page_id      CHAR(36)  NOT NULL,
    also_page_id CHAR(36)  NULL,
    drop_lines   TEXT      NOT NULL,
    updated_by   CHAR(36)  NULL,
    updated_at   DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version      INT       NOT NULL DEFAULT 1,
    PRIMARY KEY (campaign_id, page_id),
    CONSTRAINT fk_crafting_drops_campaign FOREIGN KEY (campaign_id)  REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_drops_page     FOREIGN KEY (page_id)      REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_crafting_drops_also     FOREIGN KEY (also_page_id) REFERENCES entities(id)  ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One row per thing that dropped: a defeated copy of a creature when a fight
-- ended, or a page's list the Director put in by hand. The page's name is
-- kept so the tray still reads when the page is deleted.
CREATE TABLE IF NOT EXISTS crafting_spoils (
    id          CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    page_id     CHAR(36)     NULL,
    page_name   VARCHAR(200) NOT NULL,
    copy_no     INT          NOT NULL DEFAULT 1,
    copies      INT          NOT NULL DEFAULT 1,
    origin      VARCHAR(200) NOT NULL DEFAULT '',
    created_by  CHAR(36)     NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_crafting_spoils_campaign (campaign_id, created_at),
    CONSTRAINT fk_crafting_spoils_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_spoils_page     FOREIGN KEY (page_id)     REFERENCES entities(id)  ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Each line of a spoil, copied from the drop list when it dropped, so
-- editing the list later never changes spoils already waiting. A line is
-- waiting, taken (by which hero, how many, and the test's total for a
-- harvest) or cleared by the Director; taking is a guarded status change,
-- so two players can't both take one line. A line whose item is deleted
-- goes with it.
CREATE TABLE IF NOT EXISTS crafting_spoil_lines (
    id           BIGINT       NOT NULL AUTO_INCREMENT,
    spoil_id     CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    item_id      CHAR(36)     NOT NULL,
    item_name    VARCHAR(200) NOT NULL,
    mode         VARCHAR(12)  NOT NULL,
    qty          INT          NOT NULL DEFAULT 1,
    stat         VARCHAR(40)  NOT NULL DEFAULT '',
    yield_low    INT          NOT NULL DEFAULT 0,
    yield_mid    INT          NOT NULL DEFAULT 0,
    yield_high   INT          NOT NULL DEFAULT 0,
    status       VARCHAR(12)  NOT NULL DEFAULT 'waiting',
    character_id CHAR(36)     NULL,
    taken_by     CHAR(36)     NULL,
    taken_qty    INT          NOT NULL DEFAULT 0,
    rolled       INT          NULL,
    taken_at     DATETIME     NULL,
    PRIMARY KEY (id),
    KEY idx_crafting_spoil_lines_spoil (spoil_id, id),
    KEY idx_crafting_spoil_lines_status (campaign_id, status, taken_at),
    CONSTRAINT fk_crafting_spoil_lines_spoil     FOREIGN KEY (spoil_id)     REFERENCES crafting_spoils(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_spoil_lines_campaign  FOREIGN KEY (campaign_id)  REFERENCES campaigns(id)       ON DELETE CASCADE,
    CONSTRAINT fk_crafting_spoil_lines_item      FOREIGN KEY (item_id)      REFERENCES entities(id)        ON DELETE CASCADE,
    CONSTRAINT fk_crafting_spoil_lines_character FOREIGN KEY (character_id) REFERENCES entities(id)        ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
