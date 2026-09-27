-- Calendar V5 "game nights" (issue #741, Part C): real-world calendar feeds,
-- per-occurrence RSVPs on a repeating session, and the "suggest another time"
-- email-link flow. Idempotent (ADD COLUMN IF NOT EXISTS / CREATE TABLE IF NOT
-- EXISTS) per CLAUDE.md's migration-safety rules.

-- The organizer's IANA zone the session's wall-clock time was set in. Nullable
-- — every row that predates this migration has no zone, same as it does today
-- (ScheduledTime is already zone-less); a viewer without their own stored zone
-- falls back to this rather than UTC or the calendar's own zone (a session is
-- not necessarily tied to one calendar row).
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS scheduled_tz VARCHAR(64) DEFAULT NULL AFTER scheduled_time;

-- Soft delete so a cancelled/deleted game night is restorable (a co-Director
-- control) instead of the previous hard DELETE ... CASCADE, which threw away
-- attendee history the moment a session was removed.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS deleted_at DATETIME DEFAULT NULL;

-- Attendee note + the count/recheck flags for a NON-recurring session's single
-- attendee row.
--   note                — a short player-visible note on their own RSVP
--                          ("running 15 late"); NULL means no note (an empty
--                          string is also stored as NULL — see
--                          sessionRepository.SetAttendeeNote's doc comment).
--   excluded_from_count — the Director's own "leave myself out of the N/M
--                          tally" switch (operator answer #3). Only ever set
--                          on the caller's own row (enforced in the service).
--   needs_recheck       — set when the session moves or is cancelled AFTER
--                          this member had already answered; the answer is
--                          KEPT, never cleared, per the operator's explicit
--                          instruction, and this flag just asks them to look
--                          again.
ALTER TABLE session_attendees
  ADD COLUMN IF NOT EXISTS note VARCHAR(140) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS excluded_from_count TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS needs_recheck TINYINT(1) NOT NULL DEFAULT 0;

-- Per-occurrence RSVPs for a REPEATING session. A non-recurring session keeps
-- using session_attendees unchanged (zero behavior change there); a recurring
-- session's answers live here instead, one row per (series, member, specific
-- night) — "keyed per occurrence" per the operator's server-change comment #2.
-- Occurrence dates are virtual (computed from the session's recurrence
-- pattern, never stored on `sessions` itself), so this table's UNIQUE key is
-- what gives each night its own independent answer.
CREATE TABLE IF NOT EXISTS session_occurrence_rsvps (
    id                  INT AUTO_INCREMENT PRIMARY KEY,
    session_id          CHAR(36)     NOT NULL,
    user_id             CHAR(36)     NOT NULL,
    occurrence_date     DATE         NOT NULL,
    status              VARCHAR(20)  NOT NULL DEFAULT 'invited',
    note                VARCHAR(140) DEFAULT NULL,
    excluded_from_count TINYINT(1)   NOT NULL DEFAULT 0,
    needs_recheck       TINYINT(1)   NOT NULL DEFAULT 0,
    responded_at        DATETIME     DEFAULT NULL,

    CONSTRAINT fk_session_occ_rsvp_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_occ_rsvp_user    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE,
    UNIQUE KEY uq_session_occurrence_rsvp (session_id, user_id, occurrence_date),
    INDEX idx_session_occ_rsvp_session_date (session_id, occurrence_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- "Suggest another time" — the third RSVP email-link fix (issue #741): unlike
-- the accept/decline/tentative tokens, a suggestion does not resolve to one of
-- three fixed outcomes, so it gets its own small record rather than trying to
-- force a free-text date/time into session_rsvp_tokens.action. occurrence_date
-- is set only when the suggestion is against one night of a recurring series.
CREATE TABLE IF NOT EXISTS session_reschedule_suggestions (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    session_id      CHAR(36)     NOT NULL,
    user_id         CHAR(36)     NOT NULL,
    occurrence_date DATE         DEFAULT NULL,
    suggested_date  DATE         NOT NULL,
    suggested_time  VARCHAR(5)   DEFAULT NULL,
    note            VARCHAR(140) DEFAULT NULL,
    created_at      DATETIME     NOT NULL,

    CONSTRAINT fk_session_suggest_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_suggest_user    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One private, replaceable calendar-feed credential per campaign member
-- (operator answer #1: "Members get a private link that puts game nights in
-- their own calendar app"). Replacing deletes-then-recreates so a leaked link
-- stops resolving immediately, with no key-rotation window.
CREATE TABLE IF NOT EXISTS session_calendar_feed_tokens (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    user_id     CHAR(36) NOT NULL,
    token       CHAR(43) NOT NULL,
    created_at  DATETIME NOT NULL,

    CONSTRAINT fk_session_feed_token_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_feed_token_user     FOREIGN KEY (user_id)     REFERENCES users(id)      ON DELETE CASCADE,
    UNIQUE KEY uq_session_feed_token (token),
    UNIQUE KEY uq_session_feed_member (campaign_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Campaign-wide owner kill switch. ABSENCE OF A ROW MEANS ENABLED — matching
-- this codebase's "absence is a first-class state" convention (see
-- member_availability_status) — so the feed is on for every campaign the
-- moment this migration lands (operator answer #1), and a row is only ever
-- inserted when an owner explicitly flips it off.
CREATE TABLE IF NOT EXISTS session_calendar_feed_settings (
    campaign_id CHAR(36) PRIMARY KEY,
    enabled     TINYINT(1) NOT NULL DEFAULT 1,

    CONSTRAINT fk_session_feed_settings_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
