-- 029_moon_marks — a moon the Director sets by hand.
--
-- calendar_moons.phase_mode: 'cycle' (the moon follows cycle_days and
-- phase_offset, as every moon did before) or 'manual' (the moon has no
-- cycle; it holds whatever state the Director last set). cycle_days and
-- phase_offset are kept while a moon is manual, so switching back restores
-- its cycle.
--
-- calendar_moons.state_names: the Director's own words for a manual moon's
-- states as a JSON object ({"vanished": "Gone below"}), NULL for the
-- defaults.
--
-- calendar_moon_marks: one row per day the Director set a manual moon. A
-- mark holds from its day until the next one. shown says whether players
-- see that change; who sees which mark is a service rule. Marks cascade
-- with the moon.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendar_moons
  ADD COLUMN IF NOT EXISTS phase_mode VARCHAR(10) NOT NULL DEFAULT 'cycle';

ALTER TABLE calendar_moons
  ADD COLUMN IF NOT EXISTS state_names JSON DEFAULT NULL;

CREATE TABLE IF NOT EXISTS calendar_moon_marks (
    moon_id     INT          NOT NULL,
    year        INT          NOT NULL,
    month       INT          NOT NULL,
    day         INT          NOT NULL,
    state       VARCHAR(10)  NOT NULL,
    shown       BOOLEAN      NOT NULL DEFAULT FALSE,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (moon_id, year, month, day),
    CONSTRAINT fk_cal_moon_marks_moon FOREIGN KEY (moon_id) REFERENCES calendar_moons(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
