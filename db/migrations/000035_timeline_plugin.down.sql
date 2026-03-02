-- Revert timeline addon to its original seed values from migration 000015
-- (category='widget', status='planned') rather than deleting it.
UPDATE addons
SET category    = 'widget',
    status      = 'planned',
    description = 'Visual timeline widget for campaign events and entity histories'
WHERE slug = 'timeline';

-- Drop timeline tables in dependency order.
DROP TABLE IF EXISTS timeline_entity_group_members;
DROP TABLE IF EXISTS timeline_entity_groups;
DROP TABLE IF EXISTS timeline_event_links;
DROP TABLE IF EXISTS timelines;
