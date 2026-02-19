// Package database provides connection setup for MariaDB and Redis.
// This file handles auto-running SQL migrations on startup.
package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"

	// File source driver for reading migration files from disk.
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending migrations from the given directory.
// Uses golang-migrate to track which migrations have already been applied.
// Safe to call on every startup — already-applied migrations are skipped.
func RunMigrations(db *sql.DB, migrationsPath string) error {
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

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	slog.Info("migrations applied",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty),
	)

	return nil
}
