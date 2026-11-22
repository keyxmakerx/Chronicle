-- 031_event_outcome — what became of an event, and whether the calendar asks.
--
-- calendar_events.status: 'planned', 'happened', 'moved' or 'called_off'.
-- Every event starts planned and an unanswered one stays planned, which
-- reads as happened once its day has passed. Validated in Go, so the four
-- words stay one list the page, the campaign AI and Foundry share.
--
-- calendar_events.outcome: the Director's line on what happened, plain
-- text, or NULL. It is part of the event, so it follows the event's
-- visibility.
--
-- calendar_events.moved_from_year/_month/_day: the day a moved event was
-- planned for, so the calendar keeps a mark there. NULL unless status is
-- 'moved'.
--
-- calendars.ask_when_day_passes: whether moving today past events opens
-- the sheet that asks what became of them. On by default.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendar_events
  ADD COLUMN IF NOT EXISTS status           VARCHAR(12) NOT NULL DEFAULT 'planned',
  ADD COLUMN IF NOT EXISTS outcome          TEXT        DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS moved_from_year  INT         DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS moved_from_month INT         DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS moved_from_day   INT         DEFAULT NULL;

ALTER TABLE calendars
  ADD COLUMN IF NOT EXISTS ask_when_day_passes TINYINT(1) NOT NULL DEFAULT 1;
