-- Availability scheduler — Slice 2 (C-SCHED-P2): slot proposals, per-option
-- responses, one-click response tokens, and a scheduler-scoped notification
-- store. Chains after 002 (availability). Idempotent (CREATE TABLE IF NOT
-- EXISTS) per the migration-safety rules.
--
-- RULING RC-12.5 / C-SCHED-AUDIT §P2:
--   * A proposal OPTION is a concrete point in time, so it is stored as a UTC
--     INSTANT (starts_at_utc / ends_at_utc, DATETIME in UTC) — NOT a zone-local
--     wall-clock. (Recurring availability is the opposite: wall-clock, see 002.)
--     The viewer's local render is derived at read time via t.In(viewerLoc).
--   * Per-option RESPONSES live in their OWN table (slot_proposal_responses) —
--     deliberately NOT session_attendees, whose RSVP status already rides
--     campaign + AI export egress. This keeps proposals/responses out of egress
--     by construction (see the own-tables egress test, extended in 0b).
--   * Response TOKENS mirror session_rsvp_tokens as a PATTERN only (one-click,
--     keyed option_id + response); they are a distinct table.
--   * NOTIFICATIONS are a generic, session-agnostic, removable store (T-B2) but
--     the FEATURE stays scheduler-scoped in this slice (no prefs, no digests,
--     no per-user websockets).

-- A DM's scheduling proposal: a title/note plus N candidate slots (options).
CREATE TABLE IF NOT EXISTS slot_proposals (
    id           CHAR(36)     PRIMARY KEY,
    campaign_id  CHAR(36)     NOT NULL,
    created_by   CHAR(36)     NOT NULL,           -- the proposing user (Scribe+)
    title        VARCHAR(200) NOT NULL,
    note         TEXT         DEFAULT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'open',  -- open | closed
    created_at   DATETIME     NOT NULL,
    updated_at   DATETIME     NOT NULL,

    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_slot_proposals_campaign (campaign_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One candidate slot on a proposal. UTC instants (RC-12.5); ordinal 1..5 fixes
-- display order; is_winner is reserved for P3's confirm flow (unused this slice).
CREATE TABLE IF NOT EXISTS slot_proposal_options (
    id            CHAR(36)   PRIMARY KEY,
    proposal_id   CHAR(36)   NOT NULL,
    starts_at_utc DATETIME   NOT NULL,            -- UTC instant
    ends_at_utc   DATETIME   NOT NULL,            -- UTC instant
    ordinal       TINYINT    NOT NULL,            -- 1..5, display order
    is_winner     TINYINT(1) NOT NULL DEFAULT 0,  -- reserved for P3 confirm

    FOREIGN KEY (proposal_id) REFERENCES slot_proposals(id) ON DELETE CASCADE,
    INDEX idx_slot_options_proposal (proposal_id, ordinal)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Per-option RSVP. Its OWN table — never session_attendees (RC-12.5). One row
-- per (option, user): re-responding upserts. yes/no/maybe mirror the RSVP
-- accepted/declined/tentative trichotomy without reusing that table.
CREATE TABLE IF NOT EXISTS slot_proposal_responses (
    id         CHAR(36)                     PRIMARY KEY,
    option_id  CHAR(36)                     NOT NULL,
    user_id    CHAR(36)                     NOT NULL,
    response   ENUM('yes','no','maybe')     NOT NULL,
    updated_at DATETIME                     NOT NULL,

    FOREIGN KEY (option_id) REFERENCES slot_proposal_options(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE KEY uq_option_user (option_id, user_id),
    INDEX idx_slot_responses_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One-click email response tokens (mirrors session_rsvp_tokens as a pattern).
-- The token encodes a single (option, user, response) so an emailed link records
-- that response with no login. Single-use (used_at) + expiring (expires_at).
CREATE TABLE IF NOT EXISTS slot_proposal_tokens (
    id          INT                       AUTO_INCREMENT PRIMARY KEY,
    token       VARCHAR(64)               NOT NULL UNIQUE,
    option_id   CHAR(36)                  NOT NULL,
    user_id     CHAR(36)                  NOT NULL,
    response    ENUM('yes','no','maybe')  NOT NULL,
    used_at     DATETIME                  DEFAULT NULL,
    expires_at  DATETIME                  NOT NULL,
    created_at  DATETIME                  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (option_id) REFERENCES slot_proposal_options(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_slot_token (token),
    INDEX idx_slot_token_option_user (option_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- In-app notification store. Schema is intentionally generic and removable
-- (T-B2): user_id owns it, campaign_id is contextual (nullable), type + payload
-- + link carry the render. Scheduler feature writes rows for new proposals and
-- received responses; nothing else subscribes in this slice.
CREATE TABLE IF NOT EXISTS notifications (
    id          CHAR(36)     PRIMARY KEY,
    user_id     CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     DEFAULT NULL,
    type        VARCHAR(48)  NOT NULL,            -- e.g. proposal_created | proposal_response
    payload     JSON         DEFAULT NULL,        -- render context (title, counts, etc.)
    link        VARCHAR(512) DEFAULT NULL,        -- in-app URL the notification points to
    read_at     DATETIME     DEFAULT NULL,        -- NULL = unread
    created_at  DATETIME     NOT NULL,

    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    INDEX idx_notifications_user_unread (user_id, read_at),
    INDEX idx_notifications_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
