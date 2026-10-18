-- 028_event_rolls — a roll on an event, and the result of each night it lands.
--
-- calendar_event_rolls: one row per event that rolls when it happens (the
-- Bloodnight). config is the roll as JSON: dice or a rolling table, the
-- result bands with what players and the Director see, and whether a night
-- not rolled ahead is rolled when it arrives. The service validates it.
--
-- calendar_event_roll_nights: one stored result per occurrence date, the
-- same shape as calendar_weather_days. total and faces are a dice roll's,
-- entry a rolling table's. A locked night keeps its result when the year is
-- rolled again. told is NULL when the night follows its result's own "tell
-- players" setting, or the Director's choice for this night alone. Who may
-- see which night is a service rule (players: told nights up to today).
--
-- Both cascade with the event, so deleting it leaves nothing behind.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS calendar_event_rolls (
    event_id    VARCHAR(36) NOT NULL,
    config      JSON        NOT NULL,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (event_id),
    CONSTRAINT fk_cal_event_rolls_event FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS calendar_event_roll_nights (
    event_id    VARCHAR(36)  NOT NULL,
    year        INT          NOT NULL,
    month       INT          NOT NULL,
    day         INT          NOT NULL,
    total       INT          DEFAULT NULL,
    faces       TEXT         DEFAULT NULL,
    entry       VARCHAR(200) DEFAULT NULL,
    source      VARCHAR(10)  NOT NULL DEFAULT 'ahead',
    locked      BOOLEAN      NOT NULL DEFAULT FALSE,
    told        BOOLEAN      DEFAULT NULL,
    rolled_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (event_id, year, month, day),
    CONSTRAINT fk_cal_event_roll_nights_event FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
