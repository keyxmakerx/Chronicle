// Data-shape hygiene checks run on every boot as part of
// RunStartupHealthChecks, catching invariants the FK schema can't enforce
// (rules stored in JSON columns, sub-category nesting depth).
//
// A small registry of HygieneCheck values; the engine iterates them and
// reports counts. Add a new check here when the DB schema can't enforce
// the rule and drift is cheap to detect via SQL; skip it when the check
// would scale with entity count (use a periodic admin job) or the right
// action is auto-fix rather than warn (use a migration or admin action).
//
// All checks are WARN-level: a single inconsistent row must not block a
// boot. Auto-fix is intentionally avoided — most inconsistencies reflect
// intent the engine can't infer, and silently rewriting data is worse
// than the warning.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HygieneCheck is one self-contained data-shape probe.
type HygieneCheck struct {
	// Name is a stable id used in slog and the admin hygiene view.
	// Goes through as `data_hygiene.<name>` in health-check output.
	Name string

	// Description is a one-sentence explanation of the invariant.
	// Used as the success message when count == 0.
	Description string

	// Run executes the probe. Returns:
	//   count: number of violations (0 = healthy).
	//   detail: operator-actionable message ON VIOLATION ONLY; the
	//           Description is used when count == 0.
	//   err: only set if the probe could not execute (DB error,
	//        unsupported feature, etc.). Probe-level failures
	//        downgrade to warnings; they do not block the engine.
	Run func(ctx context.Context, db *sql.DB) (count int, detail string, err error)
}

// hygieneChecks is the registered set, run in order on every boot.
//
// Append a new HygieneCheck here to extend the engine. No other code
// needs to change. Keep checks scoped to invariants the FK schema
// alone can't catch — schema concerns belong in checkCriticalColumns,
// security concerns in checkSecurity, smoke tests in their own
// surface.
var hygieneChecks = []HygieneCheck{
	{
		Name:        "subcat_depth",
		Description: "all sub-categories are exactly one level deep",
		Run:         probeSubcatDepth,
	},
	{
		Name:        "subcat_campaign",
		Description: "all sub-categories share a campaign with their parent",
		Run:         probeSubcatCampaign,
	},
	{
		Name:        "sidebar_subcat_leak",
		Description: "no sidebar_config item references a sub-category entity_type",
		Run:         probeSidebarSubcatLeak,
	},
	{
		Name:        "sidebar_orphan_typeid",
		Description: "all sidebar_config category items reference existing entity_types",
		Run:         probeSidebarOrphanTypeID,
	},
}

// checkDataHygiene runs every registered probe. Each runs independently;
// a single SQL failure downgrades that probe to a warning but does not
// stop the others.
func checkDataHygiene(db *sql.DB, result *HealthCheckResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, check := range hygieneChecks {
		name := "data_hygiene." + check.Name
		count, detail, err := check.Run(ctx, db)
		if err != nil {
			addWarn(result, name, "probe failed: "+err.Error())
			continue
		}
		if count == 0 {
			addPass(result, name, check.Description)
			continue
		}
		addWarn(result, name, fmt.Sprintf("%d violation(s): %s", count, detail))
	}
}

// --- Probes ---

// probeSubcatDepth enforces "sub-cats are exactly one level deep". A
// sub-cat (parent_type_id IS NOT NULL) whose parent is missing or itself
// a sub-cat is a model violation: the sidebar filter, drill panel and
// variant picker all assume single-level nesting. entities/service.go
// rejects this on writes; this probe catches rows that slipped past
// that guard via raw SQL or pre-rule data.
func probeSubcatDepth(ctx context.Context, db *sql.DB) (int, string, error) {
	const query = `
		SELECT COUNT(*) FROM entity_types c
		LEFT JOIN entity_types p ON p.id = c.parent_type_id
		WHERE c.parent_type_id IS NOT NULL
		  AND (p.id IS NULL OR p.parent_type_id IS NOT NULL)
	`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, "", err
	}
	if count == 0 {
		return 0, "", nil
	}
	const detail = "sub-cat entity_type(s) have an invalid parent (missing or itself a sub-cat). " +
		"Inspect: SELECT id, name, campaign_id, parent_type_id FROM entity_types c " +
		"WHERE c.parent_type_id IS NOT NULL AND c.parent_type_id NOT IN " +
		"(SELECT id FROM entity_types WHERE parent_type_id IS NULL); " +
		"Fix: re-parent (UPDATE entity_types SET parent_type_id = <valid_parent_id>) " +
		"or promote to top-level (UPDATE entity_types SET parent_type_id = NULL)."
	return count, detail, nil
}

