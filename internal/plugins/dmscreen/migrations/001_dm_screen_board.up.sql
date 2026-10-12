-- 001_dm_screen_board: what the DM Screen's Adventures tab holds: Adventures
-- pages pinned to it and short loose notes.
--
-- One table for both, told apart by kind: a pin carries entity_id, a note
-- carries body. A page can be pinned once per campaign (the unique key; notes
-- leave entity_id NULL, which the key allows to repeat). Items show oldest
-- first. Deleting a page or a campaign removes its pins; a trashed page keeps
-- its pin, which the screen hides until the page is restored. created_by has
-- no foreign key on purpose: it only records who added the item and must not
-- block removing a user. Core tables run before plugin ones, and the collation
-- matches them, so the foreign keys are safe.
CREATE TABLE IF NOT EXISTS dm_screen_board_items (
    id           CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    kind         VARCHAR(8)   NOT NULL,
    entity_id    CHAR(36)     NULL,
    body         VARCHAR(600) NULL,
    created_by   CHAR(36)     NULL,
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_dm_board_pin (campaign_id, entity_id),
    KEY idx_dm_board_campaign (campaign_id, created_at),
    CONSTRAINT fk_dm_board_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_dm_board_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
