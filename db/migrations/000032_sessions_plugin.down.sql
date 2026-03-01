DROP TABLE IF EXISTS session_attendees;
DROP TABLE IF EXISTS session_entities;
DROP TABLE IF EXISTS sessions;

DELETE FROM campaign_addons WHERE addon_id = (SELECT id FROM addons WHERE slug = 'sessions');
DELETE FROM addons WHERE slug = 'sessions';
