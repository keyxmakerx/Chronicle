-- 003_board_fill: a board can fill itself with the campaign's quests.
--
-- quest_boards.fill lists the quest statuses the board shows, comma
-- separated; empty means the DM pins every notice by hand. A notice the board
-- placed itself carries auto_ref (the quest page id), and the unique key keeps
-- two saves landing at once from pinning the same quest twice. NULLs never
-- collide, so hand-pinned items are unaffected.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE quest_boards
  ADD COLUMN IF NOT EXISTS fill VARCHAR(64) NOT NULL DEFAULT '' AFTER who;

ALTER TABLE quest_board_items
  ADD COLUMN IF NOT EXISTS auto_ref CHAR(36) NULL AFTER ref_id,
  ADD UNIQUE KEY IF NOT EXISTS uq_quest_board_items_auto (board_id, auto_ref);
