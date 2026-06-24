package database

// migrate_state.go holds the migration-state helpers and the boot-time
// orchestration that makes migrating robust for self-hosters: it backs up only
// when a migration is actually pending, and it tolerates a database that is
// AHEAD of this build's migration set (a downgrade/rollback, or an
// accidentally-deleted-but-applied migration) by logging and continuing instead
// of crash-looping. See ADR-044 + the 000030 incident.

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// ExpectedCoreMigrationVersion is the highest core migration the code requires
// the database to be at (the health-check floor). It MUST equal the highest
// db/migrations/NNNNNN_*.up.sql number — TestExpectedCoreMigrationVersion_MatchesMax
// enforces that, so this constant can never silently drift from reality again.
const ExpectedCoreMigrationVersion uint = 30

// HighestSourceVersion returns the highest migration version present in
// migrationsPath, parsed from the leading NNNNNN_ of each *.up.sql filename.
// Returns 0 when the directory holds no migration files.
func HighestSourceVersion(migrationsPath string) (uint, error) {
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return 0, fmt.Errorf("reading migrations dir %q: %w", migrationsPath, err)
	}
	var max uint
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		i := strings.IndexByte(name, '_')
		if i <= 0 {
			continue
		}
		n, perr := strconv.ParseUint(name[:i], 10, 64)
		if perr != nil {
			continue
		}
		if uint(n) > max {
			max = uint(n)
		}
	}
	return max, nil
}

// DBMigrationVersion reads the current migration version + dirty flag from
// schema_migrations. Returns (0, false, nil) when the table is empty. It reads
// cleanly even when the on-disk migration file for that version is missing —
// that property is exactly what lets MigrateWithBackup detect a DB-ahead state
// BEFORE golang-migrate's Up() would hard-error on it. A genuine query failure
// (e.g. the table doesn't exist yet on a brand-new DB) is returned as an error
// for the caller to interpret.
func DBMigrationVersion(db *sql.DB) (version uint, dirty bool, err error) {
	if db == nil {
		return 0, false, nil
	}
	var v sql.NullInt64
	var d sql.NullBool
	row := db.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1")
	if scanErr := row.Scan(&v, &d); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, scanErr
	}
	if !v.Valid {
		return 0, false, nil
	}
	return uint(v.Int64), d.Bool, nil
}

// MigrateWithBackup is the boot-time migration orchestration. It computes the
// DB version and the highest available migration ONCE and then:
//
//   - DB AHEAD of code (dbVer > srcMax): logs a loud, actionable warning and
//     starts ANYWAY (skips both backup and Up()). golang-migrate's Up() would
//     otherwise hard-error ("no migration found for version N"); since
//     migrations are additive, an older binary runs fine on a newer schema. The
//     startup health checks backstop the rare destructive-rollback edge. This is
//     the fix for the 000030 crash-loop and for ordinary image rollbacks.
//   - Up to date (dbVer == srcMax, not dirty): skips BOTH the backup and Up().
//     A backup's purpose is to capture state before a schema change; with
//     nothing pending there is nothing to protect. This stops the
//     backup-on-every-restart storm.
//   - Pending (or dirty): runs the pre-migration backup, then RunMigrations.
//
// It replaces the previous unconditional "PreMigrationBackup then RunMigrations"
// boot sequence.
func MigrateWithBackup(db *sql.DB, dsn, migrationsPath string, cfg HealthCheckConfig) error {
	dbVer, dbDirty, verErr := DBMigrationVersion(db)
	if verErr != nil {
		// schema_migrations likely doesn't exist yet (brand-new DB). Treat as
		// version 0 so the pending path runs and creates the schema.
		slog.Debug("could not read schema_migrations (fresh database?); treating as version 0",
			slog.Any("error", verErr))
		dbVer, dbDirty = 0, false
	}

	srcMax, srcErr := HighestSourceVersion(migrationsPath)
	if srcErr != nil {
		return fmt.Errorf("scanning migration source %q: %w", migrationsPath, srcErr)
	}

	switch {
	case dbVer > srcMax:
		slog.Warn("database is AHEAD of this build — skipping migrations and starting anyway",
			slog.Uint64("db_version", uint64(dbVer)),
			slog.Uint64("build_supports_up_to", uint64(srcMax)),
			slog.String("likely_cause", "this image is OLDER than the one that last wrote this database (a downgrade/rollback)"),
			slog.String("effect", fmt.Sprintf("features added after migration %d are unavailable until you deploy a build that includes them", srcMax)),
			slog.String("action", "to move forward deploy the newer image; to stay on this version intentionally, ignore this warning"),
		)
		return nil

	case dbVer == srcMax && !dbDirty:
		slog.Info("database schema is up to date — no pending migrations",
			slog.Uint64("version", uint64(dbVer)))
		return nil
	}

	// A migration is pending (or the DB is dirty and needs the recovery path).
	// Capture a backup BEFORE applying schema changes.
	slog.Info("pending migration(s) detected — backing up before applying",
		slog.Uint64("from_version", uint64(dbVer)),
		slog.Uint64("to_version", uint64(srcMax)),
		slog.Bool("dirty", dbDirty),
	)
	if backupErr := PreMigrationBackup(db, cfg); backupErr != nil {
		if cfg.BackupRequired {
			return fmt.Errorf("pre-migration backup failed and BACKUP_REQUIRED=1; refusing to apply migrations: %w", backupErr)
		}
		slog.Warn("pre-migration backup failed (non-fatal in default mode); migrations will still apply",
			slog.Any("error", backupErr))
	}

	return RunMigrations(db, dsn, migrationsPath)
}
