// Package campaigns — smoketest.go provides a startup probe that verifies the
// campaign SELECT+Scan pattern works. Co-located with repository.go so that
// any change to the query or struct fields is immediately adjacent to the
// smoke test that validates them.
package campaigns

import (
	"database/sql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// campaignScanColumns is the canonical column list for campaign queries.
// Defined once so the smoke test and repository stay in sync. If a migration
// adds a column to this list, the Scan in both repository.go and this file
// must be updated — and the smoke test will catch it if they diverge.
const campaignScanColumns = `id, name, slug, description, is_public,
	settings, backdrop_path, sidebar_config, dashboard_layout,
	owner_dashboard_layout, created_by, created_at, updated_at,
	archived_at, join_code`

// ScanSmokeTest returns a startup probe that runs a real SELECT + Scan on the
// campaigns table. If the column list in the query doesn't match the Campaign
// struct's Scan destinations, this fails at startup instead of when a user
// visits the campaigns page.
func ScanSmokeTest() database.SmokeTest {
	return database.SmokeTest{
		Name: "campaigns.scan",
		Fn: func(db *sql.DB) error {
			query := `SELECT ` + campaignScanColumns + ` FROM campaigns LIMIT 1`
			var c Campaign
			return db.QueryRow(query).Scan(
				&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
				&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
				&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
			)
		},
	}
}
