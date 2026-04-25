-- Adds the two security tables referenced by syncapi/repository.go but
-- never previously created:
--
--   - api_ip_blocklist: admin-managed IP blocklist consulted by the REST API
--     middleware. Without this table the blocklist check fails open with
--     `Error 1146 Table 'chronicle.api_ip_blocklist' doesn't exist` WARN
--     logs on every authenticated request. See middleware.go:37-40.
--
--   - api_security_events: audit log for auth failures, IP blocks, device
--     mismatches, rate-limit triggers, and other suspicious activity.
--     Written by LogSecurityEvent and read by the admin security page.
--
-- Both tables live in the syncapi plugin migrations because the syncapi
-- feature owns them (every reference to both tables is inside
-- internal/plugins/syncapi/). Placing them here also matches the
-- migration-layering rule in .ai/conventions.md rule #7.
--
-- Failure mode before this migration: REST requests still succeed (the
-- IsIPBlocked error is swallowed with a WARN and treated as "not blocked";
-- LogSecurityEvent errors are also non-fatal). The cost was only log
-- noise and missing security-event visibility. After this migration both
-- features become live.

CREATE TABLE IF NOT EXISTS api_ip_blocklist (
    id         INT          AUTO_INCREMENT PRIMARY KEY,
    ip_address VARCHAR(45)  NOT NULL,
    reason     VARCHAR(255) DEFAULT NULL,
    blocked_by VARCHAR(36)  NOT NULL,
    expires_at DATETIME     DEFAULT NULL,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_api_ip_blocklist_ip (ip_address),
    KEY idx_api_ip_blocklist_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS api_security_events (
    id          BIGINT       AUTO_INCREMENT PRIMARY KEY,
    event_type  VARCHAR(50)  NOT NULL,
    api_key_id  INT          DEFAULT NULL,
    campaign_id VARCHAR(36)  DEFAULT NULL,
    ip_address  VARCHAR(45)  NOT NULL,
    user_agent  VARCHAR(500) DEFAULT NULL,
    details     JSON         DEFAULT NULL,
    resolved    TINYINT(1)   NOT NULL DEFAULT 0,
    resolved_by VARCHAR(36)  DEFAULT NULL,
    resolved_at DATETIME     DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    KEY idx_api_security_events_type (event_type),
    KEY idx_api_security_events_key (api_key_id),
    KEY idx_api_security_events_campaign (campaign_id),
    KEY idx_api_security_events_ip (ip_address),
    KEY idx_api_security_events_created (created_at),
    KEY idx_api_security_events_resolved (resolved)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
