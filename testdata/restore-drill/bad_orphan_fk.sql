-- testdata/restore-drill/bad_orphan_fk.sql -- entities.campaign_id points at
-- a campaign that doesn't exist in this dump (loaded with FK checks
-- disabled, same as a real mysqldump restore). Proves the spot FK check
-- in tools/restore-drill.sh actually catches broken referential integrity.
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL);
INSERT INTO schema_migrations (version, dirty) VALUES (30, 0);

CREATE TABLE users (id CHAR(36) PRIMARY KEY, email VARCHAR(255) NOT NULL);
INSERT INTO users (id, email) VALUES ('11111111-1111-1111-1111-111111111111', 'gm@example.test');

CREATE TABLE campaigns (id CHAR(36) PRIMARY KEY, name VARCHAR(255) NOT NULL);
INSERT INTO campaigns (id, name) VALUES ('22222222-2222-2222-2222-222222222222', 'Fixture Campaign');

CREATE TABLE entities (
    id CHAR(36) PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    CONSTRAINT fk_entities_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
);
-- campaign_id below does NOT match any row in campaigns -- orphan, only
-- possible to insert because FOREIGN_KEY_CHECKS is off (as in a real restore).
INSERT INTO entities (id, campaign_id, name)
    VALUES ('33333333-3333-3333-3333-333333333333', '99999999-9999-9999-9999-999999999999', 'Orphaned Hero');

SET FOREIGN_KEY_CHECKS=1;
