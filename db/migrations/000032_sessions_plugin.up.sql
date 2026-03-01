-- Sessions plugin: game session scheduling, linked entities, and RSVP tracking.

CREATE TABLE sessions (
    id VARCHAR(36) PRIMARY KEY,
    campaign_id VARCHAR(36) NOT NULL,
    name VARCHAR(200) NOT NULL,
    summary TEXT,
    notes TEXT,
    notes_html TEXT,
    scheduled_date DATE,
    calendar_year INT,
    calendar_month INT,
    calendar_day INT,
    status VARCHAR(20) NOT NULL DEFAULT 'planned',
    sort_order INT NOT NULL DEFAULT 0,
    created_by VARCHAR(36) NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE session_entities (
    id INT AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    entity_id VARCHAR(36) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'mentioned',
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    UNIQUE KEY uq_session_entity (session_id, entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE session_attendees (
    id INT AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'invited',
    responded_at DATETIME,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE KEY uq_session_attendee (session_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_sessions_campaign ON sessions(campaign_id);
CREATE INDEX idx_sessions_scheduled ON sessions(campaign_id, scheduled_date);
CREATE INDEX idx_session_entities_entity ON session_entities(entity_id);
CREATE INDEX idx_session_attendees_user ON session_attendees(user_id);

-- Register sessions addon.
INSERT INTO addons (id, name, slug, description, category, status, created_at, updated_at)
VALUES (
    UUID(), 'Sessions', 'sessions',
    'Track game sessions with scheduling, linked entities, and RSVP.',
    'plugin', 'active', NOW(), NOW()
)
ON DUPLICATE KEY UPDATE name = VALUES(name);
