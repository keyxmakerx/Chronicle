package foundry_modules

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ErrVersionExists is returned when an admin tries to upload a version
// string that's already in the catalog. Callers translate it to
// apperror.NewConflict for the HTTP boundary.
var ErrVersionExists = errors.New("foundry module version already exists")

// Repository is the persistence boundary for the foundry_modules plugin.
// Single interface for both tables — they're conceptually one feature
// (the catalog + per-campaign token state) and splitting them would
// add a service-layer indirection without a payoff.
type Repository interface {
	// --- foundry_module_versions ---
	InsertVersion(ctx context.Context, v *ModuleVersion) error
	GetVersion(ctx context.Context, version string) (*ModuleVersion, error)
	GetVersionByID(ctx context.Context, id string) (*ModuleVersion, error)
	// GetVersionByGitHubReleaseID is the poller's idempotency check:
	// before ingesting a release it asks "have I already pulled this
	// one?" by release ID. Returns (nil, nil) when not found.
	GetVersionByGitHubReleaseID(ctx context.Context, releaseID int64) (*ModuleVersion, error)
	ListVersions(ctx context.Context, includeWithdrawn bool) ([]*ModuleVersion, error)
	SetVersionStatus(ctx context.Context, version string, status ModuleStatus) error
	LatestAvailable(ctx context.Context) (*ModuleVersion, error)

	// --- foundry_module_campaign_tokens ---
	GetCampaignToken(ctx context.Context, campaignID string) (*CampaignToken, error)
	UpsertCampaignToken(ctx context.Context, t *CampaignToken) error
	BumpCampaignTokenVersion(ctx context.Context, campaignID string) (newVersion int, err error)

	// --- joined views ---
	CampaignsUsingVersion(ctx context.Context, version string) ([]CampaignUsage, error)
	CampaignsOlderThan(ctx context.Context, version string, semverLess func(a, b string) bool) ([]CampaignUsage, error)
}

// repository is the MariaDB implementation.
type repository struct {
	db *sql.DB
}

// NewRepository constructs a Repository backed by the given *sql.DB.
func NewRepository(db *sql.DB) Repository {
	return &repository{db: db}
}

const versionCols = `id, version, file_path, file_size, content_sha256,
	manifest_json, compatibility_minimum, compatibility_verified,
	compatibility_maximum, status, release_notes, uploaded_by_user_id,
	uploaded_at, source, github_release_tag, github_release_id,
	created_at, updated_at`

