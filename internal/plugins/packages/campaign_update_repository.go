// campaign_update_repository.go — SQL for campaign_package_updates.
//
// One repository per aggregate root: this table is keyed by (campaign,
// package) and owned by the packages plugin, so it lives beside the package
// repository rather than inside it, which keeps PackageRepository's many
// fakes untouched.
package packages

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CampaignUpdateRow is one stored row. Version, HeldVersion and
// DismissedVersion are "" when NULL, and the times are nil.
type CampaignUpdateRow struct {
	CampaignID   string
	CampaignName string // Filled by the list queries only; "" from Get.
	PackageID    string
	Mode         UpdateMode
	Version      string
	HeldVersion  string

	// HeldAt is when HeldVersion was first offered, so the admin can see how
	// long an owner has been asked.
	HeldAt *time.Time

	// DismissedVersion is the waiting version the owner answered "Later" to.
	// It hides the owner's prompt for that version only: a newer one asks
	// again.
	DismissedVersion string

	// AdminHold is set while a site admin is keeping the campaign on its
	// version; the owner cannot move it until the admin lets go.
	AdminHold   bool
	AdminHoldAt *time.Time
}

// CampaignUpdateRepository is the data layer for per-campaign update choices.
type CampaignUpdateRepository interface {
	// Get returns the row, or (nil, nil) when the campaign has no choice
	// stored (meaning automatic).
	Get(ctx context.Context, campaignID, packageID string) (*CampaignUpdateRow, error)

	// ListByPackage returns every stored row for a package.
	ListByPackage(ctx context.Context, packageID string) ([]CampaignUpdateRow, error)

	// ListHeld returns every row that has a held version, across packages.
	ListHeld(ctx context.Context) ([]CampaignUpdateRow, error)

	// ListKeptVersions returns every non-empty version or held version stored
	// for a SYSTEM package, keyed by package slug then version. Clean-up uses
	// it to protect every version some campaign is on.
	ListKeptVersions(ctx context.Context) (map[string]map[string]bool, error)

	// UpsertModeVersion writes mode and version and leaves held_version alone.
	UpsertModeVersion(ctx context.Context, campaignID, packageID string, mode UpdateMode, version string) error

	// SetHeld records (or with "" clears) the held version. Clearing never
	// creates a row. Offering a different version restarts HeldAt; offering the
	// same one again keeps it.
	SetHeld(ctx context.Context, campaignID, packageID, held string) error

	// SetDismissed records the waiting version the owner chose "Later" for.
	SetDismissed(ctx context.Context, campaignID, packageID, version string) error

	// SetAdminHold turns the site admin's hold on or off. Turning it off never
	// creates a row.
	SetAdminHold(ctx context.Context, campaignID, packageID string, hold bool) error

	// DeleteDefaultRows removes rows that say nothing (automatic, no version,
	// no held version, no dismissal, no admin hold) and reports how many went.
	DeleteDefaultRows(ctx context.Context) (int64, error)
}

type campaignUpdateRepository struct {
	db *sql.DB
}

// NewCampaignUpdateRepository builds the MariaDB implementation.
func NewCampaignUpdateRepository(db *sql.DB) CampaignUpdateRepository {
	return &campaignUpdateRepository{db: db}
}

const campaignUpdateColumns = `campaign_id, '', package_id, update_mode, COALESCE(version,''), COALESCE(held_version,''),
	held_at, COALESCE(dismissed_version,''), admin_hold, admin_hold_at`

// campaignUpdateListColumns adds the campaign's name for the admin lists; the
// campaigns table is read-only here, as in GetUsageByCampaign.
const campaignUpdateListColumns = `u.campaign_id, COALESCE(c.name,''), u.package_id, u.update_mode, COALESCE(u.version,''), COALESCE(u.held_version,''),
	u.held_at, COALESCE(u.dismissed_version,''), u.admin_hold, u.admin_hold_at`

func scanCampaignUpdate(sc interface{ Scan(dest ...any) error }) (CampaignUpdateRow, error) {
	var r CampaignUpdateRow
	var mode string
	var heldAt, holdAt sql.NullTime
	if err := sc.Scan(&r.CampaignID, &r.CampaignName, &r.PackageID, &mode, &r.Version, &r.HeldVersion,
		&heldAt, &r.DismissedVersion, &r.AdminHold, &holdAt); err != nil {
		return r, err
	}
	r.Mode = UpdateMode(mode)
	if heldAt.Valid {
		r.HeldAt = &heldAt.Time
	}
	if holdAt.Valid {
		r.AdminHoldAt = &holdAt.Time
	}
	return r, nil
}

