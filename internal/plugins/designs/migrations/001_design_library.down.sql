-- Reverses 001_design_library: drops the tables this migration created, in
-- dependency order. Saved designs are lost; copies elsewhere are separate rows.
DROP TABLE IF EXISTS design_share_owners;
DROP TABLE IF EXISTS design_shares;
DROP TABLE IF EXISTS design_library;
