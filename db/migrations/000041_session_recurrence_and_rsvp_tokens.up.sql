-- Migration 000041: Add session recurrence support and RSVP email tokens.
--
-- Recurrence fields allow sessions to repeat on a schedule (weekly, biweekly,
-- monthly). RSVP tokens enable one-click email responses without login.

-- Add recurrence columns to sessions table.
ALTER TABLE sessions
    ADD COLUMN is_recurring TINYINT(1) NOT NULL DEFAULT 0 AFTER status,
    ADD COLUMN recurrence_type VARCHAR(20) DEFAULT NULL AFTER is_recurring,
    ADD COLUMN recurrence_interval INT DEFAULT 1 AFTER recurrence_type,
    ADD COLUMN recurrence_day_of_week INT DEFAULT NULL AFTER recurrence_interval,
    ADD COLUMN recurrence_end_date DATE DEFAULT NULL AFTER recurrence_day_of_week;

-- RSVP tokens for email-based one-click responses.
-- Each token is single-use and tied to a specific session + user + action.
CREATE TABLE IF NOT EXISTS session_rsvp_tokens (
    id INT AUTO_INCREMENT PRIMARY KEY,
    token VARCHAR(64) NOT NULL UNIQUE,
    session_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    action VARCHAR(20) NOT NULL COMMENT 'accepted, declined, tentative',
    used_at DATETIME DEFAULT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_rsvp_token (token),
    INDEX idx_rsvp_session_user (session_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
