// Package database provides connection setup for MariaDB and Redis.
// This file handles auto-running SQL migrations on startup.
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"

	// File source driver for reading migration files from disk.
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending migrations from the given directory.
// Opens a separate connection with multiStatements=true (required by
// golang-migrate for migration files containing multiple SQL statements)
// so the main app connection stays secure without multi-statement support.
// Handles dirty database state by forcing the version and retrying.
// Safe to call on every startup — already-applied migrations are skipped.
func RunMigrations(appDB *sql.DB, dsn string, migrationsPath string) error {
	// Open a dedicated connection for migrations with multiStatements enabled.
	// golang-migrate requires this for migration files with multiple statements.
	sep := "&"
	if !strings.Contains(dsn, "?") {
		sep = "?"
	}
	migrationDSN := dsn + sep + "multiStatements=true"
	db, err := sql.Open("mysql", migrationDSN)
	if err != nil {
		return fmt.Errorf("opening migration connection: %w", err)
	}
	defer db.Close()

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("creating migration driver: %w", err)
	}
	// The driver takes a connection of its own out of the pool and gives it
	// back only when it is closed; closing the pool alone leaves that one open
	// for as long as the process lives.
	defer func() { _ = driver.Close() }()

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"mysql",
		driver,
	)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	err = m.Up()

	// A dirty version means a previous migration failed partway. FAIL FAST —
	// never auto-force-and-retry: older, non-idempotent migrations can die on
	// "Duplicate column" (Error 1060) and re-dirty the version, a permanent
	// crash-loop. New migrations must stay idempotent (TestMigrations_IdempotentDDL
	// enforces it). Recovery is an explicit operator action; fatalBoot's backoff
	// prevents hot-looping while they act.
	if err != nil {
		var dirtyErr migrate.ErrDirty
		if errors.As(err, &dirtyErr) {
			return fmt.Errorf("database migration %d is DIRTY (a previous migration failed "+
				"partway); automatic recovery is disabled because re-running a partially-applied "+
				"migration can be unsafe. Restore the most recent pre-migration backup in "+
				"BACKUP_DIR, or repair manually per docs/deployment.md §Rollback, then redeploy: %w",
				dirtyErr.Version, err)
		}
	}

	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	slog.Info("migrations applied",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty),
	)

	return nil
}
