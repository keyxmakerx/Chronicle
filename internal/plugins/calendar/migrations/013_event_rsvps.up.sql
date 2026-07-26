-- C-CAL-RSVP-P1: first-class RSVPs on CALENDAR EVENTS.
--
-- Distinct from sessions' attendance (session_attendees / session_rsvp_tokens).
-- A calendar event is not a session: it may be a festival, a downtime window, or
-- a one-off scene. The operator's #1 ask is Outlook/Google-style "who's coming"
-- on the calendar itself, so RSVP state belongs to the EVENT aggregate and lives
-- in calendar-owned tables. We MIRROR the sessions token PATTERN (single-use,
-- expiring, opaque DB-stored token) into a DISTINCT table rather than reusing
-- session_rsvp_tokens — the same precedent slot_proposal_tokens set in
-- sessions/migrations/003_proposals.up.sql ("mirror the PATTERN, distinct
-- table"). Reusing sessions' table would put a calendar FK on a sessions row and
-- couple two plugins' schemas.
--
-- Tokens are OPAQUE and DB-stored (crypto/rand hex, house pattern), NOT
-- HMAC-signed: revocation and single-use are properties of the row, so a used or
-- deleted row is dead immediately with no key rotation to reason about.

-- One row per (event, user). Status is the member's current answer; re-answering
-- UPDATEs in place (see the UNIQUE key) so there is no response history to leak
-- and no way to inflate a count by answering twice.
CREATE TABLE IF NOT EXISTS calendar_event_rsvps (
    id         CHAR(36)                        PRIMARY KEY,
    event_id   CHAR(36)                        NOT NULL,
    user_id    CHAR(36)                        NOT NULL,
    status     ENUM('yes','maybe','no')        NOT NULL,
    note       TEXT                            DEFAULT NULL,
    updated_at DATETIME                        NOT NULL,

    CONSTRAINT fk_cal_event_rsvps_event FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE,
    CONSTRAINT fk_cal_event_rsvps_user  FOREIGN KEY (user_id)  REFERENCES users(id)           ON DELETE CASCADE,
    UNIQUE KEY uq_cal_event_rsvp (event_id, user_id),
    INDEX idx_cal_event_rsvps_event (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One row per emailed action link. `action` carries the intent the link encodes,
-- so the recipient never picks a target: 'out_week' and 'suggest' are the two
-- richer actions the operator asked for on top of yes/maybe/no.
--
-- used_at + expires_at make every link single-use and time-boxed (7 days, same
-- as sessions). ON DELETE CASCADE on both FKs means deleting the event or the
-- user invalidates every outstanding link for free.
CREATE TABLE IF NOT EXISTS calendar_event_rsvp_tokens (
    id         INT AUTO_INCREMENT                              PRIMARY KEY,
    token      CHAR(64)                                        NOT NULL,
    event_id   CHAR(36)                                        NOT NULL,
    user_id    CHAR(36)                                        NOT NULL,
    action     ENUM('yes','maybe','no','out_week','suggest')   NOT NULL,
    used_at    DATETIME                                        DEFAULT NULL,
    expires_at DATETIME                                        NOT NULL,
    created_at DATETIME                                        NOT NULL,

    CONSTRAINT fk_cal_rsvp_tokens_event FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE,
    CONSTRAINT fk_cal_rsvp_tokens_user  FOREIGN KEY (user_id)  REFERENCES users(id)           ON DELETE CASCADE,
    UNIQUE KEY uq_cal_rsvp_token (token),
    INDEX idx_cal_rsvp_tokens_event_user (event_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Per-event opt-in. Collection is OFF by default so no existing event starts
-- soliciting responses (and no existing event triggers an email fan-out) the
-- moment this migration lands.
ALTER TABLE calendar_events
  ADD COLUMN IF NOT EXISTS collect_rsvps TINYINT(1) NOT NULL DEFAULT 0;
