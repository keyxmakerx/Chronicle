-- C-RSVP-P9: alternating-week availability, and "has this member answered yet?"
-- as a state the product can actually see.
--
-- TWO INDEPENDENT GAPS, ONE MIGRATION because both are one column and one small
-- table on the same aggregate.
--
-- 1. week_parity. A recurring block was "this weekday, every week, forever".
--    An alternating-week group had to hand-punch an exception every fortnight.
--    0 = every week (the DEFAULT, so every row written before this migration
--    keeps exactly the meaning it had), 1 and 2 = the two alternating tracks.
--    See availability_cadence.go for how a real date is mapped onto a track.
--
-- 2. member_availability_status. Absence of availability rows has always meant
--    "unavailable", so a member who never opened the page was INDISTINGUISHABLE
--    from a member who is genuinely never free. The Director could not tell
--    "nobody is free Tuesday" from "nobody has answered", and neither could any
--    nudge we might send. This table records that a member ANSWERED — including
--    answering with an empty grid, which is a real answer and the exact case the
--    derived-from-row-count shortcut gets wrong.

ALTER TABLE member_availability
    ADD COLUMN IF NOT EXISTS week_parity TINYINT NOT NULL DEFAULT 0;

-- The old uniqueness rule predates cadence, so it would collide two blocks that
-- differ ONLY by track — e.g. free Monday evening on week A, busy on week B.
-- Dropped and re-added with the cadence included. Both statements are
-- conditional, so re-running the migration is a no-op rather than an error.
ALTER TABLE member_availability
    DROP INDEX IF EXISTS uq_member_avail_block;

ALTER TABLE member_availability
    ADD UNIQUE KEY IF NOT EXISTS uq_member_avail_block_cadence
        (campaign_id, user_id, day_of_week, start_minute, end_minute, week_parity);

CREATE TABLE IF NOT EXISTS member_availability_status (
    campaign_id CHAR(36)    NOT NULL,
    user_id     CHAR(36)    NOT NULL,
    answered_at DATETIME    NOT NULL,           -- when they last saved a pattern (empty grid included)
    tz          VARCHAR(64) NOT NULL DEFAULT '', -- the zone they answered in, for the "answered" tooltip

    PRIMARY KEY (campaign_id, user_id),
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
