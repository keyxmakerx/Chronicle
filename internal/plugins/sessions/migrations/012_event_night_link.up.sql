-- A game night made from a calendar Session event remembers that event, and
-- "Not this one" on the Game nights strip is kept per campaign.
--
-- sessions.calendar_event_id is the event the night stands for. NULL for every
-- night made any other way, and for every night made before this column
-- existed: it is set at creation only, never backfilled (schema-only
-- migration), so the strip still matches those older nights by name or day.
-- The UNIQUE index is what closes the make-a-night race: two requests for one
-- event can both pass the "no night yet" read, but only one insert gets the
-- key. A night in the Trash keeps its link (the row stays), so its event is
-- not offered again until the night is restored or the Trash is emptied.
-- Deleting the event unlinks the night instead of deleting it.
--
-- session_event_dismissals holds the events whose strip row someone chose to
-- hide, one row per campaign and event, shared by everyone who sees the strip.
-- Deleting the event or the campaign removes the row.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.

ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS calendar_event_id VARCHAR(36) NULL DEFAULT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_sessions_calendar_event ON sessions (calendar_event_id);

ALTER TABLE sessions
  DROP FOREIGN KEY IF EXISTS fk_sessions_calendar_event;

ALTER TABLE sessions
  ADD CONSTRAINT fk_sessions_calendar_event
    FOREIGN KEY (calendar_event_id) REFERENCES calendar_events(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS session_event_dismissals (
    campaign_id       CHAR(36)    NOT NULL,
    calendar_event_id VARCHAR(36) NOT NULL,
    dismissed_by      CHAR(36)    DEFAULT NULL,
    created_at        DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, calendar_event_id),
    CONSTRAINT fk_session_event_dismissal_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_event_dismissal_event    FOREIGN KEY (calendar_event_id) REFERENCES calendar_events(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_event_dismissal_user     FOREIGN KEY (dismissed_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
