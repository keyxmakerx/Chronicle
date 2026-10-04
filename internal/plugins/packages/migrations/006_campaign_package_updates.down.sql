-- Dropping the table discards every campaign's update choice, which reverts
-- all campaigns to "automatic" (the behaviour before this table existed).
DROP TABLE IF EXISTS campaign_package_updates;
