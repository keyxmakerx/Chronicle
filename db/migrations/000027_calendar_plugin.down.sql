-- Revert calendar addon to its original seed values from migration 000015.
UPDATE addons
SET category    = 'widget',
    status      = 'planned',
    description = 'Campaign calendar with custom date systems and event tracking'
WHERE slug = 'calendar';

-- Drop calendar tables in dependency order.
DROP TABLE IF EXISTS calendar_events;
DROP TABLE IF EXISTS calendar_seasons;
DROP TABLE IF EXISTS calendar_moons;
DROP TABLE IF EXISTS calendar_weekdays;
DROP TABLE IF EXISTS calendar_months;
DROP TABLE IF EXISTS calendars;

-- Revert category ENUM (remove 'plugin'). Safe because calendar/maps rows
-- that used 'plugin' were reverted to 'widget' above.
ALTER TABLE addons MODIFY COLUMN category ENUM('module', 'widget', 'integration') NOT NULL DEFAULT 'module';
