-- 022_event_recurrence_rules (down) — drop per-occurrence overrides and the
-- rule column. Destroys every stored override and rule; an event left with
-- recurrence_type 'rule' then repeats nowhere and shows on its own date only.
-- Idempotent.
DROP TABLE IF EXISTS calendar_event_overrides;

ALTER TABLE calendar_events
  DROP COLUMN IF EXISTS recurrence_rule;
