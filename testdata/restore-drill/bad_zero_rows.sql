-- testdata/restore-drill/bad_zero_rows.sql -- campaigns table exists but is
-- empty. Proves the "row counts > 0 for core tables" check in
-- tools/restore-drill.sh actually catches an empty/truncated dump.
SET FOREIGN_KEY_CHECKS=0;

CREATE TABLE schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL);
INSERT INTO schema_migrations (version, dirty) VALUES (30, 0);

CREATE TABLE users (id CHAR(36) PRIMARY KEY, email VARCHAR(255) NOT NULL);
INSERT INTO users (id, email) VALUES ('11111111-1111-1111-1111-111111111111', 'gm@example.test');

CREATE TABLE campaigns (id CHAR(36) PRIMARY KEY, name VARCHAR(255) NOT NULL);
-- intentionally no rows inserted

CREATE TABLE entities (
    id CHAR(36) PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    CONSTRAINT fk_entities_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
);

SET FOREIGN_KEY_CHECKS=1;
