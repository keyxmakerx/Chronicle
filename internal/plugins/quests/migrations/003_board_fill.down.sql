-- 003_board_fill (down): boards go back to hand-pinned only. Notices a board
-- placed itself stay, as ordinary hand-pinned notices. Idempotent.
ALTER TABLE quest_board_items
  DROP INDEX IF EXISTS uq_quest_board_items_auto,
  DROP COLUMN IF EXISTS auto_ref;

ALTER TABLE quest_boards
  DROP COLUMN IF EXISTS fill;
