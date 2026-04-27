-- Heal addon status divergence: in-code addon definitions
-- (builtinAddons in internal/plugins/addons/service.go) are the source of
-- truth, but a latent bug in repository.go:Upsert excluded `status` from
-- ON DUPLICATE KEY UPDATE, so when an addon flipped from `planned` to
-- `active` in a release, existing DB rows kept the stale status forever.
--
-- Concrete symptom: PR #263 flipped `player-notes` to StatusActive, but
-- existing deployments kept `status='planned'` in the DB. The toggle UI
-- showed the addon, but EnableForCampaign rejected with "only active
-- addons can be enabled" and the entity_notes block silently never
-- rendered because the addon-gate check returned `false`.
--
-- Repository.go:Upsert is now fixed to include `status` in the UPDATE
-- clause; this migration heals the data for deployments that ran the
-- buggy code. Deployments fresh from this migration onward are correct
-- by construction.
--
-- Scope: only widget/integration/plugin addons that are known-Active
-- in the current release. We DO NOT touch system-category addons
-- because their `planned/active` distinction is meaningful (a
-- `planned` system has no backing data and shouldn't be enableable).
-- Same heal pattern as 000021 used for system addons; this one extends
-- it to widgets/integrations.
--
-- Idempotent: re-running this migration on a healed DB is a no-op.

UPDATE addons SET status = 'active'
WHERE slug IN (
    'player-notes',
    -- Defensive belt-and-braces for the widget/integration addons that
    -- have backing code today. If any of these were ever seeded as
    -- planned by an old baseline, this heals them in one shot.
    'notes',
    'attributes',
    'sync-api'
)
AND status = 'planned';
