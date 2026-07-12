-- Availability scheduler — Slice 1 (C-SCHED-P1): recurring per-member weekly
-- availability plus per-date exceptions. These tables are the substrate for
-- the DM heatmap overlay and (later phases) slot proposals.
--
-- RULING RC-12.5 / C-SCHED-AUDIT §A5: a RECURRING availability block is stored
-- as a ZONE-LOCAL WALL-CLOCK — (day_of_week, start_minute, end_minute) in the
-- member's own IANA zone (`tz`) — NEVER as a UTC instant/offset. The wall-clock
-- is resolved to an absolute instant only when projected onto a concrete week's
-- real dates (internal/timeutil), which is what keeps it DST-correct: "18:00
-- local" is 18:00 regardless of daylight-saving.
--
-- SECURITY (design §5 / RC-11): availability is member-only data. It lives in
-- its OWN tables — deliberately NOT session_attendees, whose RSVP status is
-- already serialized into campaign + AI export payloads — so it stays out of
-- egress by construction (see the own-tables egress test).

CREATE TABLE IF NOT EXISTS member_availability (
    id           CHAR(36)    PRIMARY KEY,
    campaign_id  CHAR(36)    NOT NULL,
    user_id      CHAR(36)    NOT NULL,
    day_of_week  TINYINT     NOT NULL,           -- 0=Sun .. 6=Sat (matches Go time.Weekday and sessions.recurrence_day_of_week)
    start_minute SMALLINT    NOT NULL,           -- minutes from LOCAL midnight, zone-local wall-clock [0..1440)
    end_minute   SMALLINT    NOT NULL,           -- exclusive end [1..1440]
    state        VARCHAR(16) NOT NULL DEFAULT 'available',  -- available | preferred (absence of a row = unavailable)
    tz           VARCHAR(64) NOT NULL,           -- IANA zone the wall-clock is expressed in (e.g. America/New_York)
    updated_at   DATETIME    NOT NULL,

    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE KEY uq_member_avail_block (campaign_id, user_id, day_of_week, start_minute, end_minute),
    INDEX idx_member_avail_campaign (campaign_id),
    INDEX idx_member_avail_user (campaign_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS availability_exceptions (
    id           CHAR(36)    PRIMARY KEY,
    campaign_id  CHAR(36)    NOT NULL,
    user_id      CHAR(36)    NOT NULL,
    on_date      DATE        NOT NULL,           -- the specific real-world (Gregorian) date this override applies to
    start_minute SMALLINT    NOT NULL,           -- zone-local wall-clock minutes from local midnight
    end_minute   SMALLINT    NOT NULL,
    state        VARCHAR(16) NOT NULL DEFAULT 'unavailable',  -- available | preferred | unavailable (overrides the recurring pattern for on_date)
    tz           VARCHAR(64) NOT NULL,
    updated_at   DATETIME    NOT NULL,

    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE KEY uq_avail_exc_block (campaign_id, user_id, on_date, start_minute, end_minute),
    INDEX idx_avail_exc_campaign_date (campaign_id, on_date),
    INDEX idx_avail_exc_user (campaign_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
