-- Migration: 000024_security_admin
-- Description: Adds site-wide security event logging and user account disable
-- capability. Security events track logins, failed attempts, password resets,
-- admin actions, and session terminations for the admin security dashboard.

-- Allow admins to disable user accounts (blocks login, destroys sessions).
ALTER TABLE users ADD COLUMN is_disabled BOOLEAN NOT NULL DEFAULT FALSE;

-- Site-wide security event log. Unlike audit_log (campaign-scoped), this
-- tracks authentication and admin security actions across the entire site.
CREATE TABLE IF NOT EXISTS security_events (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,

    -- Event classification: login.success, login.failed, password.reset_initiated,
    -- password.reset_completed, admin.privilege_changed, admin.user_disabled,
    -- admin.user_enabled, admin.session_terminated, admin.force_logout,
    -- account.disabled_login_attempt
    event_type  VARCHAR(50) NOT NULL,

    -- The user this event relates to (NULL for failed logins with unknown email).
    user_id     CHAR(36) DEFAULT NULL,

    -- The admin who initiated the action (NULL for user-initiated events).
    actor_id    CHAR(36) DEFAULT NULL,

    -- Client information for forensic analysis.
    ip_address  VARCHAR(45) NOT NULL DEFAULT '',
    user_agent  TEXT DEFAULT NULL,

    -- Flexible metadata (email attempted, reason, etc.).
    details     JSON DEFAULT NULL,

    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Query patterns: filter by type, look up by user, search by IP, recent events.
    INDEX idx_security_events_type_created (event_type, created_at DESC),
    INDEX idx_security_events_user (user_id, created_at DESC),
    INDEX idx_security_events_ip (ip_address, created_at DESC),
    INDEX idx_security_events_created (created_at DESC),
    INDEX idx_security_events_actor (actor_id, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
