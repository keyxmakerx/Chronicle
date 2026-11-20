-- Reverse 000064: the tables hold only rules and requests this migration
-- introduced, so dropping them loses nothing else.
DROP TABLE IF EXISTS campaign_choice_requests;
DROP TABLE IF EXISTS campaign_choice_rules;
