-- Drop owner_user_id and its associated FK + index. Order matters:
-- FK first (it depends on the column), then index, then column.
ALTER TABLE entities DROP FOREIGN KEY fk_entities_owner_user;
ALTER TABLE entities DROP INDEX idx_entities_campaign_owner;
ALTER TABLE entities DROP COLUMN owner_user_id;
