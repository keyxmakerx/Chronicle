package app

// admin_db_adapters.go wires the admin Database page's Health and Backups tabs
// to their data sources from the app layer, so the admin package depends on
// neither the boot health config nor the backup/restore plugins (same pattern as
// the DatabaseExplorer / addonListerAdapter wiring).

import (
	"context"
	"database/sql"

	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
	"github.com/keyxmakerx/chronicle/internal/plugins/restore"
)

// adminHealthChecker runs the SAME health checks boot runs, on demand, for the
// Database > Health tab (via the shared StartupHealthCheckConfig).
type adminHealthChecker struct {
	db  *sql.DB
	cfg *config.Config
}

func (h *adminHealthChecker) RunChecks() *database.HealthCheckResult {
	return database.RunHealthChecks(h.db, StartupHealthCheckConfig(h.cfg))
}

// adminBackupLister adapts the backup + restore plugin services to the admin
// BackupLister interface.
type adminBackupLister struct {
	backups   backup.Service
	restores  restore.Service
	backupDir string
}

func (a *adminBackupLister) BackupInfo(_ context.Context) (admin.BackupInfo, error) {
	info := admin.BackupInfo{Enabled: a.backupDir != ""}
	if !info.Enabled {
		return info, nil
	}

	arts, err := a.backups.ListBackups()
	if err != nil {
		return info, err
	}
	for _, ar := range arts {
		pm := ar.Kind == "pre-migrate"
		ba := admin.BackupArtifact{
			Name:       ar.Name,
			SizeBytes:  ar.SizeBytes,
			ModTime:    ar.ModTime,
			Kind:       ar.Kind,
			PreMigrate: pm,
		}
		info.Artifacts = append(info.Artifacts, ba)
		// ListBackups is newest-first, so the first pre-migrate artifact is the
		// most recent automatic backup.
		if pm && info.LastPreMigrate == nil {
			latest := ba
			info.LastPreMigrate = &latest
		}
	}

	mans, err := a.restores.ListManifests()
	if err != nil {
		return info, err
	}
	for _, m := range mans {
		info.Manifests = append(info.Manifests, admin.RestoreManifest{
			Name:             m.Name,
			ChronicleVersion: m.ChronicleVersion,
			MigrationVersion: m.MigrationVersion,
			ModTime:          m.ModTime,
			ParseError:       m.ParseError,
		})
	}
	return info, nil
}
