-- 002_calv5_clear_calendar_links — CALV5-PLACEHOLDER
--
-- The calendar's tables are wiped by the calendar plugin's migration 019
-- (CALV5 clean slate). Two things in THIS plugin pointed at them:
--
--   timeline_event_links.event_id -> calendar_events.id  (ON DELETE CASCADE)
--   timelines.calendar_id         -> calendars.id        (ON DELETE SET NULL)
--
-- Both are cleared here, in the timeline plugin's own migration, because a
-- plugin owns its tables (Chronicle rule 8) and because of ORDERING: plugin
-- migrations run in the order main.go registers them, calendar before
-- timeline, so a calendar migration touching these would run before they
-- existed on a fresh database.
--
-- BELT AND BRACES, DELIBERATELY. Since 019 EMPTIES `calendars` and
-- `calendar_events` rather than dropping them (it has to: these two FKs are
-- among the three that made dropping them fail — see 019's header), the two
-- constraints above already do this work as 019 runs. Both statements below
-- are therefore expected to affect zero rows on a database that took 019
-- first, and both are idempotent. They stay because they are the timeline
-- plugin's own guarantee of its own state: if 019 is ever changed to drop the
-- constraints instead of leaning on them, this migration is what still leaves
-- the plugin decoupled rather than holding rows that reference nothing.
--
-- Without this, every row in timeline_event_links references an event that no
-- longer exists. The repository's reads of them are already dark
-- (CALV5-PLACEHOLDER in repository.go), so the rows would be invisible ballast
-- that V5 would later have to guess about.
--
-- The link TABLE is kept — the timeline plugin still owns the concept of
-- linking an event to a timeline, and V5 re-fills it through a calendar
-- service interface rather than a JOIN.

DELETE FROM timeline_event_links;

UPDATE timelines SET calendar_id = NULL WHERE calendar_id IS NOT NULL;
