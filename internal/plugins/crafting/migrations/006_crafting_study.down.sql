DROP TABLE IF EXISTS crafting_studies;
DROP TABLE IF EXISTS crafting_known_items;
ALTER TABLE crafting_spoil_lines
    DROP COLUMN IF EXISTS study_once,
    DROP COLUMN IF EXISTS study_stat,
    DROP COLUMN IF EXISTS study;
