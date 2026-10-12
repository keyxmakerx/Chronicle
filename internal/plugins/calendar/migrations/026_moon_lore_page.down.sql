-- 026_moon_lore_page (down) — drop the moon's lore page link. Destroys which
-- page each moon pointed at; the pages themselves are untouched. Idempotent.
ALTER TABLE calendar_moons
  DROP FOREIGN KEY IF EXISTS fk_cal_moons_lore_entity;

ALTER TABLE calendar_moons
  DROP INDEX IF EXISTS fk_cal_moons_lore_entity,
  DROP COLUMN IF EXISTS lore_entity_id;
