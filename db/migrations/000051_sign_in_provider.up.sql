-- Sign-in with an outside provider (OpenID Connect): the site's one provider
-- setting, and which provider account belongs to which Chronicle account.
-- The client secret is stored AES-256-GCM encrypted under a key derived from
-- SECRET_KEY, like the email password.
-- Core tables, core migration: no plugin tables referenced.

CREATE TABLE IF NOT EXISTS oidc_settings (
    id                      TINYINT       NOT NULL DEFAULT 1,
    enabled                 BOOLEAN       NOT NULL DEFAULT FALSE,
    button_name             VARCHAR(40)   NOT NULL DEFAULT '',
    issuer                  VARCHAR(255)  NOT NULL DEFAULT '',
    client_id               VARCHAR(255)  NOT NULL DEFAULT '',
    client_secret_encrypted VARCHAR(1024) NULL,
    allow_signup            BOOLEAN       NOT NULL DEFAULT FALSE,
    hide_password           BOOLEAN       NOT NULL DEFAULT FALSE,
    last_test_at            DATETIME      NULL,
    last_test_ok            BOOLEAN       NOT NULL DEFAULT FALSE,
    last_test_note          VARCHAR(255)  NOT NULL DEFAULT '',
    updated_at              DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT chk_oidc_settings_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_identities (
    id           BIGINT       NOT NULL AUTO_INCREMENT,
    user_id      CHAR(36)     NOT NULL,
    issuer       VARCHAR(255) NOT NULL,
    subject      VARCHAR(255) NOT NULL,
    email        VARCHAR(255) NOT NULL DEFAULT '',
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME     NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_user_identities_subject (issuer, subject),
    KEY idx_user_identities_user (user_id),
    CONSTRAINT fk_user_identities_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
