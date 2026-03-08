-- Sprint S-1: Add FK constraints for campaign deletion cleanup (ADR-025).
--
-- api_keys: add CASCADE FK so keys are deleted when campaign is deleted.
-- api_request_log: make campaign_id nullable, add SET NULL FK to retain audit trail.
--
-- Before adding FK on api_keys, clean up any orphaned rows that reference
-- deleted campaigns (otherwise the FK constraint creation will fail).

DELETE FROM api_keys WHERE campaign_id NOT IN (SELECT id FROM campaigns);

ALTER TABLE api_keys
  ADD CONSTRAINT fk_api_keys_campaign
  FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE;

-- api_request_log.campaign_id is NOT NULL — must change to NULLable first.
-- Clean up orphaned rows is not needed since we use SET NULL (not CASCADE).
ALTER TABLE api_request_log
  MODIFY COLUMN campaign_id VARCHAR(36) NULL;

-- Only add FK if there are no orphaned rows referencing deleted campaigns.
-- SET NULL will handle future deletions; clean up existing orphans first.
UPDATE api_request_log SET campaign_id = NULL
  WHERE campaign_id NOT IN (SELECT id FROM campaigns);

ALTER TABLE api_request_log
  ADD CONSTRAINT fk_api_request_log_campaign
  FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE SET NULL;
