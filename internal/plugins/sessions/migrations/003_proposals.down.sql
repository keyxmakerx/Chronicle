-- Reverse C-SCHED-P2 Slice 2. IF EXISTS keeps the rollback idempotent (safe to
-- re-run / partial-apply recovery). Drop in FK-dependency order: tokens and
-- responses reference options, options reference proposals.
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS slot_proposal_tokens;
DROP TABLE IF EXISTS slot_proposal_responses;
DROP TABLE IF EXISTS slot_proposal_options;
DROP TABLE IF EXISTS slot_proposals;
