-- Password reset tokens for the forgot-password flow.
-- Tokens are single-use, expire after a configurable window (default 1 hour).
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id         INT           NOT NULL AUTO_INCREMENT,
    user_id    CHAR(36)      NOT NULL,
    email      VARCHAR(255)  NOT NULL,
    token_hash CHAR(64)      NOT NULL,  -- SHA-256 hash of the token (never store plaintext)
    expires_at DATETIME      NOT NULL,
    used_at    DATETIME      DEFAULT NULL,
    created_at DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY idx_reset_token_hash (token_hash),
    KEY idx_reset_user_id (user_id),
    KEY idx_reset_expires (expires_at),

    CONSTRAINT fk_reset_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
