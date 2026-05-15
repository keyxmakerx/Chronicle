package foundry_vtt

import (
	"context"
	"fmt"
	"log/slog"
)

// AutoPinMigrationSettingKey is the settings.SettingsRepository key
// that tracks whether the one-time C-FMC-6 auto-pin migration has
// completed. Value is the version string that was used as the pin
// target — present means the migration ran; absent means it hasn't.
//
// Stored in the existing settings table so the check survives
// process restarts and doesn't need a new schema row.
const AutoPinMigrationSettingKey = "foundry_vtt.autopin_migration_completed_for_version"

// SettingsKVStore is the narrow contract AutoPinMigrate needs:
// just Get/Set on string keys. Implemented by settings.SettingsRepository
// in production. Kept here as a local interface so the migration
// can be tested without importing settings.
type SettingsKVStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// AutoPinMigrate runs the one-time C-FMC-6 auto-pin migration:
// every campaign with an empty foundry_module_pin is pinned to
// the currently-installed foundry-module version, so the next
// install triggers the AutoPinHook flow (which preserves state +
// notifies admin) instead of silently bumping them.
//
// Idempotency: stores the version it pinned to under
// AutoPinMigrationSettingKey. Subsequent calls read that key and
// return immediately. If the operator manually clears the key,
// the migration re-runs (intentional escape hatch).
//
// Called from cmd/server/main.go AFTER plugin migrations have run
// (so foundry_vtt's schema exists) but BEFORE the HTTP server
// starts accepting traffic (so the migration completes before any
// campaign loads its settings page).
//
// No-op cases (return nil without setting the flag, so future
// boots retry the migration once data is available):
//   - No foundry-module package registered yet
//   - Foundry-module package has no InstalledVersion
//
// Failure cases (return error, abort startup):
//   - Settings store read/write fails (DB connectivity)
//   - CampaignsWithEmptyPin query fails
//
// Per-campaign pin failures are logged + skipped — one bad campaign
// doesn't abort the migration for the rest. The flag is set on
// completion regardless of per-campaign failures (the migration
// is best-effort; missed campaigns can be re-pinned manually via
// the admin UI).
func AutoPinMigrate(ctx context.Context, svc Service, settings SettingsKVStore) error {
	// Idempotency check: skip if flag is set.
	// settings.Get returns ("", *apperror.AppError{Type:"not_found"})
	// when the key doesn't exist. We treat that as "first-time
	// migration, proceed". Any non-empty value means a previous
	// migration completed → skip.
	//
	// On other DB errors (connection lost, etc.) settings.Get
	// returns a different apperror Type; we'd rather proceed than
	// abort startup over a stale-check failure (worst case: the
	// migration runs again, which is effectively a no-op for
	// already-pinned campaigns since CampaignsWithEmptyPin returns
	// only un-pinned rows).
	existing, _ := settings.Get(ctx, AutoPinMigrationSettingKey)
	if existing != "" {
		slog.Info("foundry_vtt autopin migration: already completed",
			slog.String("pinned_to", existing))
		return nil
	}

	// Find the foundry-module package + its currently-installed
	// version. No-op if absent — operator hasn't set up the
	// foundry-module package yet, so there's no version to pin TO.
	pkg, err := svc.FindFoundryPackage(ctx)
	if err != nil {
		return fmt.Errorf("foundry_vtt.AutoPinMigrate: find foundry package: %w", err)
	}
	if pkg == nil {
		slog.Info("foundry_vtt autopin migration: no foundry-module package registered, skipping (will retry on next boot)")
		return nil
	}
	if pkg.InstalledVersion == "" {
		slog.Info("foundry_vtt autopin migration: foundry-module package has no installed version, skipping")
		return nil
	}

	// Pin every empty-pin campaign to the currently-installed
	// version. Distinct from AutoPinOnInstall because:
	//   - AutoPinOnInstall short-circuits on previous==new (defensive
	//     against InstallVersion re-firing on a no-op install).
	//   - The migration's whole point is to make an effective state
	//     explicit; the same-version pin IS the operation.
	//   - Migration events use a different type
	//     (EventModuleAutoPinMigration) so the audit log distinguishes
	//     the one-time bootstrap from the per-install variant.
	count, err := svc.MigrateAutoPinToVersion(ctx, pkg.InstalledVersion)
	if err != nil {
		return fmt.Errorf("foundry_vtt.AutoPinMigrate: iterate campaigns: %w", err)
	}

	// Set the completion flag so subsequent boots skip the migration.
	if err := settings.Set(ctx, AutoPinMigrationSettingKey, pkg.InstalledVersion); err != nil {
		return fmt.Errorf("foundry_vtt.AutoPinMigrate: write completion flag: %w", err)
	}

	slog.Info("foundry_vtt autopin migration: completed",
		slog.Int("campaigns_pinned", count),
		slog.String("version", pkg.InstalledVersion))
	return nil
}

