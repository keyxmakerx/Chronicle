-- Reverse C-RSVP-P9. IF EXISTS keeps the rollback idempotent (safe to re-run /
-- partial-apply recovery).
--
-- The cadence index is dropped and the pre-cadence one restored BEFORE the
-- column goes, so the table is never left without a uniqueness rule on a
-- member's blocks. Rolling back does collapse both alternating tracks back into
-- "every week" — the column is where that distinction lived, so it cannot
-- survive its own removal.
DROP TABLE IF EXISTS member_availability_status;

ALTER TABLE member_availability
    DROP INDEX IF EXISTS uq_member_avail_block_cadence;

ALTER TABLE member_availability
    ADD UNIQUE KEY IF NOT EXISTS uq_member_avail_block
        (campaign_id, user_id, day_of_week, start_minute, end_minute);

ALTER TABLE member_availability DROP COLUMN IF EXISTS week_parity;
