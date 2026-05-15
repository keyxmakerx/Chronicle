package foundry_vtt

import (
	"context"
	"database/sql"
)

// Repository is the foundry_vtt plugin's data layer. Currently only
// manages the per-campaign token table; everything else (packages,
// pin storage on CampaignSettings) is owned by other plugins and
// reached via interface adapters.
//
// NOTE on table name during the C-FMC-5b parallel period: this
// plugin reads/writes the EXISTING `foundry_module_campaign_tokens`
// table (created by foundry_modules' migration 001). The table is
// renamed to `foundry_vtt_campaign_tokens` in C-FMC-5c when the
// foundry_modules plugin is deleted. Both plugins coexist on the
// same physical table during C-FMC-5b — token rotation in one
// plugin is visible to the other, by design.
//
// The literal table name `foundry_module_campaign_tokens` in the
// queries below is the source-of-truth for what gets renamed.
type Repository interface {
	GetCampaignToken(ctx context.Context, campaignID string) (*CampaignToken, error)
	UpsertCampaignToken(ctx context.Context, t *CampaignToken) error
	BumpCampaignTokenVersion(ctx context.Context, campaignID string) (newVersion int, err error)
}

// repository is the default Repository implementation against a
// MariaDB *sql.DB. Hand-written SQL per the project conventions —
// no ORM.
type repository struct {
	db *sql.DB
}

// NewRepository constructs a Repository backed by the given DB
// handle.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

// GetCampaignToken returns the current token row for a campaign,
// or nil if no row has been minted yet (token_version=1 will be
// created lazily by the service on first install URL request).
//
// Reads from foundry_module_campaign_tokens during the C-FMC-5b
// parallel period. C-FMC-5c renames to foundry_vtt_campaign_tokens.
func (r *repository) GetCampaignToken(ctx context.Context, campaignID string) (*CampaignToken, error) {
	t := &CampaignToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT campaign_id, token_version, rotated_at
		   FROM foundry_module_campaign_tokens
		   WHERE campaign_id = ?`, campaignID,
	).Scan(&t.CampaignID, &t.TokenVersion, &t.RotatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// UpsertCampaignToken creates or updates a token row. The service
// uses this for the initial mint (token_version=1) on the first
// install-URL request.
func (r *repository) UpsertCampaignToken(ctx context.Context, t *CampaignToken) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO foundry_module_campaign_tokens (campaign_id, token_version)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE token_version = VALUES(token_version),
		                        rotated_at = CURRENT_TIMESTAMP`,
		t.CampaignID, t.TokenVersion)
	return err
}

// BumpCampaignTokenVersion atomically increments token_version,
// returning the new value. INSERT...ON DUPLICATE KEY handles the
// cold-start case where a campaign has never minted a token: it
// starts at 2 (so the old implicit "1" is invalidated even though
// it never had a row).
func (r *repository) BumpCampaignTokenVersion(ctx context.Context, campaignID string) (int, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO foundry_module_campaign_tokens (campaign_id, token_version)
		VALUES (?, 2)
		ON DUPLICATE KEY UPDATE token_version = token_version + 1,
		                        rotated_at = CURRENT_TIMESTAMP`,
		campaignID)
	if err != nil {
		return 0, err
	}
	var v int
	err = r.db.QueryRowContext(ctx,
		`SELECT token_version FROM foundry_module_campaign_tokens WHERE campaign_id = ?`,
		campaignID).Scan(&v)
	return v, err
}
