-- C-CALV4-RSVP-P8B [PB-4 SIGNED]: the send bookkeeping behind the schedule-ask
-- email — one row per recipient ACTUALLY HANDED TO THE MAILER WITHOUT ERROR.
--
-- WHY A TABLE AND NOT AN IN-MEMORY COUNTER. This endpoint mails other people's
-- inboxes on demand; it is the only control in calendar-v4 whose abuse cannot
-- be retracted. A cooldown that resets on every deploy is not a cooldown. It
-- also buys the readout that makes the control honest: the Bench prints "asked
-- 2 hours ago — you can ask again in 4 hours" and disables the button with that
-- reason visible, instead of letting the Director discover the limit by
-- hitting a 429.
--
-- TWO LIMITS ARE READ OFF THIS TABLE, and they answer different questions:
--
--   per CAMPAIGN, 6h    MAX(sent_at) WHERE campaign_id = ?
--                       "has this roster been asked recently at all?"
--                       Refuses the whole send.
--   per RECIPIENT, 24h  the distinct recipients inside the window
--                       "has THIS member been asked recently?"
--                       SKIPS that member; the rest of the send proceeds. So a
--                       legitimate second ask after somebody joins mails the
--                       new member and nobody else.
--
-- The third layer — a per-USER 10/hour limiter on the route itself — is
-- deliberately NOT here. It is the cheap outer guard against a stuck client,
-- it is about the actor rather than the roster, and an in-memory sliding
-- window is the right shape for it (internal/middleware/ratelimit.go).
--
-- NOTHING IS RECORDED WHEN NOTHING WAS SENT. SMTP unconfigured, no address on
-- file, or a send error writes NO row: a cooldown must never lock out a
-- campaign that received no mail.
--
-- event_id IS NULLABLE AND ON DELETE SET NULL. The ask outlives the session it
-- mentioned — the schedule question ("when are you generally free?") is valid
-- in a campaign that has scheduled nothing at all, which is precisely the
-- campaign that most needs asking. It is recorded only so a future reader can
-- tell which send also carried RSVP action links.
--
-- The campaign and user FKs CASCADE. A deleted campaign or user takes its ask
-- rows with it, which can only ever SHORTEN a cooldown, never lengthen one —
-- the safe direction for a row whose absence means "nobody has been asked yet".
--
-- IDEMPOTENT DDL (`CREATE TABLE IF NOT EXISTS`) per CLAUDE.md and ADR-044/045.
-- PLUGIN-SCOPED, not core: it FKs calendar_events, this plugin's own migration
-- 001, and plugin migrations run AFTER core so the core FKs resolve too. A core
-- migration referencing calendar_events would crash a fresh DB.
--
-- APPEND-ONLY: 013 and 014 are untouched (tools/check-migration-immutability.sh).

CREATE TABLE IF NOT EXISTS calendar_schedule_asks (
    id                CHAR(36)  PRIMARY KEY,
    campaign_id       CHAR(36)  NOT NULL,
    event_id          CHAR(36)  DEFAULT NULL,
    recipient_user_id CHAR(36)  NOT NULL,
    actor_user_id     CHAR(36)  NOT NULL,
    sent_at           DATETIME  NOT NULL,

    CONSTRAINT fk_cal_schedule_asks_campaign  FOREIGN KEY (campaign_id)       REFERENCES campaigns(id)       ON DELETE CASCADE,
    CONSTRAINT fk_cal_schedule_asks_event     FOREIGN KEY (event_id)          REFERENCES calendar_events(id) ON DELETE SET NULL,
    CONSTRAINT fk_cal_schedule_asks_recipient FOREIGN KEY (recipient_user_id) REFERENCES users(id)           ON DELETE CASCADE,
    CONSTRAINT fk_cal_schedule_asks_actor     FOREIGN KEY (actor_user_id)     REFERENCES users(id)           ON DELETE CASCADE,

    -- The campaign cooldown read: MAX(sent_at) for one campaign.
    INDEX idx_cal_schedule_asks_campaign (campaign_id, sent_at),
    -- The per-recipient floor read: who in this campaign was asked since X.
    INDEX idx_cal_schedule_asks_recipient (campaign_id, recipient_user_id, sent_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
