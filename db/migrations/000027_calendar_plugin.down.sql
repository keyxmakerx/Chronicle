-- Remove calendar addon registration.
DELETE FROM addons WHERE slug = 'calendar';

-- Drop calendar tables in dependency order.
DROP TABLE IF EXISTS calendar_events;
DROP TABLE IF EXISTS calendar_seasons;
DROP TABLE IF EXISTS calendar_moons;
DROP TABLE IF EXISTS calendar_weekdays;
DROP TABLE IF EXISTS calendar_months;
DROP TABLE IF EXISTS calendars;
