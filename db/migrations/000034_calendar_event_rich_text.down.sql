-- 000034 rollback: Remove rich text column from calendar events.
ALTER TABLE calendar_events
    DROP COLUMN description_html;
