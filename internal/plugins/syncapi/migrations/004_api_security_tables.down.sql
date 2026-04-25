-- Drops both security tables. This is safe because:
--   - IsIPBlocked and LogSecurityEvent both fail open when the tables are
--     missing, so authenticated API traffic is unaffected.
--   - Rolling back loses historical IP blocks and security-event audit
--     trails; that data is admin-scoped and not reconstructible.
--
-- Rollback is therefore fine for dev but should be avoided in production
-- once operators start relying on the blocklist and event log.

DROP TABLE IF EXISTS api_security_events;
DROP TABLE IF EXISTS api_ip_blocklist;
