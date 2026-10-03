-- 024_era_look — how eras look behind the month, and what an era tells.
--
-- calendars: the whole calendar's era colours. era_colors_on switches the
-- tint behind the days off; era_feel is the named feel ('still', 'subtle',
-- 'lively', or 'custom' when the owner fine-tuned it), and era_intensity /
-- era_speed are the numbers that feel stands for, so a custom feel needs no
-- second shape. Validated in Go.
--
-- calendar_eras:
--   color_2             the era's second colour; NULL draws the era in its
--                       first colour alone.
--   style               how the two colours mix: 'gas' or 'ink'.
--   feel                the era's own feel ('still', 'subtle', 'lively');
--                       NULL follows the calendar's.
--   lore_entity_id      a page about the era. Deleting the page unlinks it.
--   dm_note             the Director's private note; never sent to a player.
--   hidden_until_begins players learn nothing of the era until the
--                       calendar's current date reaches its start.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendars
  ADD COLUMN IF NOT EXISTS era_colors_on TINYINT(1)  NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS era_feel      VARCHAR(10) NOT NULL DEFAULT 'subtle',
  ADD COLUMN IF NOT EXISTS era_intensity FLOAT       NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS era_speed     FLOAT       NOT NULL DEFAULT 1;

ALTER TABLE calendar_eras
  ADD COLUMN IF NOT EXISTS color_2             VARCHAR(20) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS style               VARCHAR(8)  NOT NULL DEFAULT 'gas',
  ADD COLUMN IF NOT EXISTS feel                VARCHAR(10) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS lore_entity_id      VARCHAR(36) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS dm_note             TEXT        DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS hidden_until_begins TINYINT(1)  NOT NULL DEFAULT 0;

ALTER TABLE calendar_eras
  ADD CONSTRAINT fk_cal_eras_lore_entity
    FOREIGN KEY IF NOT EXISTS (lore_entity_id) REFERENCES entities(id) ON DELETE SET NULL;
