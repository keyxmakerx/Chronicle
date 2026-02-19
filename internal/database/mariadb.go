// Package database provides connection setup for MariaDB and Redis.
// Both connections are created once at startup and shared across the
// application via dependency injection. This package owns the connection
// lifecycle (open, configure pool, ping, close).
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	// MariaDB driver -- imported for side effect of registering the driver.
	_ "github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/config"
)

// NewMariaDB creates a new MariaDB connection pool configured with the
// settings from the provided config. It pings the database to verify
// connectivity before returning.
func NewMariaDB(cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("opening mariadb connection: %w", err)
	}

	// Configure connection pool settings to prevent connection exhaustion
	// and stale connections under load.
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Retry with exponential backoff — MariaDB may still be starting up
	// when the app container launches. This avoids crash-loop restarts
	// during Docker Compose cold-starts.
	const maxRetries = 10
	backoff := 1 * time.Second
	var pingErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr = db.PingContext(ctx)
		cancel()

		if pingErr == nil {
			return db, nil
		}

		if attempt == maxRetries {
			break
		}

		slog.Warn("mariadb not ready, retrying...",
			slog.Int("attempt", attempt),
			slog.Int("max_retries", maxRetries),
			slog.Duration("backoff", backoff),
			slog.Any("error", pingErr),
		)
		time.Sleep(backoff)
		backoff = min(backoff*2, 30*time.Second)
	}

	db.Close()
	return nil, fmt.Errorf("pinging mariadb after %d attempts: %w", maxRetries, pingErr)
}
