-- Wave 1.7A §G: per-user sidebar pin preference. Extends the existing
-- calendar_active table (PR #363) rather than introducing a new
-- calendar_v2_user_prefs table per coordinator decision (PR #368
-- stop-and-flag #3): simpler; reuses the existing (user_id,
-- campaign_id) PK + FK cascade + index discipline.
--
-- Default TRUE: pin on creation matches the viewport-aware default
-- (≥1024px viewports show the sidebar pinned by default; operators
-- on narrower viewports collapse via the toggle, which writes FALSE).
-- Backfilling existing rows with TRUE matches the same heuristic.

ALTER TABLE calendar_active
  ADD COLUMN sidebar_pinned BOOLEAN NOT NULL DEFAULT TRUE;
