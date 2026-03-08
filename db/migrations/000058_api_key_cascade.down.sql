-- Rollback Sprint S-1: Remove FK constraints added for campaign deletion cleanup.

ALTER TABLE api_request_log
  DROP FOREIGN KEY fk_api_request_log_campaign;

ALTER TABLE api_request_log
  MODIFY COLUMN campaign_id VARCHAR(36) NOT NULL;

ALTER TABLE api_keys
  DROP FOREIGN KEY fk_api_keys_campaign;
