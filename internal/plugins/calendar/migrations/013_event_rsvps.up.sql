-- C-CAL-RSVP-P1: first-class RSVPs on calendar events. A calendar event is NOT
-- a session (sessions live in the sessions plugin with their own attendee +
-- token tables); the operator wants members to RSVP directly to a calendar
-- event, in-app and via emailed links, with the Director seeing counts. This
-- supersedes the old drawer ruling (calendar_v2.templ) that RSVPs required an
-- event<->session link — see .ai/decisions.md.
--
-- Two NEW tables, PATTERN-mirrored on the sessions plugin's RSVP rails
-- (session_rsvp_tokens, sessions/migrations/001_session_tables.up.sql:60-73) and
-- the slot_proposal_tokens precedent (sessions/migrations/003_proposals.up.sql:
-- 70-84) — "mirror the PATTERN, distinct table". We deliberately do NOT reuse
-- the sessions tables: RSVP data on a calendar event is its own aggregate and
-- must stay out of the sessions/campaign/AI export egress by construction
-- (see the calendar-side egress pin test, rsvp_egress_test.go).
--
-- Plugin-scoped migration (runs after core). References calendar_events (this
-- plugin's own table, migration 001) and users (a CORE table, baseline) — both
-- exist by migration 013, so the FKs are safe on a fresh DB. Schema-only,
-- idempotent, append-only per the migration-safety rules.

-- One row per (event, user): re-responding UPSERTs (UNIQUE(event_id, user_id)).
-- status yes/maybe/no; note carries the free-text from "Suggest another time".
CREATE TABLE IF NOT EXISTS calendar_event_rsvps (
    id         INT                        NOT NULL AUTO_INCREMENT PRIMARY KEY,
    event_id   VARCHAR(36)                NOT NULL,
    user_id    CHAR(36)                   NOT NULL,
    status     ENUM('yes','maybe','no')   NOT NULL,
    note       TEXT                       DEFAULT NULL,
    updated_at DATETIME                   NOT NULL,

    UNIQUE KEY uq_calendar_event_rsvp (event_id, user_id),
    INDEX idx_calendar_event_rsvps_event (event_id),
    INDEX idx_calendar_event_rsvps_user (user_id),
    CONSTRAINT fk_calendar_event_rsvps_event
      FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE,
    CONSTRAINT fk_calendar_event_rsvps_user
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One-click email response tokens (mirrors session_rsvp_tokens as a PATTERN).
-- The token is a DB-stored OPAQUE credential (crypto/rand hex, NOT HMAC-signed —
-- the house pattern), encoding a single (event, user, action). action carries
-- the extra email-only verbs beyond the yes/maybe/no RSVP set: out_week (RSVP
-- no + mark the responder's real week unavailable) and suggest (free-text →
-- RSVP note + notify the event owner). Single-use (used_at) + expiring
-- (expires_at, 7 days).
CREATE TABLE IF NOT EXISTS calendar_event_rsvp_tokens (
    id         INT                                              NOT NULL AUTO_INCREMENT PRIMARY KEY,
    token      CHAR(64)                                         NOT NULL UNIQUE,
    event_id   VARCHAR(36)                                      NOT NULL,
    user_id    CHAR(36)                                         NOT NULL,
    action     ENUM('yes','maybe','no','out_week','suggest')    NOT NULL,
    used_at    DATETIME                                         DEFAULT NULL,
    expires_at DATETIME                                         NOT NULL,
    created_at DATETIME                                         NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_calendar_event_rsvp_token (token),
    INDEX idx_calendar_event_rsvp_tokens_event_user (event_id, user_id),
    CONSTRAINT fk_calendar_event_rsvp_tokens_event
      FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE,
    CONSTRAINT fk_calendar_event_rsvp_tokens_user
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Per-event opt-in: Scribe+ flips "Collect RSVPs" on the event editor drawer.
-- DEFAULT 0 backfills every existing event as collection-off, so nothing changes
-- on upgrade (provably zero-change when collect_rsvps=0). ADD COLUMN IF NOT
-- EXISTS keeps the migration idempotent.
ALTER TABLE calendar_events
  ADD COLUMN IF NOT EXISTS collect_rsvps TINYINT(1) NOT NULL DEFAULT 0;
