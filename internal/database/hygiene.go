// Package database — hygiene.go
// Data-shape hygiene checks that run on every Chronicle boot as part of
// RunStartupHealthChecks. These complement the schema-level checks
// (migration version, critical columns) by catching invariants the FK
// schema can't express on its own — primarily the entity-type / sidebar
// model rules introduced by ADR-031 (sub-categories as template variants)
// and PR #241 (subcategories-as-templates rewrite).
//
// All checks here are WARN-level by design. A single inconsistent row
// must not block a boot — the running server stays serviceable, the
// admin sees a warning in the logs (and eventually in the admin
// hygiene dashboard), and they fix via SQL or a re-parent UI. Auto-fix
// is intentionally avoided here: most inconsistencies reflect intent
// the engine can't infer (e.g. "this top-level type was meant to be a
// sub-cat") and silently rewriting parent_type_id would be worse than
// the warning.
//
// Add a new check here when:
//   - The DB schema can't enforce the rule (FK + types alone).
//   - Detecting drift cheaply via SQL is feasible (sub-second on a
//     reasonable campaign size).
//   - Operators benefit from knowing about it on every boot, not only
//     when triaging a user complaint.
//
// Don't add a check here when:
//   - It requires walking large JSON blobs or doing per-row work
//     that scales with entity count (move that to a periodic admin job).
//   - The right action is auto-fix rather than warn (move to a
//     migration or an admin one-click).

package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// checkDataHygiene runs the full hygiene pass against the live DB.
// Each individual probe runs independently — a single SQL failure
// downgrades that probe to a warning but does not block the others.
func checkDataHygiene(db *sql.DB, result *HealthCheckResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	checkSubCategoryDepth(ctx, db, result)
	checkSubCategoryCampaign(ctx, db, result)
}

// checkSubCategoryDepth enforces the "sub-cats are exactly one level
// deep" rule. A sub-cat (parent_type_id IS NOT NULL) whose parent is
// either missing or itself a sub-cat is a model violation — the
// rendering code (sidebar filter, drill panel, variant picker) all
// assume a single level of nesting, and deeper nesting silently
// produces orphan items in the UI.
//
// The rule is documented at .ai/conventions.md and enforced at write
// time in entities/service.go:909-919 (CreateEntityType) and
// entities/service.go:1041-1042 (UpdateEntityType). Pre-existing rows
// from before the rule was enforced, or rows created by raw SQL, slip
// past those checks. This probe surfaces them.
func checkSubCategoryDepth(ctx context.Context, db *sql.DB, result *HealthCheckResult) {
	const name = "data_hygiene.subcat_depth"
	const query = `
		SELECT COUNT(*) FROM entity_types c
		LEFT JOIN entity_types p ON p.id = c.parent_type_id
		WHERE c.parent_type_id IS NOT NULL
		  AND (p.id IS NULL OR p.parent_type_id IS NOT NULL)
	`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		addWarn(result, name, "could not check sub-cat depth: "+err.Error())
		return
	}
	if count == 0 {
		addPass(result, name, "all sub-categories are exactly one level deep")
		return
	}
	addWarn(result, name, fmt.Sprintf(
		"%d sub-category entity_type(s) have an invalid parent (missing or itself a sub-cat). "+
			"Sub-cats must be exactly one level deep under a top-level category. "+
			"Inspect with: SELECT id, name, campaign_id, parent_type_id FROM entity_types c "+
			"WHERE c.parent_type_id IS NOT NULL AND c.parent_type_id NOT IN "+
			"(SELECT id FROM entity_types WHERE parent_type_id IS NULL); "+
			"Fix by either re-parenting (UPDATE entity_types SET parent_type_id = <valid_parent_id>) "+
			"or promoting to top-level (UPDATE entity_types SET parent_type_id = NULL).",
		count))
}

// checkSubCategoryCampaign enforces "a sub-cat must share a campaign
// with its parent". The campaigns FK on entity_types prevents
// dangling references but doesn't constrain parent_type_id to point
// at a same-campaign row, so a careless raw-SQL update or a future
// bug could create a cross-campaign reference. The render code would
// happily display the sub-cat under the wrong campaign's drill panel.
//
// This probe walks the entity_types table once with a self-join — fast
// even on large instances.
func checkSubCategoryCampaign(ctx context.Context, db *sql.DB, result *HealthCheckResult) {
	const name = "data_hygiene.subcat_campaign"
	const query = `
		SELECT COUNT(*) FROM entity_types c
		JOIN entity_types p ON p.id = c.parent_type_id
		WHERE c.parent_type_id IS NOT NULL
		  AND c.campaign_id != p.campaign_id
	`
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		addWarn(result, name, "could not check sub-cat campaign scoping: "+err.Error())
		return
	}
	if count == 0 {
		addPass(result, name, "all sub-categories share a campaign with their parent")
		return
	}
	addWarn(result, name, fmt.Sprintf(
		"%d sub-category entity_type(s) reference a parent in a different campaign — "+
			"this should never happen. Inspect with: SELECT c.id AS child_id, c.campaign_id AS child_campaign, "+
			"p.id AS parent_id, p.campaign_id AS parent_campaign FROM entity_types c "+
			"JOIN entity_types p ON p.id = c.parent_type_id "+
			"WHERE c.campaign_id != p.campaign_id;",
		count))
}
