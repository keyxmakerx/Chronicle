-- Two-factor recovery codes: each lets its owner past the code step once
-- after losing their authenticator. Only a SHA-256 of each code is kept, so
-- a database copy can't be used to sign in.
-- Core table, core migration: no plugin tables referenced.

CREATE TABLE IF NOT EXISTS user_recovery_codes (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    user_id    CHAR(36)     NOT NULL,
    code_hash  CHAR(64)     NOT NULL,
    used_at    DATETIME     NULL,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_user_recovery_codes_hash (user_id, code_hash),
    CONSTRAINT fk_user_recovery_codes_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