// probeSubcatCampaign enforces "a sub-cat must share a campaign with its
// parent". The FK schema doesn't constrain parent_type_id to a
// same-campaign row, so raw SQL or a bug could create a cross-campaign
// reference that the render code would display under the wrong campaign.
func probeSubcatCampaign(ctx context.Context, db *sql.DB) (int, string, error) {
	const query = `
		SELECT COUNT(*) FROM entity_types c
		JOIN entity_types p ON p.id = c.parent_type_id
		WHERE c.parent_type_id IS NOT NULL
		  AND c.campaign_id != p.campaign_id
	`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, "", err
	}
	if count == 0 {
		return 0, "", nil
	}
	const detail = "sub-cat entity_type(s) reference a parent in a different campaign — " +
		"this should never happen. Inspect: SELECT c.id AS child_id, c.campaign_id AS child_campaign, " +
		"p.id AS parent_id, p.campaign_id AS parent_campaign FROM entity_types c " +
		"JOIN entity_types p ON p.id = c.parent_type_id WHERE c.campaign_id != p.campaign_id;"
	return count, detail, nil
}

// probeSidebarSubcatLeak finds persisted sidebar_config rows whose items
// reference a sub-category entity_type. Sub-cats are template variants
// (ADR: sub-cats-as-templates) and must never be SidebarItems; the
// render-time filter drops them silently, but their persistence signals
// drift worth surfacing.
//
// Uses MariaDB's JSON_TABLE (requires 10.11+). When sidebar_config is
// NULL or empty, JSON_EXTRACT returns NULL and JSON_TABLE produces no
// rows — safe no-op.
func probeSidebarSubcatLeak(ctx context.Context, db *sql.DB) (int, string, error) {
	const query = `
		SELECT COUNT(*) FROM campaigns c,
		JSON_TABLE(JSON_EXTRACT(c.sidebar_config, '$.items'), '$[*]'
			COLUMNS (
				item_type VARCHAR(20) PATH '$.type',
				type_id INT PATH '$.type_id'
			)) AS items
		JOIN entity_types et ON et.id = items.type_id
		WHERE c.sidebar_config IS NOT NULL
		  AND c.sidebar_config != ''
		  AND JSON_VALID(c.sidebar_config)
		  AND items.item_type = 'category'
		  AND et.parent_type_id IS NOT NULL
	`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, "", err
	}
	if count == 0 {
		return 0, "", nil
	}
	const detail = "sidebar_config item(s) reference sub-category entity_types. Sub-cats are " +
		"template variants and should never appear as SidebarItems. The render-time filter " +
		"drops them silently; persistence indicates past auto-add drift. Cleanup: " +
		"the admin hygiene UI is the right home for one-click pruning."
	return count, detail, nil
}

// probeSidebarOrphanTypeID finds sidebar_config items whose type_id
// references a deleted entity_type. The render-time filter drops orphans
// silently; this surfaces the drift so it can be cleaned up.
func probeSidebarOrphanTypeID(ctx context.Context, db *sql.DB) (int, string, error) {
	const query = `
		SELECT COUNT(*) FROM campaigns c,
		JSON_TABLE(JSON_EXTRACT(c.sidebar_config, '$.items'), '$[*]'
			COLUMNS (
				item_type VARCHAR(20) PATH '$.type',
				type_id INT PATH '$.type_id'
			)) AS items
		LEFT JOIN entity_types et ON et.id = items.type_id
		WHERE c.sidebar_config IS NOT NULL
		  AND c.sidebar_config != ''
		  AND JSON_VALID(c.sidebar_config)
		  AND items.item_type = 'category'
		  AND items.type_id IS NOT NULL
		  AND et.id IS NULL
	`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, "", err
	}
	if count == 0 {
		return 0, "", nil
	}
	const detail = "sidebar_config item(s) reference deleted entity_types. " +
		"Render-time filter drops them silently; admin hygiene UI is the right home for cleanup."
	return count, detail, nil
}
