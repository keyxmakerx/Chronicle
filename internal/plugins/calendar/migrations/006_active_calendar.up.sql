-- C-CAL-V2-SHELL-FOUNDATION (Wave 1 PR 1): per-(user, campaign)
-- active-calendar pointer. Stores which calendar the user last
-- selected via the multi-cal switcher in a given campaign. Used by
-- the V2 calendar shell to resume on the right calendar across
-- navigation + sessions.
--
-- Falls back to the campaign's default calendar (is_default=1) when
-- no row exists for (user_id, campaign_id) — first visit by a user
-- to a campaign always lands on the default.
--
-- Calendar-scoped FK: when a calendar is deleted, its active-cal
-- pointers go too (ON DELETE CASCADE); the next read falls back to
-- the new default automatically.

CREATE TABLE IF NOT EXISTS calendar_active (
    user_id     VARCHAR(36) NOT NULL,
    campaign_id VARCHAR(36) NOT NULL,
    calendar_id VARCHAR(36) NOT NULL,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, campaign_id),
    INDEX idx_calendar_active_campaign (campaign_id),
    CONSTRAINT fk_calendar_active_cal
      FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
