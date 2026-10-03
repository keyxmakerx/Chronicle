-- 022_event_recurrence_rules — events that repeat by a rule, and per-occurrence
-- overrides for every repeating event.
--
-- recurrence_rule: the rule an event with recurrence_type 'rule' repeats by
-- ({"match":[...conditions], "every":N, "offset_days":K}), validated in Go.
-- NULL for every other event; existing rows are untouched.
--
-- calendar_event_overrides: "this one only" — one row per occurrence of a
-- repeating event that the Director skipped or moved, keyed by the date the
-- rule put it on. A move carries the new date in new_*; a skip leaves them
-- NULL (application code keeps the three all set or all NULL). Deleting the
-- event deletes its overrides.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendar_events
  ADD COLUMN IF NOT EXISTS recurrence_rule JSON DEFAULT NULL;

CREATE TABLE IF NOT EXISTS calendar_event_overrides (
    event_id         VARCHAR(36)          NOT NULL,
    occurrence_year  INT                  NOT NULL,
    occurrence_month INT                  NOT NULL,
    occurrence_day   INT                  NOT NULL,
    action           ENUM('skip', 'move') NOT NULL,
    new_year         INT                  DEFAULT NULL,
    new_month        INT                  DEFAULT NULL,
    new_day          INT                  DEFAULT NULL,
    created_at       DATETIME             NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (event_id, occurrence_year, occurrence_month, occurrence_day),
    CONSTRAINT fk_cal_event_overrides_event FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
