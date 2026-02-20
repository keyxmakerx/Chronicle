-- Sync API tables: API key management, request logging, and rate limiting.
-- Supports the generic sync API addon that external clients (Foundry VTT, etc.) use.

-- API keys: each key belongs to a user and is scoped to a campaign.
-- Keys are bcrypt-hashed for storage; the plaintext is only shown once at creation.
CREATE TABLE IF NOT EXISTS api_keys (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    key_hash    VARCHAR(255) NOT NULL,                            -- bcrypt hash of the API key.
    key_prefix  VARCHAR(8)   NOT NULL,                            -- First 8 chars for identification.
    name        VARCHAR(100) NOT NULL,                            -- Human-readable label (e.g. "Foundry VTT Sync").
    user_id     VARCHAR(36)  NOT NULL,                            -- Owner of this key.
    campaign_id VARCHAR(36)  NOT NULL,                            -- Campaign this key can access.
    permissions JSON         DEFAULT NULL,                        -- Allowed operations: ["read", "write", "sync"].
    ip_allowlist JSON        DEFAULT NULL,                        -- Optional IP whitelist (CIDR notation).
    rate_limit  INT          NOT NULL DEFAULT 60,                 -- Max requests per minute.
    is_active   TINYINT(1)   NOT NULL DEFAULT 1,                  -- Soft disable without deleting.
    last_used_at DATETIME    DEFAULT NULL,                        -- Last successful API call.
    last_used_ip VARCHAR(45) DEFAULT NULL,                        -- IP of last API call (IPv4/IPv6).
    expires_at  DATETIME     DEFAULT NULL,                        -- Optional expiry (NULL = no expiry).
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_api_keys_prefix (key_prefix),
    KEY idx_api_keys_user (user_id),
    KEY idx_api_keys_campaign (campaign_id),
    KEY idx_api_keys_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- API request log: records every API call for auditing and monitoring.
-- Partitioned by date for efficient cleanup of old entries.
CREATE TABLE IF NOT EXISTS api_request_log (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    api_key_id  INT          NOT NULL,                            -- Which key was used.
    campaign_id VARCHAR(36)  NOT NULL,                            -- Campaign accessed.
    user_id     VARCHAR(36)  NOT NULL,                            -- Key owner.
    method      VARCHAR(10)  NOT NULL,                            -- HTTP method.
    path        VARCHAR(500) NOT NULL,                            -- Request path.
    status_code INT          NOT NULL,                            -- HTTP response status.
    ip_address  VARCHAR(45)  NOT NULL,                            -- Client IP address.
    user_agent  VARCHAR(500) DEFAULT NULL,                        -- Client user agent string.
    request_size  INT        DEFAULT 0,                           -- Request body size in bytes.
    response_size INT        DEFAULT 0,                           -- Response body size in bytes.
    duration_ms   INT        DEFAULT 0,                           -- Request processing time.
    error_message VARCHAR(500) DEFAULT NULL,                      -- Error details if request failed.
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_api_log_key (api_key_id),
    KEY idx_api_log_campaign (campaign_id),
    KEY idx_api_log_created (created_at),
    KEY idx_api_log_ip (ip_address),
    KEY idx_api_log_status (status_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- API security events: tracks blocked requests, rate limit hits, auth failures.
-- Used by admin dashboard for security monitoring.
CREATE TABLE IF NOT EXISTS api_security_events (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    event_type  VARCHAR(50)  NOT NULL,                            -- "rate_limit", "auth_failure", "ip_blocked", "key_expired", "suspicious".
    api_key_id  INT          DEFAULT NULL,                        -- Which key (NULL if auth failed before key identified).
    campaign_id VARCHAR(36)  DEFAULT NULL,
    ip_address  VARCHAR(45)  NOT NULL,
    user_agent  VARCHAR(500) DEFAULT NULL,
    details     JSON         DEFAULT NULL,                        -- Event-specific details.
    resolved    TINYINT(1)   NOT NULL DEFAULT 0,                  -- Admin has reviewed/resolved this event.
    resolved_by VARCHAR(36)  DEFAULT NULL,                        -- Admin who resolved it.
    resolved_at DATETIME     DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_security_type (event_type),
    KEY idx_security_ip (ip_address),
    KEY idx_security_created (created_at),
    KEY idx_security_resolved (resolved)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- IP blocklist: admin-managed list of blocked IPs.
-- Checked on every API request before processing.
CREATE TABLE IF NOT EXISTS api_ip_blocklist (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    ip_address  VARCHAR(45)  NOT NULL,                            -- IP or CIDR range to block.
    reason      VARCHAR(500) DEFAULT NULL,                        -- Why this IP was blocked.
    blocked_by  VARCHAR(36)  NOT NULL,                            -- Admin who added the block.
    expires_at  DATETIME     DEFAULT NULL,                        -- Optional auto-expiry (NULL = permanent).
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_blocklist_ip (ip_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
