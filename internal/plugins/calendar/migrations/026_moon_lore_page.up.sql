-- 026_moon_lore_page — a page about a moon.
--
-- calendar_moons.lore_entity_id: an entity in the calendar's campaign that
-- tells of the moon. NULL means no page. Deleting the page unlinks it
-- rather than deleting the moon, the same as an era's lore page.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendar_moons
  ADD COLUMN IF NOT EXISTS lore_entity_id VARCHAR(36) DEFAULT NULL;

ALTER TABLE calendar_moons
  ADD CONSTRAINT fk_cal_moons_lore_entity
    FOREIGN KEY IF NOT EXISTS (lore_entity_id) REFERENCES entities(id) ON DELETE SET NULL;
