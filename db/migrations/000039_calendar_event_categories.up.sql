-- Custom event categories per calendar (replaces hardcoded list).
CREATE TABLE IF NOT EXISTS calendar_event_categories (
    id         INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36)  NOT NULL,
    slug       VARCHAR(50)  NOT NULL,
    name       VARCHAR(100) NOT NULL,
    icon       VARCHAR(10)  NOT NULL DEFAULT '',
    color      VARCHAR(7)   NOT NULL DEFAULT '#6b7280',
    sort_order INT          NOT NULL DEFAULT 0,
    UNIQUE KEY idx_calendar_category_slug (calendar_id, slug),
    CONSTRAINT fk_event_categories_calendar FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
