-- 019_calv5_clean_slate — CALV5-PLACEHOLDER
--
-- Drops every table the calendar plugin created in 001-018. The operator ruled
-- on 2026-08-21 that no old calendar data is preserved: V5 is built on a clean
-- slate rather than migrated onto.
--
-- WHY THIS IS A NEW MIGRATION AND NOT AN EDIT. Chronicle's migrations are
-- append-only and immutable, and a CI guard enforces it — deleting or editing
-- an applied migration crash-loops boot (the 000030 incident, ADR-044/045).
-- 001-018 therefore stay exactly as they shipped and this one undoes them.
--
-- On a FRESH database this is create-then-drop: 001-018 build the schema and
-- 019 removes it. That is wasteful and it is correct; the alternative is
-- editing history, which is the thing that breaks instances.
--
-- Every statement is IF EXISTS so re-running is a no-op, and the order is
-- children before parents so foreign keys never block a drop.
--
-- V5 ships its own numbered migrations from 020 onward in this same directory,
-- so the plugin's lineage stays unbroken.

-- Link tables and rows that point INTO the calendar, first.
DROP TABLE IF EXISTS entity_event_links;
DROP TABLE IF EXISTS entity_era_links;
DROP TABLE IF EXISTS calendar_event_rsvp_tokens;
DROP TABLE IF EXISTS calendar_event_rsvps;
DROP TABLE IF EXISTS calendar_schedule_asks;

-- World-state cluster (migration 008).
DROP TABLE IF EXISTS calendar_day_weather;
DROP TABLE IF EXISTS calendar_celestial_events;
DROP TABLE IF EXISTS calendar_moon_phases;
DROP TABLE IF EXISTS calendar_special_days;

-- Weather / cycles / festivals cluster (003, 005).
DROP TABLE IF EXISTS calendar_cycle_entries;
DROP TABLE IF EXISTS calendar_cycles;
DROP TABLE IF EXISTS calendar_festivals;
DROP TABLE IF EXISTS calendar_weather_zones;
DROP TABLE IF EXISTS calendar_weather;

-- Per-viewer preferences (006, 007, 014, 016, 017). This table held four
-- unrelated facts on one row and only one of them was about a calendar; the
-- other three were sidebar/layer/section preferences that a calendar delete
-- once wiped as a side effect. V5 must not rebuild it that way.
DROP TABLE IF EXISTS calendar_active;

-- Events and sub-resources.
DROP TABLE IF EXISTS calendar_events;
DROP TABLE IF EXISTS calendar_event_categories;
DROP TABLE IF EXISTS calendar_eras;
DROP TABLE IF EXISTS calendar_seasons;
DROP TABLE IF EXISTS calendar_moons;
DROP TABLE IF EXISTS calendar_weekdays;
DROP TABLE IF EXISTS calendar_months;

-- The root, last.
DROP TABLE IF EXISTS calendars;
