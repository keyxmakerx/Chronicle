package foundry_vtt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PreMigrationCheck verifies that the C-FMC-5c migration can run
// safely BEFORE the migration runner applies the SQL. The check is:
//
//	If `foundry_module_versions` table exists AND has rows,
//	REFUSE to migrate.
//
// Reasoning: 001_consolidate_foundry_modules.up.sql drops
// foundry_module_versions unconditionally. The operator confirmed the
// table is empty during planning, but a manual upload (via the
// foundry_modules admin UI that PR #305 left intact) between then and
// deploy would silently destroy data on migrate. Failing loudly here
// gives the operator a chance to inspect, back up, or manually clear.
//
// Called from cmd/server/main.go right before
// database.RunPluginMigrations. If it returns an error, server
// startup aborts with the categorized message — the operator sees
// exactly what to do.
//
// This check is idempotent and cheap (one SQL query against
// information_schema + at most one SELECT COUNT). Re-running it after
// the migration successfully completed returns nil — the table is gone.
func PreMigrationCheck(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("foundry_vtt.PreMigrationCheck: nil db handle")
	}
	return preMigrationCheck(ctx, sqlDBChecker{db})
}

// preMigrationChecker is the narrow contract the internal check uses.
// The two methods are exactly the two queries the check needs;
// keeping them as separate methods (vs. a generic Query) lets a test
// stub return canned values without parsing SQL.
type preMigrationChecker interface {
	// tableExists returns whether `foundry_module_versions` is in
	// the current database's information_schema.
	tableExists(ctx context.Context) (bool, error)
	// rowCount returns the row count of foundry_module_versions.
	// Only called when tableExists returned true.
	rowCount(ctx context.Context) (int, error)
}

// sqlDBChecker is the production implementation backed by a *sql.DB.
type sqlDBChecker struct{ db *sql.DB }

func (c sqlDBChecker) tableExists(ctx context.Context) (bool, error) {
	var n int
	err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = DATABASE()
		   AND table_name = 'foundry_module_versions'
	`).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (c sqlDBChecker) rowCount(ctx context.Context) (int, error) {
	var n int
	err := c.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM foundry_module_versions`).Scan(&n)
	return n, err
}

// preMigrationCheck is the testable core; takes the small
// preMigrationChecker interface so tests can stub canned values
// without a real DB or sqlmock dependency.
func preMigrationCheck(ctx context.Context, c preMigrationChecker) error {
	exists, err := c.tableExists(ctx)
	if err != nil {
		return fmt.Errorf(
			"foundry_vtt.PreMigrationCheck: query information_schema.tables: %w. "+
				"The pre-check needs to read schema metadata to know whether to gate the C-FMC-5c migration. "+
				"DB connectivity issue or missing privileges on information_schema. "+
				"Verify the chronicle DB user has SELECT on information_schema.tables and retry",
			err)
	}
	if !exists {
		// Migration already applied (or table never existed). Nothing
		// to check; the migration's RENAME TABLE will fail loudly on
		// its own if the token table is also already gone.
		return nil
	}

	count, err := c.rowCount(ctx)
	if err != nil {
		return fmt.Errorf(
			"foundry_vtt.PreMigrationCheck: query foundry_module_versions row count: %w. "+
				"The table exists but the row-count query failed. "+
				"DB connectivity issue or a partial schema state (table exists but unreadable). "+
				"Investigate via psql/mysql client; if the table is corrupt, restore from backup before retrying",
			err)
	}

	if count > 0 {
		// This is the operator-actionable failure path. The migration
		// would otherwise silently drop these rows. Refuse + explain.
		return fmt.Errorf(
			"C-FMC-5c migration aborted: foundry_module_versions has %d row(s). "+
				"The migration drops this table; running it would destroy these rows. "+
				"The empty-state assumption was wrong — a manual upload through foundry_modules' "+
				"admin UI happened after C-FMC-5b deployed. "+
				"To proceed: (1) inspect the rows via `SELECT * FROM foundry_module_versions;` to confirm they are not needed, "+
				"(2) `DROP TABLE foundry_module_versions;` manually (the catalog is replaced by packages plugin's package_versions), "+
				"(3) restart the server — the migration will then succeed",
			count)
	}

	return nil
}
