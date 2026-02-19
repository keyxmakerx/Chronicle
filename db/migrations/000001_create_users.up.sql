-- Migration: 000001_create_users
-- Description: Creates the users table for authentication and user management.
-- Related: ADR-002 (MariaDB), Auth Plugin

CREATE TABLE IF NOT EXISTS users (
    -- UUID generated in Go code (MariaDB doesn't have gen_random_uuid).
    id          CHAR(36)     NOT NULL,

    -- Login credentials.
    email       VARCHAR(255) NOT NULL,
    display_name VARCHAR(100) NOT NULL,

    -- Password hash using argon2id (memory-hard, GPU-resistant).
    password_hash VARCHAR(255) NOT NULL,

    -- Optional profile image path.
    avatar_path VARCHAR(500) DEFAULT NULL,

    -- System-level admin flag (for instance management, not campaign).
    is_admin    BOOLEAN      NOT NULL DEFAULT FALSE,

    -- 2FA/TOTP support (Phase 3).
    totp_secret  VARCHAR(255) DEFAULT NULL,
    totp_enabled BOOLEAN      NOT NULL DEFAULT FALSE,

    -- Timestamps. Use DATETIME (not TIMESTAMP which has 2038 limit).
    -- parseTime=true in DSN ensures Go scans these as time.Time.
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME   DEFAULT NULL,

    PRIMARY KEY (id),
    UNIQUE KEY idx_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