// scanVersion reads one row into a ModuleVersion. Several columns are
// NULL-able in the schema; we use sql.NullString so an absent
// compatibility floor doesn't blow up the scan.
func scanVersion(s interface{ Scan(...any) error }) (*ModuleVersion, error) {
	v := &ModuleVersion{}
	var compatMin, compatVer, compatMax, notes, ghTag, uploader sql.NullString
	var ghID sql.NullInt64
	err := s.Scan(
		&v.ID, &v.Version, &v.FilePath, &v.FileSize, &v.ContentSHA256,
		&v.ManifestJSON, &compatMin, &compatVer, &compatMax,
		&v.Status, &notes, &uploader,
		&v.UploadedAt, &v.Source, &ghTag, &ghID,
		&v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.CompatibilityMinimum = compatMin.String
	v.CompatibilityVerified = compatVer.String
	v.CompatibilityMaximum = compatMax.String
	v.ReleaseNotes = notes.String
	if uploader.Valid {
		s := uploader.String
		v.UploadedByUserID = &s
	}
	v.GitHubReleaseTag = ghTag.String
	if ghID.Valid {
		id := ghID.Int64
		v.GitHubReleaseID = &id
	}
	return v, nil
}

// InsertVersion writes a new row. Translates the MySQL unique-key
// violation on uk_foundry_module_version (the version string) OR
// uk_github_release (the GitHub release ID) into ErrVersionExists so
// the service can return a clean 409 / the poller can skip silently.
func (r *repository) InsertVersion(ctx context.Context, v *ModuleVersion) error {
	source := v.Source
	if source == "" {
		source = SourceManualUpload
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO foundry_module_versions
			(id, version, file_path, file_size, content_sha256, manifest_json,
			 compatibility_minimum, compatibility_verified, compatibility_maximum,
			 status, release_notes, uploaded_by_user_id,
			 source, github_release_tag, github_release_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.Version, v.FilePath, v.FileSize, v.ContentSHA256, v.ManifestJSON,
		nullableString(v.CompatibilityMinimum),
		nullableString(v.CompatibilityVerified),
		nullableString(v.CompatibilityMaximum),
		string(v.Status), nullableString(v.ReleaseNotes),
		nullableUploader(v.UploadedByUserID),
		string(source),
		nullableString(v.GitHubReleaseTag),
		nullableInt64(v.GitHubReleaseID),
	)
	if err != nil && isDuplicateKey(err) {
		return ErrVersionExists
	}
	return err
}

func (r *repository) GetVersion(ctx context.Context, version string) (*ModuleVersion, error) {
	return scanVersion(r.db.QueryRowContext(ctx,
		`SELECT `+versionCols+` FROM foundry_module_versions WHERE version = ?`, version))
}

func (r *repository) GetVersionByID(ctx context.Context, id string) (*ModuleVersion, error) {
	return scanVersion(r.db.QueryRowContext(ctx,
		`SELECT `+versionCols+` FROM foundry_module_versions WHERE id = ?`, id))
}

// GetVersionByGitHubReleaseID looks up a row by GitHub release ID. Used
// by the poller as an idempotency pre-check so a re-poll doesn't
// trigger a UNIQUE-constraint INSERT failure (which would also work
// but is noisier in the logs).
func (r *repository) GetVersionByGitHubReleaseID(ctx context.Context, releaseID int64) (*ModuleVersion, error) {
	return scanVersion(r.db.QueryRowContext(ctx,
		`SELECT `+versionCols+` FROM foundry_module_versions WHERE github_release_id = ?`, releaseID))
}

// ListVersions returns the catalog ordered newest-uploaded first. When
// includeWithdrawn is false, withdrawn rows are filtered — that's the
// default for owner-facing selectors.
func (r *repository) ListVersions(ctx context.Context, includeWithdrawn bool) ([]*ModuleVersion, error) {
	q := `SELECT ` + versionCols + ` FROM foundry_module_versions`
	if !includeWithdrawn {
		q += ` WHERE status != 'withdrawn'`
	}
	q += ` ORDER BY uploaded_at DESC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ModuleVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		if v != nil {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

func (r *repository) SetVersionStatus(ctx context.Context, version string, status ModuleStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE foundry_module_versions SET status = ? WHERE version = ?`,
		string(status), version)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LatestAvailable returns the catalog's newest non-deprecated, non-
// withdrawn version. The service layer compares by semver; the SQL
// here just gets the newest by upload time among status='available'
// since admins upload in version order in practice and a stale "newest
// by upload date" is still safer than returning a deprecated version.
//
// Returns nil, nil when no available version exists — caller decides
// whether to fall back to the newest deprecated.
func (r *repository) LatestAvailable(ctx context.Context) (*ModuleVersion, error) {
	return scanVersion(r.db.QueryRowContext(ctx,
		`SELECT `+versionCols+`
		   FROM foundry_module_versions
		   WHERE status = 'available'
		   ORDER BY uploaded_at DESC
		   LIMIT 1`))
}

// GetCampaignToken returns the current token row for a campaign, or
// nil if no row has been minted yet (token_version=1 will be created
// lazily by the service on first install URL request).
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

// BumpCampaignTokenVersion atomically increments token_version, returning
// the new value. INSERT...ON DUPLICATE KEY handles the cold-start case
// where a campaign has never minted a token: it starts at 2 (so the old
// implicit "1" is invalidated even though it never had a row, defense
// against any client that somehow guessed token_version=1).
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

// CampaignsUsingVersion returns campaigns whose foundry_module_pin
// equals the given version. Joined with the campaign owner's user
// row so the admin UI can render owner names.
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

// CampaignsOlderThan returns campaigns whose pinned version is older
// than the given target (per the supplied semver-comparison function).
// Used by the "notify all older-version campaigns" admin action.
//
// Campaigns with no pin set (latest-tracking) are excluded — they'll
// pick up the new version automatically on next manifest fetch and
// don't need a notification.
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
	// Filter in Go because MySQL can't do semver comparison.
	var out []CampaignUsage
	for _, c := range rows {
		if semverLess(c.PinnedTo, version) {
			out = append(out, c)
		}
	}
	return out, nil
}

// queryCampaignUsage is the shared scanner for the two campaign-list
// queries above. Keeps the column projection in one place so a future
// schema change touches one spot, not two.
func (r *repository) queryCampaignUsage(ctx context.Context, query string, args ...any) ([]CampaignUsage, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

// --- helpers ---

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableUploader maps a *string (which may be nil for poller-sourced
// rows whose uploaded_by_user_id is NULL) onto the driver's NULL
// representation. The pre-PR-002 schema had this column NOT NULL; the
// 002 migration relaxes it to NULL so the poller can insert without a
// synthetic system user.
func nullableUploader(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

// nullableInt64 maps a *int64 onto the driver's NULL representation.
// Used for github_release_id which is NULL on manual uploads.
func nullableInt64(i *int64) any {
	if i == nil {
		return nil
	}
	return *i
}

// isDuplicateKey reports whether a driver error is a MySQL 1062
// duplicate-key violation. Kept narrow — broader error sniffing
// belongs in the apperror layer, not the repository.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	// MySQL driver errors implement Error() with the code in the
	// message; cheaper to string-match than to type-assert into
	// a driver-specific struct here.
	return strings.Contains(err.Error(), "1062")
}
