-- Add per-user visibility rules to calendar events and standalone timeline events.
-- Enables allowed_users/denied_users JSON lists (same pattern as timelines table).
ALTER TABLE calendar_events ADD COLUMN visibility_rules JSON DEFAULT NULL AFTER visibility;
ALTER TABLE timeline_events ADD COLUMN visibility_rules JSON DEFAULT NULL AFTER visibility;
