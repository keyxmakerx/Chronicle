// admin_list.go holds the admin campaign-list search: the game-system chip
// vocabulary, the WHERE builder and the two queries behind the search box,
// chips and pagination. Kept apart from repository.go so the query shape can
// be tested without a database.

package campaigns

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/database"
)

const (
	// SystemFilterNone selects campaigns with no game system set.
	SystemFilterNone = "none"
	// SystemFilterCustom selects campaigns on a user-supplied ("custom:<url>")
	// system. They collapse into one chip because every URL is its own id and
	// a chip per URL would never stay small.
	SystemFilterCustom = "custom"
)

// campaignSystemExpr reads the game system out of the settings JSON. It is a
// fixed fragment (no user text), reused by the filter and the grouping so
// the two can never disagree about which campaign belongs to which chip.
const campaignSystemExpr = `COALESCE(JSON_VALUE(settings, '$.system_id'), '')`

// AdminSearchOptions are the already-validated inputs of the admin
// campaign list.
type AdminSearchOptions struct {
	// Query matches the campaign name as a case-insensitive substring.
	Query string
	// System is "" (all), SystemFilterNone, SystemFilterCustom or a system id.
	System string
	ListOptions
}

// SystemCount is one filter chip: a system key and how many campaigns it has.
type SystemCount struct {
	// Key is a system id, SystemFilterNone or SystemFilterCustom.
	Key   string
	Count int
}

// buildAdminCampaignWhere returns the WHERE clause (including the keyword,
// or empty) and its placeholder args. User text only ever travels as an arg.
func buildAdminCampaignWhere(query, system string) (string, []any) {
	// A campaign in the Trash is listed on the Trash page, not here.
	conds := []string{"deleted_at IS NULL"}
	var args []any
	if query != "" {
		conds = append(conds, fmt.Sprintf("name LIKE ? ESCAPE '%s'", database.LikeEscapeChar))
		args = append(args, database.ContainsPattern(query))
	}
	switch system {
	case "":
	case SystemFilterNone:
		conds = append(conds, campaignSystemExpr+" = ''")
	case SystemFilterCustom:
		conds = append(conds, campaignSystemExpr+" LIKE 'custom:%'")
	default:
		conds = append(conds, campaignSystemExpr+" = ?")
		args = append(args, system)
	}
	clause := " WHERE " + conds[0]
	for _, c := range conds[1:] {
		clause += " AND " + c
	}
	return clause, args
}

// CountBySystem returns one count per game-system chip for a name search, in
// a single GROUP BY query, most-populated first. The counts ignore the
// active chip on purpose: they show what each chip would list.
func (r *campaignRepository) CountBySystem(ctx context.Context, query string) ([]SystemCount, error) {
	where, args := buildAdminCampaignWhere(query, "")
	q := `SELECT CASE
	               WHEN ` + campaignSystemExpr + ` = '' THEN '` + SystemFilterNone + `'
	               WHEN ` + campaignSystemExpr + ` LIKE 'custom:%' THEN '` + SystemFilterCustom + `'
	               ELSE ` + campaignSystemExpr + `
	             END AS system_key, COUNT(*)
	      FROM campaigns` + where + ` GROUP BY system_key ORDER BY COUNT(*) DESC, system_key ASC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("counting campaigns by system: %w", err)
	}
	defer rows.Close()

	var out []SystemCount
	for rows.Next() {
		var sc SystemCount
		if err := rows.Scan(&sc.Key, &sc.Count); err != nil {
			return nil, fmt.Errorf("scanning system count: %w", err)
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// SearchAll returns one page of campaigns matching the name search and
// system chip, most recently updated first. Admin only.
func (r *campaignRepository) SearchAll(ctx context.Context, opts AdminSearchOptions) ([]Campaign, error) {
	where, args := buildAdminCampaignWhere(opts.Query, opts.System)
	q := `SELECT id, name, slug, description, is_public, settings, backdrop_path, sidebar_config, dashboard_layout, owner_dashboard_layout, created_by, created_at, updated_at, archived_at, join_code
	      FROM campaigns` + where + ` ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, opts.PerPage, opts.Offset())

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("searching campaigns: %w", err)
	}
	defer rows.Close()

	var out []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsPublic,
			&c.Settings, &c.BackdropPath, &c.SidebarConfig, &c.DashboardLayout, &c.OwnerDashboardLayout,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.JoinCode,
		); err != nil {
			return nil, fmt.Errorf("scanning campaign row: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SearchAll returns one page of campaigns for the admin list. Admin only.
func (s *campaignService) SearchAll(ctx context.Context, opts AdminSearchOptions) ([]Campaign, error) {
	if opts.PerPage < 1 || opts.PerPage > 100 {
		opts.PerPage = 24
	}
	if opts.Page < 1 {
		opts.Page = 1
	}
	return s.repo.SearchAll(ctx, opts)
}

// CountBySystem returns the per-system chip counts for a name search.
// Admin only.
func (s *campaignService) CountBySystem(ctx context.Context, query string) ([]SystemCount, error) {
	return s.repo.CountBySystem(ctx, query)
}
