-- C-CAL-DASHBOARD-W5a: per-calendar visibility. Mirrors the per-event
-- visibility model (calendar_events.visibility + visibility_rules, migration
-- 001) at the CALENDAR level so a whole calendar can be hidden from specific
-- players/roles. Reusing the same shape lets the service reuse canUserView and
-- the UI (W5b) reuse the chip-row VisibilityEditor verbatim.
--
-- `calendars` is this plugin's own table (migration 001), so this plugin-scoped
-- migration (runs after core) references only a plugin table — safe.
--
-- DEFAULT 'everyone' backfills every existing calendar as visible-to-all, so
-- nothing disappears on upgrade (NO regression). visibility_rules is the
-- optional {"allowed_users":[],"denied_users":[]} JSON allow/deny override,
-- identical to calendar_events.
ALTER TABLE calendars
    ADD COLUMN visibility       VARCHAR(20) NOT NULL DEFAULT 'everyone',
    ADD COLUMN visibility_rules JSON                 DEFAULT NULL;
