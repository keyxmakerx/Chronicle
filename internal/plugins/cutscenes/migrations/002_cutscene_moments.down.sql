-- 002_cutscene_moments down: drop the moments column.
ALTER TABLE cutscenes DROP COLUMN IF EXISTS moments;
