-- Calendar eras and season weather effects.
-- Eras are named time periods (e.g. "Age of Fire", "Third Age") with year ranges.
-- Weather effects are per-season text+icon hints (e.g. "Snowy", "Mild and rainy").

CREATE TABLE calendar_eras (
    id INT AUTO_INCREMENT PRIMARY KEY,
    calendar_id VARCHAR(36) NOT NULL,
    name VARCHAR(200) NOT NULL,
    start_year INT NOT NULL,
    end_year INT DEFAULT NULL,
    description TEXT DEFAULT NULL,
    color VARCHAR(20) NOT NULL DEFAULT '#6366f1',
    sort_order INT NOT NULL DEFAULT 0,
    FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE,
    INDEX idx_eras_calendar (calendar_id)
);

ALTER TABLE calendar_seasons ADD COLUMN weather_effect VARCHAR(200) DEFAULT NULL;
