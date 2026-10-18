-- 029_moon_marks (down) — drop hand-set moons. Destroys every mark and the
-- Director's state words; a manual moon goes back to its stored cycle.
-- Idempotent.
DROP TABLE IF EXISTS calendar_moon_marks;

ALTER TABLE calendar_moons
  DROP COLUMN IF EXISTS state_names,
  DROP COLUMN IF EXISTS phase_mode;
