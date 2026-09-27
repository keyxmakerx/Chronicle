-- 020_calv5_schema — the V5 calendar schema.
--
-- 019 left `calendars` and `calendar_events` as empty stubs (foreign keys from
-- sessions and timeline need them to exist) and dropped every other calendar
-- table. This migration ALTERs those two and creates the rest, all starting
-- empty.
--
-- Idempotent throughout (`IF [NOT] EXISTS` on every clause), so it applies on a
-- fresh database, on one already at 019, and a second time on top of itself.

-- --- calendars: three new per-calendar settings; the mood-tint wash belonged
-- to the retired world-state console.
ALTER TABLE calendars
  ADD COLUMN IF NOT EXISTS hemisphere             VARCHAR(10) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS forecasts_enabled       TINYINT(1) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS month_starts_new_week   TINYINT(1) NOT NULL DEFAULT 0,
  DROP COLUMN IF EXISTS mood_tint_color,
  DROP COLUMN IF EXISTS mood_tint_intensity;

-- hemisphere: a real-world calendar's seasons run opposite in the southern
-- hemisphere ('north'/'south', validated in Go); NULL means not chosen.
--
-- forecasts_enabled: players never learn a future day's weather before it
-- happens; this switch lets a Director show a deliberately vague forecast.
--
-- month_starts_new_week: when on, day 1 of every month is the first weekday and
-- intercalary festival days belong to no week, so a festival never shifts the
-- weekdays that follow it. TODO(#741): the day counter does not read it yet.

CREATE TABLE IF NOT EXISTS calendar_months (
    id             INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id    VARCHAR(36)  NOT NULL,
    name           VARCHAR(100) NOT NULL,
    days           INT          NOT NULL DEFAULT 30,
    sort_order     INT          NOT NULL DEFAULT 0,
    is_intercalary TINYINT(1)   NOT NULL DEFAULT 0,
    leap_year_days INT          NOT NULL DEFAULT 0,

    CONSTRAINT fk_cal_months_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_months_order (calendar_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS calendar_weekdays (
    id          INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    sort_order  INT          NOT NULL DEFAULT 0,
    is_rest_day TINYINT(1)   NOT NULL DEFAULT 0,

    CONSTRAINT fk_cal_weekdays_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_weekdays_order (calendar_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- calendar_moons: hidden_from_players lets the Director keep a moon secret
-- (one not yet discovered in-world). A real-world calendar's Moon is computed
-- astronomically, gated on the calendar's real-time flag, so it needs no column.
CREATE TABLE IF NOT EXISTS calendar_moons (
    id                  INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id         VARCHAR(36)  NOT NULL,
    name                VARCHAR(100) NOT NULL,
    cycle_days          FLOAT        NOT NULL DEFAULT 29.5,
    phase_offset        FLOAT        NOT NULL DEFAULT 0,
    color               VARCHAR(7)   NOT NULL DEFAULT '#c0c0c0',
    base_design         VARCHAR(64)  NOT NULL DEFAULT 'moon-realistic-selene',
    tint                VARCHAR(32)  DEFAULT NULL,
    phase_source        VARCHAR(16)  NOT NULL DEFAULT 'css-clip',
    size                FLOAT        NOT NULL DEFAULT 1,
    orbit_speed         FLOAT        NOT NULL DEFAULT 1,
    hidden_from_players TINYINT(1)   NOT NULL DEFAULT 0,

    CONSTRAINT fk_cal_moons_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_moons_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS calendar_seasons (
    id             INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id    VARCHAR(36)  NOT NULL,
    name           VARCHAR(100) NOT NULL,
    start_month    INT          NOT NULL,
    start_day      INT          NOT NULL,
    end_month      INT          NOT NULL,
    end_day        INT          NOT NULL,
    description    TEXT,
    color          VARCHAR(7)   NOT NULL DEFAULT '#6b7280',
    weather_effect VARCHAR(200) DEFAULT NULL,

    CONSTRAINT fk_cal_seasons_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_seasons_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- calendar_eras: an era begins, and if it ends, ends, on a day. end_year
-- NULL means ongoing; application code keeps the three end_* columns all set or
-- all NULL, the same shape calendars.anchor_* uses.
CREATE TABLE IF NOT EXISTS calendar_eras (
    id          INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(200) NOT NULL,
    start_year  INT          NOT NULL,
    start_month INT          NOT NULL DEFAULT 1,
    start_day   INT          NOT NULL DEFAULT 1,
    end_year    INT          DEFAULT NULL,
    end_month   INT          DEFAULT NULL,
    end_day     INT          DEFAULT NULL,
    description TEXT         DEFAULT NULL,
    color       VARCHAR(20)  NOT NULL DEFAULT '#6366f1',
    sort_order  INT          NOT NULL DEFAULT 0,

    CONSTRAINT fk_cal_eras_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_eras_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- calendar_cycles / calendar_cycle_entries: named repeating cycles (a
-- zodiac of years, say). TODO(#771): imports do not write cycles yet.
CREATE TABLE IF NOT EXISTS calendar_cycles (
    id           INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id  VARCHAR(36)  NOT NULL,
    name         VARCHAR(100) NOT NULL,
    cycle_length INT          NOT NULL,
    type         VARCHAR(20)  NOT NULL DEFAULT 'yearly',
    sort_order   INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_cycles_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_cycles_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS calendar_cycle_entries (
    id          INT          AUTO_INCREMENT PRIMARY KEY,
    cycle_id    INT          NOT NULL,
    name        VARCHAR(100) NOT NULL,
    icon        VARCHAR(50)  DEFAULT NULL,
    year_offset INT          NOT NULL DEFAULT 0,
    sort_order  INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_cycle_entries_cycle FOREIGN KEY (cycle_id) REFERENCES calendar_cycles(id) ON DELETE CASCADE,
    INDEX idx_cal_cycle_entries_cycle (cycle_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- calendar_festivals: TODO(#771): imports parse festivals but do not write
-- them here yet.
CREATE TABLE IF NOT EXISTS calendar_festivals (
    id          INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    name        VARCHAR(200) NOT NULL,
    month       INT          DEFAULT NULL,
    day         INT          DEFAULT NULL,
    after_month INT          DEFAULT NULL,
    description TEXT         DEFAULT NULL,
    color       VARCHAR(20)  DEFAULT NULL,
    icon        VARCHAR(50)  DEFAULT NULL,
    sort_order  INT          NOT NULL DEFAULT 0,
    CONSTRAINT fk_cal_festivals_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_cal_festivals_calendar (calendar_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- calendar_event_kinds: one list of event kinds per campaign, shared by all
-- its calendars. default_announced is the "announced" value an event of this
-- kind gets unless it sets its own (see calendar_events.announced). Created
-- before calendar_events is ALTERed, so kind_id's foreign key can resolve.
CREATE TABLE IF NOT EXISTS calendar_event_kinds (
    id                INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id       VARCHAR(36)  NOT NULL,
    slug              VARCHAR(50)  NOT NULL,
    name              VARCHAR(100) NOT NULL,
    icon              VARCHAR(50)  NOT NULL DEFAULT '',
    color             VARCHAR(20)  NOT NULL DEFAULT '#6b7280',
    sort_order        INT          NOT NULL DEFAULT 0,
    default_announced VARCHAR(10)  NOT NULL DEFAULT 'on_day',

    UNIQUE KEY uq_calendar_event_kinds_slug (campaign_id, slug),
    CONSTRAINT fk_calendar_event_kinds_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- calendar_events: kind_id replaces the free-text category, and
-- collect_rsvps goes because game-night answers live with sessions.
--
-- announced: 'ahead' (players may know it in advance) or 'on_day' (revealed on
-- the day), validated in Go like visibility. NULL means the event has no
-- setting of its own and inherits one.
--
-- payload: one nullable JSON slot for small typed extras, such as a moon night
-- ({"type":"blood|magic|conjunction|eclipse|harvest", ...}) or a sky event, so a
-- new kind of extra needs no ALTER.
ALTER TABLE calendar_events
  ADD COLUMN IF NOT EXISTS kind_id    INT         DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS announced  VARCHAR(10) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS payload    JSON        DEFAULT NULL,
  DROP COLUMN IF EXISTS category,
  DROP COLUMN IF EXISTS collect_rsvps;

CREATE INDEX IF NOT EXISTS idx_calendar_events_kind ON calendar_events (kind_id);

-- MariaDB has no `ADD CONSTRAINT ... IF NOT EXISTS`, so the foreign key is
-- dropped if present and added again; a re-run lands on the same constraint.
ALTER TABLE calendar_events
  DROP FOREIGN KEY IF EXISTS fk_calendar_events_kind;
ALTER TABLE calendar_events
  ADD CONSTRAINT fk_calendar_events_kind
    FOREIGN KEY (kind_id) REFERENCES calendar_event_kinds(id) ON DELETE SET NULL;

-- --- calendar_weather: one current-weather row per calendar. zone_id and
-- zone_name are plain labels; there is no weather-zone table.
CREATE TABLE IF NOT EXISTS calendar_weather (
    id                      INT          AUTO_INCREMENT PRIMARY KEY,
    calendar_id             VARCHAR(36)  NOT NULL UNIQUE,
    preset_id               VARCHAR(50)  DEFAULT NULL,
    preset_label            VARCHAR(100) DEFAULT NULL,
    icon                    VARCHAR(50)  DEFAULT NULL,
    color                   VARCHAR(20)  DEFAULT NULL,
    temperature_celsius     FLOAT        DEFAULT NULL,
    wind_speed_kph          FLOAT        DEFAULT NULL,
    wind_speed_tier         VARCHAR(20)  DEFAULT NULL,
    wind_direction          VARCHAR(5)   DEFAULT NULL,
    wind_direction_degrees  INT          DEFAULT NULL,
    precipitation_type      VARCHAR(20)  DEFAULT NULL,
    precipitation_intensity FLOAT        DEFAULT NULL,
    zone_id                 VARCHAR(50)  DEFAULT NULL,
    zone_name               VARCHAR(100) DEFAULT NULL,
    description             TEXT         DEFAULT NULL,
    updated_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_cal_weather_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --- entity_event_links / entity_era_links: pages linked to an event or an
-- era. entities is a core table, so it exists before any plugin migration runs.
CREATE TABLE IF NOT EXISTS entity_event_links (
    id                 INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    entity_id          VARCHAR(36)  NOT NULL,
    event_id           VARCHAR(36)  NOT NULL,
    participation_role VARCHAR(20)  NOT NULL DEFAULT 'involved',
    created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_entity_event_links_pair (entity_id, event_id),
    INDEX idx_entity_event_links_entity (entity_id),
    INDEX idx_entity_event_links_event (event_id),
    CONSTRAINT fk_entity_event_links_entity
      FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_event_links_event
      FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS entity_era_links (
    id                 INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    entity_id          VARCHAR(36)  NOT NULL,
    era_id             INT          NOT NULL,
    participation_role VARCHAR(20)  NULL,
    created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_entity_era_links_pair (entity_id, era_id),
    INDEX idx_entity_era_links_entity (entity_id),
    INDEX idx_entity_era_links_era (era_id),
    CONSTRAINT fk_entity_era_links_entity
      FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_era_links_era
      FOREIGN KEY (era_id) REFERENCES calendar_eras(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
