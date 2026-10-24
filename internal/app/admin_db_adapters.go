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
	settings  backup.SettingsStore
	backupDir string
}

func (a *adminBackupLister) BackupInfo(ctx context.Context) (admin.BackupInfo, error) {
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
			PreUpdate:        m.PreUpdate,
		})
	}

	info.LastRunFailed = lastRunFailed(a.backups.LastRun(), nil)
	if a.settings != nil {
		sched, sErr := backup.LoadSchedule(ctx, a.settings)
		last, lErr := backup.LoadLastScheduledRun(ctx, a.settings)
		if sErr == nil && lErr == nil {
			info.ScheduleKnown, info.ScheduleOn = true, sched.Enabled
			info.LastRunFailed = lastRunFailed(a.backups.LastRun(), last)
		}
	}
	return info, nil
}

// lastRunFailed reports whether the newer of the last button run (held in
// memory since boot) and the last scheduled run (stored) failed.
func lastRunFailed(manual *backup.RunResult, scheduled *backup.ScheduledRun) bool {
	switch {
	case manual == nil && scheduled == nil:
		return false
	case scheduled == nil:
		return !manual.Succeeded()
	case manual == nil || scheduled.At.After(manual.FinishedAt):
		return !scheduled.OK
	default:
		return !manual.Succeeded()
	}
}
