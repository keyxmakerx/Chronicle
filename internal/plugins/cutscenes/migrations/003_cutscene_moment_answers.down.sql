-- 003_cutscene_moment_answers down: drop the answers and the released column.
DROP TABLE IF EXISTS cutscene_moment_answers;
ALTER TABLE cutscene_showings DROP COLUMN IF EXISTS released;
