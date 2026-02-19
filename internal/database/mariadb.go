// Package database provides connection setup for MariaDB and Redis.
// Both connections are created once at startup and shared across the
// application via dependency injection. This package owns the connection
// lifecycle (open, configure pool, ping, close).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	// MariaDB driver -- imported for side effect of registering the driver.
	_ "github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/config"
)

// NewMariaDB creates a new MariaDB connection pool configured with the
// settings from the provided config. It pings the database to verify
// connectivity before returning.
func NewMariaDB(cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("opening mariadb connection: %w", err)
	}

	// Configure connection pool settings to prevent connection exhaustion
	// and stale connections under load.
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify the connection is alive before returning. Use a short timeout
	// so startup fails fast if the database is unreachable.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging mariadb: %w", err)
	}

	return db, nil
}
