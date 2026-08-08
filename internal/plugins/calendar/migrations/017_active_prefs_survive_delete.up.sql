-- C-SWEEP-R4 stage 25 (data/calendar-active-cascade-wipes-prefs).
--
-- DELETING A CALENDAR DESTROYED THREE PREFERENCES THAT HAVE NOTHING TO DO WITH
-- IT. calendar_active carries FOUR facts on one (user_id, campaign_id) row:
--
--   calendar_id     the viewer's last switcher choice        CALENDAR-scoped
--   sidebar_pinned  the V2 shell's sidebar pin (007)         CAMPAIGN-scoped
--   block_layers    the calendar-v4 Block layer set (014)    CAMPAIGN-scoped
--   bench_sections  the Bench's collapsed sections (016)     CAMPAIGN-scoped
--
-- Only the first is about a particular calendar. But fk_calendar_active_cal
-- (migration 006) cascades on DELETE, and a cascade deletes the ROW — so
-- deleting one calendar silently reset every viewer's sidebar pin, layer set
-- and Bench sections for the WHOLE CAMPAIGN, including viewers who had never
-- opened the deleted calendar. Three deliberate choices lost to an unrelated
-- act, with nothing in the UI to say it happened.
--
-- THE FIX IS SET NULL, AND IT IS THE SMALLEST OF THE THREE SHAPES ON OFFER.
-- The booking recorded that every path reverses something signed, so this one
-- says plainly what it re-signs and what it does not:
--
--   (a) A SEPARATE calendar_user_prefs TABLE — NOT TAKEN. That is the exact
--       shape PR #368 stop-and-flag #3 rejected, re-affirmed by [LYR-3 SIGNED]
--       (014's header) and [BR2-5 SIGNED] (016's header). Three signed
--       refusals of one shape is an answer, not an obstacle.
--   (b) AN IN-SERVICE RESEAT before DELETE — NOT TAKEN. It has no answer when
--       the deleted calendar was the campaign's LAST one: there is nothing to
--       re-point at, so the row is destroyed anyway and the preferences with
--       it. A fix that works except in the case that loses the most is not a
--       fix.
--   (c) NULLABLE calendar_id + ON DELETE SET NULL — TAKEN. It re-signs ONE
--       sentence of 006's header ("its active-cal pointers go too") while
--       KEEPING that header's actual promise verbatim: "the next read falls
--       back to the new default automatically". It still does. NULL and "no
--       row" resolve identically — calendarRepo.GetActiveCalendarID returns ""
--       for both, and calendarService.resolveActiveCalendar's ladder then walks
--       to the campaign default, then to first-by-sort-order. The pointer is
--       still cleared by the delete; the three preferences beside it survive.
--
-- WHAT A NULL MEANS IS THEREFORE UNCHANGED FROM WHAT AN ABSENT ROW MEANT, which
-- is the property that keeps this a schema change rather than a semantic one.
-- The three preference writers (SetSidebarPinned / SetBlockLayers /
-- SetBenchSections) seed calendar_id on INSERT and their ON DUPLICATE KEY
-- UPDATE clauses name ONLY their own column, so a NULLed pointer stays NULL
-- until the viewer picks a calendar again — it is never silently re-pointed at
-- a default the viewer did not choose.
--
-- SCHEMA-ONLY and IDEMPOTENT. No row is written here: MariaDB applies SET NULL
-- to future deletes, and rows already destroyed by the old cascade are gone
-- beyond recovery — there is nothing to reconcile, only to stop losing.
-- Re-running is safe: MODIFY to the same definition is a no-op, and the FK is
-- dropped-if-present before it is added, so a run interrupted between the two
-- statements completes cleanly on the next boot.
--
-- Plugin-scoped: calendar_active is a calendar-plugin table, so this lives in
-- internal/plugins/calendar/migrations/. A core migration referencing it would
-- crash a fresh DB, because core migrations run BEFORE plugin ones.
--
-- EGRESS: unchanged. calendar_id is already member-scoped display state and
-- enters no export DTO and no AI-workspace payload; making it nullable adds no
-- field and no reader.

ALTER TABLE calendar_active
  MODIFY COLUMN calendar_id VARCHAR(36) NULL;

ALTER TABLE calendar_active
  DROP FOREIGN KEY IF EXISTS fk_calendar_active_cal;

ALTER TABLE calendar_active
  ADD CONSTRAINT fk_calendar_active_cal
    FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE SET NULL;