func (r *campaignUpdateRepository) Get(ctx context.Context, campaignID, packageID string) (*CampaignUpdateRow, error) {
	row, err := scanCampaignUpdate(r.db.QueryRowContext(ctx,
		`SELECT `+campaignUpdateColumns+` FROM campaign_package_updates
		  WHERE campaign_id = ? AND package_id = ?`, campaignID, packageID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting campaign update row: %w", err)
	}
	return &row, nil
}

func (r *campaignUpdateRepository) queryRows(ctx context.Context, query string, args ...any) ([]CampaignUpdateRow, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing campaign update rows: %w", err)
	}
	defer rows.Close()
	var out []CampaignUpdateRow
	for rows.Next() {
		row, err := scanCampaignUpdate(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning campaign update row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *campaignUpdateRepository) ListByPackage(ctx context.Context, packageID string) ([]CampaignUpdateRow, error) {
	return r.queryRows(ctx,
		`SELECT `+campaignUpdateListColumns+` FROM campaign_package_updates u
		  LEFT JOIN campaigns c ON c.id = u.campaign_id
		  WHERE u.package_id = ? ORDER BY c.name, u.campaign_id`,
		packageID)
}

func (r *campaignUpdateRepository) ListHeld(ctx context.Context) ([]CampaignUpdateRow, error) {
	return r.queryRows(ctx,
		`SELECT `+campaignUpdateListColumns+` FROM campaign_package_updates u
		  LEFT JOIN campaigns c ON c.id = u.campaign_id
		  WHERE u.held_version IS NOT NULL AND u.held_version <> ''
		  ORDER BY u.package_id, c.name, u.campaign_id`)
}

func (r *campaignUpdateRepository) ListKeptVersions(ctx context.Context) (map[string]map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.slug, COALESCE(u.version,''), COALESCE(u.held_version,'')
		   FROM campaign_package_updates u
		   INNER JOIN packages p ON p.id = u.package_id
		  WHERE p.type = 'system'`)
	if err != nil {
		return nil, fmt.Errorf("listing kept versions: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]bool{}
	add := func(slug, v string) {
		if v == "" {
			return
		}
		if out[slug] == nil {
			out[slug] = map[string]bool{}
		}
		out[slug][v] = true
	}
	for rows.Next() {
		var slug, version, held string
		if err := rows.Scan(&slug, &version, &held); err != nil {
			return nil, fmt.Errorf("scanning kept version: %w", err)
		}
		add(slug, version)
		add(slug, held)
	}
	return out, rows.Err()
}

func (r *campaignUpdateRepository) UpsertModeVersion(ctx context.Context, campaignID, packageID string, mode UpdateMode, version string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO campaign_package_updates (campaign_id, package_id, update_mode, version)
		VALUES (?, ?, ?, NULLIF(?, ''))
		ON DUPLICATE KEY UPDATE update_mode = VALUES(update_mode), version = VALUES(version)`,
		campaignID, packageID, string(mode), version)
	if err != nil {
		return fmt.Errorf("writing campaign update mode: %w", err)
	}
	return nil
}

func (r *campaignUpdateRepository) SetHeld(ctx context.Context, campaignID, packageID, held string) error {
	var err error
	if held == "" {
		_, err = r.db.ExecContext(ctx,
			`UPDATE campaign_package_updates SET held_version = NULL, held_at = NULL
			  WHERE campaign_id = ? AND package_id = ?`, campaignID, packageID)
	} else {
		// held_at is assigned first: MySQL and MariaDB evaluate the
		// assignments left to right, so it still compares against the old
		// held_version and only moves when the offered version changed.
		_, err = r.db.ExecContext(ctx, `
			INSERT INTO campaign_package_updates (campaign_id, package_id, held_version, held_at)
			VALUES (?, ?, ?, NOW())
			ON DUPLICATE KEY UPDATE
			  held_at = IF(held_version <=> VALUES(held_version) AND held_at IS NOT NULL, held_at, NOW()),
			  held_version = VALUES(held_version)`,
			campaignID, packageID, held)
	}
	if err != nil {
		return fmt.Errorf("writing held version: %w", err)
	}
	return nil
}

func (r *campaignUpdateRepository) SetDismissed(ctx context.Context, campaignID, packageID, version string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO campaign_package_updates (campaign_id, package_id, dismissed_version)
		VALUES (?, ?, NULLIF(?, ''))
		ON DUPLICATE KEY UPDATE dismissed_version = VALUES(dismissed_version)`,
		campaignID, packageID, version)
	if err != nil {
		return fmt.Errorf("writing dismissed version: %w", err)
	}
	return nil
}

func (r *campaignUpdateRepository) SetAdminHold(ctx context.Context, campaignID, packageID string, hold bool) error {
	var err error
	if !hold {
		_, err = r.db.ExecContext(ctx,
			`UPDATE campaign_package_updates SET admin_hold = 0, admin_hold_at = NULL
			  WHERE campaign_id = ? AND package_id = ?`, campaignID, packageID)
	} else {
		_, err = r.db.ExecContext(ctx, `
			INSERT INTO campaign_package_updates (campaign_id, package_id, admin_hold, admin_hold_at)
			VALUES (?, ?, 1, NOW())
			ON DUPLICATE KEY UPDATE
			  admin_hold_at = IF(admin_hold = 1 AND admin_hold_at IS NOT NULL, admin_hold_at, NOW()),
			  admin_hold = 1`,
			campaignID, packageID)
	}
	if err != nil {
		return fmt.Errorf("writing admin hold: %w", err)
	}
	return nil
}

func (r *campaignUpdateRepository) DeleteDefaultRows(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_package_updates
		  WHERE update_mode = 'automatic'
		    AND (version IS NULL OR version = '')
		    AND (held_version IS NULL OR held_version = '')
		    AND (dismissed_version IS NULL OR dismissed_version = '')
		    AND admin_hold = 0`)
	if err != nil {
		return 0, fmt.Errorf("deleting default update rows: %w", err)
	}
	return res.RowsAffected()
}
