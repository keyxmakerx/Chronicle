package syncapi

import (
	"context"
	"fmt"
	"time"
)

// DefaultKeyRateLimit is the requests per minute a new API key gets unless its
// creator picks another figure. A Foundry world's first sync and its
// dashboard open with a few dozen calls at once, so the figure has to
// carry that burst with room to spare.
const DefaultKeyRateLimit = 300

// legacyDefaultKeyRateLimit is the figure every key got by default before
// DefaultKeyRateLimit. It was low enough that a normal Foundry connect was
// refused with 429s.
const legacyDefaultKeyRateLimit = 60

// legacyDefaultCutoff bounds the reconciler to keys made while the old
// default was the only default, so a key someone sets to 60 later is theirs.
var legacyDefaultCutoff = time.Date(2026, time.November, 15, 0, 0, 0, 0, time.UTC)

// LegacyRateLimitRaiser raises keys still on one rate limit to another.
// Satisfied by the syncapi repository.
type LegacyRateLimitRaiser interface {
	RaiseKeyRateLimits(ctx context.Context, from, to int, createdBefore time.Time) (int64, error)
}

// ReconcileLegacyRateLimits moves keys still on the old default to the new
// one. Keys created with any other figure were set on purpose and are left
// alone. Idempotent reconciler, not a migration: safe to run every boot.
// Returns the number of keys changed; the caller logs an error and boots on.
func ReconcileLegacyRateLimits(ctx context.Context, keys LegacyRateLimitRaiser) (int64, error) {
	if keys == nil {
		return 0, fmt.Errorf("syncapi.ReconcileLegacyRateLimits: nil key store")
	}
	n, err := keys.RaiseKeyRateLimits(ctx, legacyDefaultKeyRateLimit, DefaultKeyRateLimit, legacyDefaultCutoff)
	if err != nil {
		return 0, fmt.Errorf("syncapi.ReconcileLegacyRateLimits: %w", err)
	}
	return n, nil
}
