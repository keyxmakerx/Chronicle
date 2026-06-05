-- Reverse C-CAL-ENTITY-TIES-DATA-MODEL: drop the entity-tie link tables. The
-- FKs are ON DELETE CASCADE so dropping the tables themselves is sufficient;
-- the referenced entities/calendar_events/calendar_eras tables are untouched.
DROP TABLE IF EXISTS entity_era_links;
DROP TABLE IF EXISTS entity_event_links;
