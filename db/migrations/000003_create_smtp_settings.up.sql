-- 000003_create_smtp_settings.up.sql
-- SMTP configuration for outbound email (password resets, transfer confirmations).
-- Singleton table: only one row (id=1) is allowed.

CREATE TABLE smtp_settings (
    id                 INT           NOT NULL DEFAULT 1,
    host               VARCHAR(255)  NOT NULL DEFAULT '',
    port               INT           NOT NULL DEFAULT 587,
    username           VARCHAR(255)  NOT NULL DEFAULT '',
    password_encrypted VARBINARY(512) DEFAULT NULL,
    from_address       VARCHAR(255)  NOT NULL DEFAULT '',
    from_name          VARCHAR(100)  NOT NULL DEFAULT 'Chronicle',
    encryption         VARCHAR(20)   NOT NULL DEFAULT 'starttls',
    enabled            BOOLEAN       NOT NULL DEFAULT FALSE,
    updated_at         DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    CONSTRAINT smtp_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Insert the default row so GET always returns data.
INSERT INTO smtp_settings (id) VALUES (1);
