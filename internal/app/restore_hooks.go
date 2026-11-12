package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// restoreHooks gives the restore plugin the application-side steps around a
// restore: closing live sockets, resetting the database pool, bringing the
// restored schema up to this build, and discarding what Redis learned from the
// old database. The restore plugin defines the interface; this lives here
// because only the app root can see auth, the hub and the migration runner.
type restoreHooks struct {
	db      *sql.DB
	maxIdle int
	rdb     *redis.Client
	auth    auth.AuthService
	ws      *wsRevokerHolder
	// migrate runs the same schema steps as a normal start; set by main.
	migrate func(ctx context.Context) error
}

// derivedCachePatterns are Redis keys built from database ids and permissions
// (page-name lists, backlinks, media access answers). After a restore the same
// id can mean something else, so they are dropped rather than trusted.
var derivedCachePatterns = []string{"entity-names:*", "backlinks:*", "media:access:*", "last_seen:*"}

// Quiesce closes every websocket; the gate refuses new ones.
func (h *restoreHooks) Quiesce(context.Context) {
	h.ws.RevokeAll()
}

// ResetConnections closes pooled connections. DROP DATABASE unsets a pooled
// connection's default database and CREATE DATABASE does not restore it, so a
// kept connection would answer "No database selected" until it died.
func (h *restoreHooks) ResetConnections() {
	h.db.SetMaxIdleConns(0)
	h.db.SetMaxIdleConns(h.maxIdle)
}

// Migrate brings a possibly older backup up to this build.
func (h *restoreHooks) Migrate(ctx context.Context) error {
	if h.migrate == nil {
		return nil
	}
	return h.migrate(ctx)
}

// FlushCaches drops derived caches and forces every session to re-check its
// user against the restored database on its next request. Sessions of people
// missing from the backup then end; everyone else stays signed in.
func (h *restoreHooks) FlushCaches(ctx context.Context) (string, error) {
	cleared := 0
	for _, pattern := range derivedCachePatterns {
		iter := h.rdb.Scan(ctx, 0, pattern, 200).Iterator()
		for iter.Next(ctx) {
			if err := h.rdb.Del(ctx, iter.Val()).Err(); err != nil {
				slog.Warn("restore: could not clear a cached answer", slog.Any("error", err))
				continue
			}
			cleared++
		}
		if err := iter.Err(); err != nil {
			return "", fmt.Errorf("clearing cached answers: %w", err)
		}
	}
	marked, err := auth.ForceSessionRevalidation(ctx, h.auth)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Every sign-in is being checked against the restored accounts; anyone not in the backup is signed out on their next click. %d sign-ins checked, %d cached answers cleared.", marked, cleared), nil
}
