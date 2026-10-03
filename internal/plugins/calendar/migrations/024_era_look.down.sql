-- 024_era_look (down) — drop the era look settings and the era's second
-- colour, style, feel, lore page, Director's note and hidden flag. Destroys
-- what was stored in them; every era then shows in its one colour to
-- everyone, including one that was hidden until it begins. Idempotent.
ALTER TABLE calendar_eras
  DROP FOREIGN KEY IF EXISTS fk_cal_eras_lore_entity;

ALTER TABLE calendar_eras
  DROP INDEX IF EXISTS fk_cal_eras_lore_entity,
  DROP COLUMN IF EXISTS hidden_until_begins,
  DROP COLUMN IF EXISTS dm_note,
  DROP COLUMN IF EXISTS lore_entity_id,
  DROP COLUMN IF EXISTS feel,
  DROP COLUMN IF EXISTS style,
  DROP COLUMN IF EXISTS color_2;

ALTER TABLE calendars
  DROP COLUMN IF EXISTS era_speed,
  DROP COLUMN IF EXISTS era_intensity,
  DROP COLUMN IF EXISTS era_feel,
  DROP COLUMN IF EXISTS era_colors_on;
