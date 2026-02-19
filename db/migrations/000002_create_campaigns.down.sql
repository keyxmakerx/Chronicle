-- Migration: 000002_create_campaigns (rollback)
-- Description: Drops campaigns, campaign_members, and ownership_transfers.

DROP TABLE IF EXISTS ownership_transfers;
DROP TABLE IF EXISTS campaign_members;
DROP TABLE IF EXISTS campaigns;
