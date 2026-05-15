package foundry_vtt

import (
	"context"
	"database/sql"
	"time"
)

// Repository is the foundry_vtt plugin's data layer.
//
// As of C-FMC-5c the per-campaign token table is renamed from
// foundry_module_campaign_tokens (under the deleted foundry_modules
// plugin's namespace) to foundry_vtt_campaign_tokens. Existing token
// rows are preserved by the rename; HMAC verification continues to
// work for already-minted URLs because tokens use this plugin's
// "foundry-vtt:" domain prefix, not the table name.
//
// CampaignsUsingVersion + CampaignsOlderThan are admin-UI queries that
// list which campaigns are pinned to a given Foundry module version.
// Used by the "campaigns using v0.1.5" expandable cards in the admin
// /admin/packages page and by the "notify older campaigns" action.
type Repository interface {
	GetCampaignToken(ctx context.Context, campaignID string) (*CampaignToken, error)
	UpsertCampaignToken(ctx context.Context, t *CampaignToken) error
	BumpCampaignTokenVersion(ctx context.Context, campaignID string) (newVersion int, err error)

	// CampaignsUsingVersion returns campaigns whose FoundryModulePin
	// equals the given version. Joined with users for owner display
	// names. ORDER BY name for stable admin UI rendering.
	CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error)

	// CampaignsOlderThan returns campaigns whose pin is strictly less
	// than the given target per the supplied semver comparator.
	// Campaigns with empty pin (latest-tracking) are excluded — they
	// auto-resolve to latest on next manifest fetch.
	CampaignsOlderThan(ctx context.Context, version string, semverLess func(a, b string) bool) ([]CampaignUsage, error)

	// CampaignsWithEmptyPin returns every campaign whose
	// foundry_module_pin is NULL, missing from settings JSON, or "".
	// These are the auto-tracking campaigns that silently follow
	// whatever foundry-module version is currently installed.
	//
	// Added in C-FMC-6 for the auto-pin install hook and the one-time
	// migration: both flows iterate these campaigns and explicit-pin
	// them to a specific version, so future installs surface the
	// version-spread to the admin instead of silently bumping.
	CampaignsWithEmptyPin(ctx context.Context) ([]CampaignUsage, error)
}

// repository is the default Repository implementation against MariaDB.
type repository struct {
	db *sql.DB
}

// NewRepository constructs a Repository backed by the given DB handle.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

// GetCampaignToken returns the current token row for a campaign, or
// nil if no row has been minted yet (the service mints lazily at
// version=1 on first install URL request).
func (r *repository) GetCampaignToken(ctx context.Context, campaignID string) (*CampaignToken, error) {
	t := &CampaignToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT campaign_id, token_version, rotated_at
		   FROM foundry_vtt_campaign_tokens
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
		INSERT INTO foundry_vtt_campaign_tokens (campaign_id, token_version)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE token_version = VALUES(token_version),
		                        rotated_at = CURRENT_TIMESTAMP`,
		t.CampaignID, t.TokenVersion)
	return err
}

// BumpCampaignTokenVersion atomically increments token_version,
// returning the new value. INSERT...ON DUPLICATE KEY handles the
// cold-start case where a campaign has never minted a token: it
// starts at 2 (so any client that somehow guessed token_version=1
// without a row is also invalidated).
func (r *repository) BumpCampaignTokenVersion(ctx context.Context, campaignID string) (int, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO foundry_vtt_campaign_tokens (campaign_id, token_version)
		VALUES (?, 2)
		ON DUPLICATE KEY UPDATE token_version = token_version + 1,
		                        rotated_at = CURRENT_TIMESTAMP`,
		campaignID)
	if err != nil {
		return 0, err
	}
	var v int
	err = r.db.QueryRowContext(ctx,
		`SELECT token_version FROM foundry_vtt_campaign_tokens WHERE campaign_id = ?`,
		campaignID).Scan(&v)
	return v, err
}

