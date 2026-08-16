-- C-CALV4-ANCHOR: pin one in-world date to one real-world date, so every
-- fantasy day has a Gregorian date derivable from it.
--
-- WHY THIS EXISTS. Chronicle stores availability on a REAL axis:
-- sessions/migrations/002 keys `member_availability` to a Gregorian
-- `day_of_week` and `availability_exceptions` to a Gregorian `on_date`. The
-- calendar Block renders FANTASY days. Nothing joined the two — `epoch_name` is
-- a label, and `tracks_real_time`/`real_time_zone` (migration 012) apply only
-- to `reallife` mode — so on a fantasy calendar the availability strip has no
-- data to draw from, and a strip drawn anyway would be a fabricated figure.
-- One stored pair closes it, and it answers "when is our next session,
-- in-world" at the same time.
--
-- WHAT IS STORED, AND WHY IT IS THE NAMED DATE RATHER THAN A DAY COUNT. The
-- alternative was (real_date, absolute_day). Both work until the owner EDITS
-- THE CALENDAR STRUCTURE — adds a month, changes a month's length — and then
-- they disagree:
--
--   · storing the absolute day keeps the DAY COUNT fixed, so the in-world date
--     the anchor names silently slides to a different day;
--   · storing the in-world date keeps the NAMED DAY fixed, and the day count
--     is recomputed from the new structure.
--
-- The second is what an owner means. "The campaign began on Marpenoth 14, which
-- was 3 October" is a fact about a named day; it should survive them fixing a
-- month they had mis-declared. So the y/m/d is stored and Calendar.AbsoluteDay
-- converts on read.
--
-- ALL FOUR ARE SET TOGETHER OR NONE ARE. A partial anchor is not a weaker
-- anchor, it is an unanswerable one, and NULL is the honest state: a calendar
-- with no anchor reports that it cannot map a day rather than guessing an
-- epoch. Enforced in the service and asserted by the model's HasRealAnchor().
--
--   anchor_year/month/day — the IN-WORLD date, in this calendar's own terms.
--                           Month is 1-based, matching Calendar.CurrentMonth
--                           and AbsoluteDay's parameter.
--   anchor_real_date      — the Gregorian date that day equals. DATE, not
--                           DATETIME: the mapping is day-granular, and the
--                           availability windows carry their own minutes in
--                           their own zone.
--
-- Schema-only, idempotent, append-only. Every column NULL-backfilled, so an
-- existing calendar is un-anchored and nothing about it changes on upgrade.
-- Setting an anchor is an owner action, never a data migration — there is no
-- defensible epoch to guess on someone else's world.
ALTER TABLE calendars
  ADD COLUMN IF NOT EXISTS anchor_year      INT  DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS anchor_month     INT  DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS anchor_day       INT  DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS anchor_real_date DATE DEFAULT NULL;
