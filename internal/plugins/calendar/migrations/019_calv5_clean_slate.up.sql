-- 019_calv5_clean_slate — CALV5-PLACEHOLDER
--
-- Drops or empties every table the calendar plugin created in 001-018. The operator ruled
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
-- Every DROP is IF EXISTS and both DELETEs are no-ops on already-empty
-- tables, so re-running is safe. The drops are ordered children before
-- parents, which settles the calendar's OWN foreign keys; the three INBOUND
-- ones (below) are settled by emptying their parents instead of dropping
-- them — ordering alone cannot, since those constraints belong to plugins
-- that migrate later.
--
-- TWO TABLES SURVIVE AS EMPTY STUBS, AND THAT IS NOT AN OVERSIGHT.
-- `calendars` and `calendar_events` are the targets of three foreign keys
-- declared by OTHER plugins, whose migrations are immutable and cannot be
-- edited to drop them:
--
--     sessions/001            fk_sessions_calendar  -> calendars(id)
--     timeline/001            fk_timelines_calendar -> calendars(id)
--     timeline/001            fk_tel_event          -> calendar_events(id)
--
-- The calendar plugin's migrations run BEFORE both of those plugins', so on a
-- FRESH database dropping these two tables makes sessions/001 and timeline/001
-- fail on their first statement with errno 150. Both plugins then sit DEGRADED
-- at version 0 and the features are dead on every new install — the exact
-- shape of the foundry_vtt bug the fresh-DB replay job was built to catch, and
-- the job caught this one before it shipped. On an EXISTING database the same
-- constraints point the other way: MariaDB refuses to DROP a table that is
-- still an FK parent, so the drop fails mid-migration with DDL already
-- committed and no rollback.
--
-- So these two are EMPTIED, not dropped. The deletes do the decoupling work
-- for free: `calendar_events` cascades `timeline_event_links` away, and
-- `calendars` sets `sessions.calendar_id` and `timelines.calendar_id` to NULL.
-- Their own outbound FKs: calendars points only at campaigns;
-- calendar_events points at calendars (both survive) and at entities. So
-- emptying them is safe in either direction — and the order below matters:
-- calendar_events is emptied BEFORE calendars, so the calendars delete's
-- cascade finds calendar_events already empty.
--
-- V5 owns their final shape. It may reuse them, or drop them in 020+ once it
-- has also given sessions and timeline migrations that remove the three
-- constraints above. Dropping them before that is what this comment exists to
-- prevent; `clean_slate_test.go` enforces it.
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

-- Events and sub-resources. `calendar_events` is EMPTIED, not dropped: it is
-- the parent of timeline/001's fk_tel_event. Deleting its rows cascades every
-- timeline_event_links row away, which is the decoupling timeline/002 asks for.
DELETE FROM calendar_events;
DROP TABLE IF EXISTS calendar_event_categories;
DROP TABLE IF EXISTS calendar_eras;
DROP TABLE IF EXISTS calendar_seasons;
DROP TABLE IF EXISTS calendar_moons;
DROP TABLE IF EXISTS calendar_weekdays;
DROP TABLE IF EXISTS calendar_months;

-- The root, last. EMPTIED, not dropped: it is the parent of
-- fk_sessions_calendar and fk_timelines_calendar, both ON DELETE SET NULL, so
-- this one statement also nulls sessions.calendar_id and timelines.calendar_id.
DELETE FROM calendars;
