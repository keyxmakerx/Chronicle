-- C-CAL-WORLDSTATE-SERVER-MODEL (Phase 1, build-order step 2): make the
-- browser-only `worldState` blob a real, server-side concept so the
-- production calendar port (Phase 2) and Foundry push (Phase 5b) can both
-- source it from real campaign data.
--
-- All tables here are PLUGIN-scoped and reference only calendar_* tables
-- (FK -> calendars(id) / calendar_moons(id)) so the migration is safe by
-- construction: it never touches core tables and runs after core on a fresh
-- DB (per CLAUDE.md "Migration Safety Rules").
--
-- Shape target: the showcase engine's worldState seed
-- (static/js/cal-almanac.js ~L464-481, CATALOG Part 8). The Go-side
-- BuildWorldStateSeed assembles the same shape from these tables + the
-- existing calendar/moon/season/event repos.

-- Per-day authored weather. Today the live calendar carries exactly ONE
-- "current weather" row in calendar_weather (migration 003); this table
-- adds per-DATE authored weather so a specific day can carry its own
-- condition independent of the live row. weather_type is a free-form
-- vocabulary id (e.g. "clear" | "rain" | "snow" | "fog"); see the PR's
-- stop-and-flag note on sharing a weather_type vocabulary with
-- calendar_weather.
CREATE TABLE IF NOT EXISTS calendar_day_weather (
    id           INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id  VARCHAR(36)  NOT NULL,
    year         INT          NOT NULL,
    month        INT          NOT NULL,
    day          INT          NOT NULL,
    weather_type VARCHAR(50)  NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    -- One authored weather row per date: the seed reads at most one.
    UNIQUE KEY uq_calendar_day_weather_date (calendar_id, year, month, day),
    CONSTRAINT fk_calendar_day_weather_calendar
      FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Date-specific celestial events (meteor shower, solar/lunar eclipse,
-- blood moon, ...). Distinct from calendar_events (the GM's narrative
-- events): these drive the sky-band's CELESTIAL_EFFECTS layer.
--
-- `visibility` is NOT in the dispatch's column listing but the GOAL
-- explicitly requires the GET to filter GM-only celestial events for
-- players (via the same dm_only semantics the events table uses). A
-- visibility column is the minimal way to honor that — flagged in the PR.
CREATE TABLE IF NOT EXISTS calendar_celestial_events (
    id             INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id    VARCHAR(36)  NOT NULL,
    year           INT          NOT NULL,
    month          INT          NOT NULL,
    day            INT          NOT NULL,
    type           VARCHAR(50)  NOT NULL,
    start_hour     INT          NOT NULL DEFAULT 0,
    duration_hours INT          NOT NULL DEFAULT 1,
    name           VARCHAR(200) NOT NULL DEFAULT '',
    -- "everyone" | "dm_only" — same vocabulary calendar_events uses so the
    -- service-layer filter (filterEventsByUser / VisibilityRole) is uniform.
    visibility     VARCHAR(20)  NOT NULL DEFAULT 'everyone',
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_calendar_celestial_events_date (calendar_id, year, month, day),
    CONSTRAINT fk_calendar_celestial_events_calendar
      FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Per-moon named-phase vocabulary (e.g. Seluene's 8 phases, Shar's 3).
-- Today moon phases are computed-only (Moon.MoonPhaseName); this table
-- lets a calendar author bespoke named spans keyed by 0..100 percent of
-- the moon's cycle. start_pct/end_pct match the showcase's
-- moon.namedPhases span shape (cyclePct*100 in [start_pct, end_pct)).
CREATE TABLE IF NOT EXISTS calendar_moon_phases (
    id         INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    moon_id    INT          NOT NULL,
    name       VARCHAR(100) NOT NULL,
    start_pct  INT          NOT NULL,
    end_pct    INT          NOT NULL,
    glyph      VARCHAR(16)  NOT NULL DEFAULT '',
    sort_order INT          NOT NULL DEFAULT 0,
    INDEX idx_calendar_moon_phases_moon (moon_id),
    CONSTRAINT fk_calendar_moon_phases_moon
      FOREIGN KEY (moon_id) REFERENCES calendar_moons(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Special-moon-day flags on specific dates (e.g. "festival of the full
-- moon", "shadowfell convergence"). `kind` is a free-form id the renderer
-- maps to a special-day treatment.
CREATE TABLE IF NOT EXISTS calendar_special_days (
    id          INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    year        INT          NOT NULL,
    month       INT          NOT NULL,
    day         INT          NOT NULL,
    kind        VARCHAR(50)  NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_calendar_special_days_date (calendar_id, year, month, day),
    CONSTRAINT fk_calendar_special_days_calendar
      FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Moon-library render params (CATALOG Part 12.1 / showcase MOON_DESIGNS):
-- today calendar_moons stores only name/cycle_days/phase_offset/color.
-- These columns persist the showcase's procedural-render parameters so a
-- moon's appearance can be authored rather than hardcoded in JS.
ALTER TABLE calendar_moons
    ADD COLUMN base_design  VARCHAR(64)  NOT NULL DEFAULT 'moon-realistic-selene',
    ADD COLUMN tint         VARCHAR(32)  NULL,
    ADD COLUMN phase_source VARCHAR(16)  NOT NULL DEFAULT 'css-clip',
    ADD COLUMN size         FLOAT        NOT NULL DEFAULT 1,
    ADD COLUMN orbit_speed  FLOAT        NOT NULL DEFAULT 1;

-- Persisted live mood-tint (the player overlay wash). D2 = page-load read,
-- so plain nullable columns on the calendar suffice; null intensity means
-- "no mood set".
ALTER TABLE calendars
    ADD COLUMN mood_tint_color     VARCHAR(32) NULL,
    ADD COLUMN mood_tint_intensity FLOAT       NULL;
