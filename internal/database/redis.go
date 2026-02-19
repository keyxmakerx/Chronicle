package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/config"
)

// NewRedis creates a new Redis client from the given config. It parses the
// URL, connects, and pings to verify connectivity before returning.
func NewRedis(cfg config.RedisConfig) (*redis.Client, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Retry with exponential backoff — Redis may still be starting up
	// when the app container launches.
	const maxRetries = 10
	backoff := 1 * time.Second
	var pingErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr = client.Ping(ctx).Err()
		cancel()

		if pingErr == nil {
			return client, nil
		}

		if attempt == maxRetries {
			break
		}

		slog.Warn("redis not ready, retrying...",
			slog.Int("attempt", attempt),
			slog.Int("max_retries", maxRetries),
			slog.Duration("backoff", backoff),
			slog.Any("error", pingErr),
		)
		time.Sleep(backoff)
		backoff = min(backoff*2, 30*time.Second)
	}

	client.Close()
	return nil, fmt.Errorf("pinging redis after %d attempts: %w", maxRetries, pingErr)
}
