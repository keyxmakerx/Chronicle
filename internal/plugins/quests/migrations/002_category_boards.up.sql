-- 002_category_boards: notice boards can live on a category page (an entity
-- type's dashboard) as well as on a single page.
--
-- A board has exactly one home: entity_id (a page) or entity_type_id (a
-- category). The service enforces "exactly one"; entity_id becomes nullable
-- so a category board needs no placeholder page. quest_board_type_looks
-- mirrors quest_board_pages for the category home.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE quest_boards
  MODIFY COLUMN entity_id CHAR(36) NULL,
  ADD COLUMN IF NOT EXISTS entity_type_id INT NULL AFTER entity_id,
  ADD KEY IF NOT EXISTS idx_quest_boards_type (entity_type_id, sort_order);

ALTER TABLE quest_boards
  ADD CONSTRAINT fk_quest_boards_entity_type
    FOREIGN KEY IF NOT EXISTS (entity_type_id) REFERENCES entity_types(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS quest_board_type_looks (
    entity_type_id INT         NOT NULL,
    campaign_id    CHAR(36)    NOT NULL,
    board_look     VARCHAR(16) NOT NULL DEFAULT 'lit',
    ledger_look    VARCHAR(16) NOT NULL DEFAULT 'lit',
    PRIMARY KEY (entity_type_id),
    CONSTRAINT fk_quest_board_type_looks_type     FOREIGN KEY (entity_type_id) REFERENCES entity_types(id) ON DELETE CASCADE,
    CONSTRAINT fk_quest_board_type_looks_campaign FOREIGN KEY (campaign_id)    REFERENCES campaigns(id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
