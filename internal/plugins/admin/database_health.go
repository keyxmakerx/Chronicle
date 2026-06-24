// Package admin — database_health.go defines the admin-local contracts the
// Database page's Health and Backups tabs render. Concrete implementations live
// in the app layer (over the boot health-check config and the backup/restore
// plugin services), so the admin package depends on neither — same pattern as
// DatabaseExplorer.
package admin

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// HealthResult is the structured health-check result the Health tab renders.
// Aliased so the handler + templates use a package-local name (no direct
// dependency on the database package from those files).
type HealthResult = database.HealthCheckResult

// HealthChecker runs the startup health checks on demand for the Health tab.
type HealthChecker interface {
	RunChecks() *HealthResult
}

// BackupArtifact is one file in the backup directory, surfaced on the Backups tab.
type BackupArtifact struct {
	Name       string
	SizeBytes  int64
	ModTime    time.Time
	Kind       string // "db", "media", "redis", "manifest", "pre-migrate", "other"
	PreMigrate bool   // automatic pre-migration backup (vs operator-triggered)
}

// RestoreManifest is one restorable snapshot manifest, surfaced on the Backups tab.
type RestoreManifest struct {
	Name             string
	ChronicleVersion string
	MigrationVersion string
	ModTime          time.Time
	ParseError       string
}

// BackupInfo aggregates everything the Backups tab needs.
type BackupInfo struct {
	Enabled        bool             // BackupDir configured
	Artifacts      []BackupArtifact // recent files, newest-first
	Manifests      []RestoreManifest // restorable snapshots, newest-first
	LastPreMigrate *BackupArtifact   // most recent automatic pre-migration backup
}

// BackupLister provides the Backups tab its data.
type BackupLister interface {
	BackupInfo(ctx context.Context) (BackupInfo, error)
}
