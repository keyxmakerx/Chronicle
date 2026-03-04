-- Rollback migration 000041: Remove session recurrence and RSVP tokens.

DROP TABLE IF EXISTS session_rsvp_tokens;

ALTER TABLE sessions
    DROP COLUMN is_recurring,
    DROP COLUMN recurrence_type,
    DROP COLUMN recurrence_interval,
    DROP COLUMN recurrence_day_of_week,
    DROP COLUMN recurrence_end_date;