// CampaignsUsingVersion lists campaigns with FoundryModulePin == version.
// JSON_UNQUOTE(JSON_EXTRACT(...)) walks the campaigns.settings JSON to
// the foundry_module_pin field. Keeping the column name as
// "foundry_module_pin" (not "foundry_vtt_pin") matches the campaigns
// plugin's existing CampaignSettings struct from PR #300 — renaming
// that field is out of scope for this PR.
func (r *repository) CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error) {
	return r.queryCampaignUsage(ctx, `
		SELECT c.id, c.name, c.created_by, COALESCE(u.display_name, ''),
		       JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')),
		       c.updated_at
		  FROM campaigns c
		  LEFT JOIN users u ON u.id = c.created_by
		  WHERE JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')) = ?
		  ORDER BY c.name`, version)
}

// CampaignsOlderThan returns campaigns whose pinned version is
// strictly less than `version` per the supplied semver-comparator.
// Two-stage: pull all pinned campaigns, then filter in Go (MySQL
// can't do semver comparison on JSON-extracted strings).
func (r *repository) CampaignsOlderThan(ctx context.Context, version string, semverLess func(a, b string) bool) ([]CampaignUsage, error) {
	rows, err := r.queryCampaignUsage(ctx, `
		SELECT c.id, c.name, c.created_by, COALESCE(u.display_name, ''),
		       JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')),
		       c.updated_at
		  FROM campaigns c
		  LEFT JOIN users u ON u.id = c.created_by
		  WHERE JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')) IS NOT NULL
		    AND JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')) != ''
		    AND JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')) != ?
		  ORDER BY c.name`, version)
	if err != nil {
		return nil, err
	}
	var out []CampaignUsage
	for _, c := range rows {
		if semverLess(c.PinnedTo, version) {
			out = append(out, c)
		}
	}
	return out, nil
}

// CampaignsWithEmptyPin lists every campaign whose pin is NULL,
// missing from the settings JSON, or empty string. These are the
// auto-tracking campaigns C-FMC-6's auto-pin logic targets.
//
// JSON_EXTRACT returns NULL when the key is missing; the OR with
// the empty-string comparison handles both shapes. JSON_UNQUOTE on
// a NULL is a literal "null" string in some MySQL configs, so we
// compare both NULL (the JSON shape) AND "null" (the unquoted shape)
// defensively.
func (r *repository) CampaignsWithEmptyPin(ctx context.Context) ([]CampaignUsage, error) {
	return r.queryCampaignUsage(ctx, `
		SELECT c.id, c.name, c.created_by, COALESCE(u.display_name, ''),
		       JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')),
		       c.updated_at
		  FROM campaigns c
		  LEFT JOIN users u ON u.id = c.created_by
		  WHERE JSON_EXTRACT(c.settings, '$.foundry_module_pin') IS NULL
		     OR JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')) = ''
		     OR JSON_UNQUOTE(JSON_EXTRACT(c.settings, '$.foundry_module_pin')) = 'null'
		  ORDER BY c.name`)
}

// queryCampaignUsage is the shared scanner for the campaign-list
// queries. Columns must match the SELECT projections above.
func (r *repository) queryCampaignUsage(ctx context.Context, query string, args ...any) ([]CampaignUsage, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []CampaignUsage
	for rows.Next() {
		var u CampaignUsage
		var pin sql.NullString
		var lastActive sql.NullTime
		if err := rows.Scan(&u.CampaignID, &u.CampaignName, &u.OwnerUserID, &u.OwnerName, &pin, &lastActive); err != nil {
			return nil, err
		}
		u.PinnedTo = pin.String
		if lastActive.Valid {
			t := lastActive.Time
			u.LastActiveAt = &t
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CampaignUsage is the renderable row for the admin's "campaigns
// using version X" panel. Joined fields come from the queries above.
//
// Defined here (not in model.go) because it's tightly coupled to the
// repository's column projection. Moving it would require keeping
// the SELECT in sync with model.go separately; colocating keeps the
// contract in one file.
type CampaignUsage struct {
	CampaignID   string     `json:"campaign_id"`
	CampaignName string     `json:"campaign_name"`
	OwnerUserID  string     `json:"owner_user_id"`
	OwnerName    string     `json:"owner_name"`
	PinnedTo     string     `json:"pinned_to"`              // version string or "" (latest-tracking)
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}
