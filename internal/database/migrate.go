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

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"mysql",
		driver,
	)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	err = m.Up()

	// Handle dirty database state: a previous migration failed partway through.
	// Since our migrations use IF NOT EXISTS / IF EXISTS, it's safe to force
	// the version back and retry.
	if err != nil {
		var dirtyErr migrate.ErrDirty
		if errors.As(err, &dirtyErr) {
			slog.Warn("dirty migration state detected, forcing version and retrying",
				slog.Int("dirty_version", dirtyErr.Version),
			)
			// Force to the previous clean version (dirty version - 1).
			// If dirty_version is 1, force to -1 (no version / clean slate).
			forceVersion := dirtyErr.Version - 1
			if forceErr := m.Force(forceVersion); forceErr != nil {
				return fmt.Errorf("forcing migration version %d: %w", forceVersion, forceErr)
			}
			// Retry migrations from the forced version.
			err = m.Up()
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
