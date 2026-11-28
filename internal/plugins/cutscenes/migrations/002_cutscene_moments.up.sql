-- 002_cutscene_moments: a scene's moments, the beats where it stops and asks
-- a question. NULL for a scene without any, so every existing row reads as
-- it did. A showing keeps its scene whole in cutscene_showings.scene, which
-- needs no change.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE cutscenes ADD COLUMN IF NOT EXISTS moments TEXT NULL AFTER `text`;
