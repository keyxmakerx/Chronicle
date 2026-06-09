-- Reverse C-CAL-DASHBOARD-W5a: drop the per-calendar visibility columns.
ALTER TABLE calendars
    DROP COLUMN visibility_rules,
    DROP COLUMN visibility;
