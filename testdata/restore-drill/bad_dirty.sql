-- testdata/restore-drill/bad_dirty.sql -- schema_migrations.dirty=1.
-- Simulates a backup taken mid-migration. Proves tools/restore-drill.sh's
-- FAIL path actually fires for this condition.
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL);
INSERT INTO schema_migrations (version, dirty) VALUES (30, 1);

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
INSERT INTO entities (id, campaign_id, name)
    VALUES ('33333333-3333-3333-3333-333333333333', '22222222-2222-2222-2222-222222222222', 'Fixture Hero');

SET FOREIGN_KEY_CHECKS=1;
