-- 002_category_boards (down): remove category homes. Boards that live on a
-- category (and, by cascade, their items) are deleted, since they have no page
-- to fall back to; the category looks table goes too. Idempotent.
DELETE FROM quest_boards WHERE entity_id IS NULL;

DROP TABLE IF EXISTS quest_board_type_looks;

ALTER TABLE quest_boards
  DROP FOREIGN KEY IF EXISTS fk_quest_boards_entity_type;

ALTER TABLE quest_boards
  DROP INDEX IF EXISTS idx_quest_boards_type,
  DROP COLUMN IF EXISTS entity_type_id,
  MODIFY COLUMN entity_id CHAR(36) NOT NULL;
