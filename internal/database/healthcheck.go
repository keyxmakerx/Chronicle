// Package database — healthcheck.go
// Comprehensive startup health checks that run after migrations to catch
// configuration, schema, and security issues before the server accepts traffic.
// Each check logs its result and returns an error only for fatal problems.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// HealthCheckConfig controls which startup checks run and their parameters.
type HealthCheckConfig struct {
	// ExpectedMigrationVersion is the migration version the code requires.
	ExpectedMigrationVersion uint

	// CriticalColumns maps table names to columns that MUST exist for the app
	// to function. Catches schema drift from unapplied or failed migrations.
	CriticalColumns map[string][]string

	// BackupDir is where pre-migration backups are stored. Empty disables
	// the entire pre-migration capture subsystem (legacy behavior).
	BackupDir string

	// BackupMaxAge is how long to keep old backups. Defaults to 7 days.
	BackupMaxAge time.Duration

	// BackupRequired switches PreMigrationBackup from fail-open
	// (legacy: log a warning, return nil so migrations proceed) to
	// fail-closed (return an error). main() surfaces the error as a
	// startup failure when this flag is true. Use in production where
	// no rollback story is acceptable.
	BackupRequired bool

	// MediaPath is the on-disk root of the media tree. When set, the
	// pre-migration capture also tar+gzips this directory alongside
	// the DB dump. Empty skips media capture (DB-only mode).
	MediaPath string

	// RedisURL is the connection string passed to `redis-cli --rdb`
	// during pre-migration capture. Empty skips redis capture
	// (sessions only; recoverable). Format matches REDIS_URL env var.
	RedisURL string

	// DSN is the database connection string (needed for mysqldump).
	DSN string

	// DBName is the database name (needed for mysqldump).
	DBName string

	// DBHost is the database host (needed for mysqldump).
	DBHost string

	// DBUser is the database user (needed for mysqldump).
	DBUser string

	// DBPassword is the database password (needed for mysqldump).
	DBPassword string

	// DBTLSMode is the configured TLS mode for the database connection.
	// Empty or "disabled" means no TLS. Used by security checks to warn
	// about unencrypted database traffic in production.
	DBTLSMode string

	// Env is the runtime environment ("development" or "production").
	Env string

	// BaseURL is the public-facing URL of the server.
	BaseURL string

	// SmokeTests are startup probes that run actual SELECT + Scan queries
	// against critical tables. Catches column count mismatches and type errors
	// that information_schema checks cannot detect.
	SmokeTests []SmokeTest
}

// SmokeTest is a named startup probe that verifies a critical query+scan
// pattern works end-to-end. Each probe runs SELECT ... LIMIT 1 on a table
// and scans into the real Go struct. If the scan fails (wrong column count,
// type mismatch, etc.), the server refuses to start.
type SmokeTest struct {
	Name string                  // Human-readable label (e.g. "campaigns.list_scan").
	Fn   func(db *sql.DB) error // Runs the query and scan. sql.ErrNoRows = pass.
}

// HealthCheckResult aggregates all check outcomes.
type HealthCheckResult struct {
	Checks  []CheckOutcome
	Errors  int
	Warns   int
	Passed  int
}

// CheckOutcome represents a single check's result.
type CheckOutcome struct {
	Name    string
	Status  string // "pass", "warn", "fail"
	Message string
}

// RunStartupHealthChecks executes all startup validation checks.
// Returns an error if any fatal check fails (the server should not start).
func RunStartupHealthChecks(db *sql.DB, cfg HealthCheckConfig) error {
	slog.Info("running startup health checks...")
	result := &HealthCheckResult{}

	// 1. Migration version validation.
	checkMigrationVersion(db, cfg, result)

	// 2. Critical schema validation — verify expected columns exist.
	checkCriticalColumns(db, cfg, result)

	// 3. Database connectivity and performance.
	checkDBHealth(db, result)

	// 4. Security audit.
	checkSecurity(db, cfg, result)

	// 5. Smoke-test queries — verify critical SELECT+Scan patterns work.
	checkSmokeTests(db, cfg, result)

	// 6. Data-shape hygiene — invariants the FK schema can't enforce.
	checkDataHygiene(db, result)

	// Log summary.
	for _, c := range result.Checks {
		switch c.Status {
		case "pass":
			slog.Info("health check passed", slog.String("check", c.Name), slog.String("detail", c.Message))
		case "warn":
			slog.Warn("health check warning", slog.String("check", c.Name), slog.String("detail", c.Message))
		case "fail":
			slog.Error("health check FAILED", slog.String("check", c.Name), slog.String("detail", c.Message))
		}
	}

	slog.Info("health check summary",
		slog.Int("passed", result.Passed),
		slog.Int("warnings", result.Warns),
		slog.Int("failures", result.Errors),
	)

	if result.Errors > 0 {
		return fmt.Errorf("%d startup health check(s) failed — server cannot start safely", result.Errors)
	}
	return nil
}

// --- Check implementations ---

// checkMigrationVersion verifies the DB is at the expected migration version.
func checkMigrationVersion(db *sql.DB, cfg HealthCheckConfig, result *HealthCheckResult) {
	var version int
	var dirty bool
	err := db.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if err != nil {
		addFail(result, "migration_version",
			fmt.Sprintf("cannot read schema_migrations: %v — has 'make migrate-up' been run?", err))
		return
	}

	if dirty {
		addFail(result, "migration_version",
			fmt.Sprintf("migration %d is in DIRTY state — a migration failed mid-apply. Run 'make migrate-down' then 'make migrate-up'", version))
		return
	}

	if uint(version) < cfg.ExpectedMigrationVersion {
		addFail(result, "migration_version",
			fmt.Sprintf("database at migration %d but code requires %d — run 'make migrate-up'", version, cfg.ExpectedMigrationVersion))
		return
	}

	addPass(result, "migration_version",
		fmt.Sprintf("at version %d (expected >=%d)", version, cfg.ExpectedMigrationVersion))
}

// checkCriticalColumns verifies that expected tables and columns exist.
// This catches the exact bug where code references columns from unapplied migrations.
func checkCriticalColumns(db *sql.DB, cfg HealthCheckConfig, result *HealthCheckResult) {
	if len(cfg.CriticalColumns) == 0 {
		return
	}

	var missing []string
	for table, columns := range cfg.CriticalColumns {
		for _, col := range columns {
			var exists int
			err := db.QueryRow(`
				SELECT COUNT(*) FROM information_schema.COLUMNS
				WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
			`, table, col).Scan(&exists)
			if err != nil || exists == 0 {
				missing = append(missing, fmt.Sprintf("%s.%s", table, col))
			}
		}
	}

	if len(missing) > 0 {
		addFail(result, "schema_columns",
			fmt.Sprintf("%d critical column(s) missing: %s — run 'make migrate-up'",
				len(missing), strings.Join(missing, ", ")))
		return
	}

	total := 0
	for _, cols := range cfg.CriticalColumns {
		total += len(cols)
	}
	addPass(result, "schema_columns",
		fmt.Sprintf("all %d critical columns verified across %d tables", total, len(cfg.CriticalColumns)))
}

// checkDBHealth verifies basic database connectivity and responsiveness.
func checkDBHealth(db *sql.DB, result *HealthCheckResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if err := db.PingContext(ctx); err != nil {
		addFail(result, "db_connectivity", fmt.Sprintf("ping failed: %v", err))
		return
	}
	pingMs := time.Since(start).Milliseconds()

	if pingMs > 500 {
		addWarn(result, "db_connectivity",
			fmt.Sprintf("ping succeeded but slow (%dms) — check network/load", pingMs))
		return
	}

	// Check connection pool stats.
	stats := db.Stats()
	if stats.OpenConnections > stats.MaxOpenConnections*80/100 {
		addWarn(result, "db_pool",
			fmt.Sprintf("connection pool near capacity (%d/%d open)", stats.OpenConnections, stats.MaxOpenConnections))
	}

	addPass(result, "db_connectivity", fmt.Sprintf("ping %dms, pool %d/%d connections",
		pingMs, stats.OpenConnections, stats.MaxOpenConnections))
}

// checkSecurity runs security-related startup audits.
func checkSecurity(db *sql.DB, cfg HealthCheckConfig, result *HealthCheckResult) {
	isProd := cfg.Env == "production"

	// Check for default database credentials in production.
	if isProd && cfg.DBPassword != "" {
		weak := []string{"chronicle", "password", "secret", "changeme", "root", "admin", ""}
		for _, w := range weak {
			if cfg.DBPassword == w {
				addWarn(result, "security_db_password",
					"database using default/weak password in production — change DB_PASSWORD")
				break
			}
		}
	}
	if isProd && cfg.DBPassword == "" {
		addWarn(result, "security_db_password", "no database password set in production")
	}

	// Check for HTTP in production BaseURL.
	if isProd && cfg.BaseURL != "" && strings.HasPrefix(cfg.BaseURL, "http://") {
		addWarn(result, "security_base_url",
			"BASE_URL uses http:// in production — CSRF cookies require HTTPS; set BASE_URL to https://")
	}

	// Check for unencrypted database connections in production.
	tlsMode := strings.ToLower(cfg.DBTLSMode)
	if isProd && (tlsMode == "" || tlsMode == "disabled") {
		addWarn(result, "security_db_tls",
			"database connection is unencrypted in production — set DB_TLS_MODE=required for TLS encryption")
	}

	// Check for overprivileged DB user (SUPER, FILE, PROCESS grants).
	var grantRows *sql.Rows
	grantRows, err := db.Query("SHOW GRANTS FOR CURRENT_USER()")
	if err == nil {
		defer grantRows.Close()
		dangerousPrivs := []string{"ALL PRIVILEGES", "SUPER", "FILE", "PROCESS", "GRANT OPTION"}
		for grantRows.Next() {
			var grant string
			if grantRows.Scan(&grant) == nil {
				for _, priv := range dangerousPrivs {
					if strings.Contains(strings.ToUpper(grant), priv) {
						if isProd {
							addWarn(result, "security_db_grants",
								fmt.Sprintf("DB user has %s privilege — use a least-privilege account in production", priv))
						}
						break
					}
				}
			}
		}
	}

	// Check that schema_migrations table isn't world-writable.
	// (If an attacker can write to it, they can mark migrations as applied
	// and prevent security patches from being deployed.)
	var tablePriv string
	err = db.QueryRow(`
		SELECT PRIVILEGE_TYPE FROM information_schema.TABLE_PRIVILEGES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'schema_migrations'
		AND GRANTEE != CONCAT("'", CURRENT_USER(), "'")
		LIMIT 1
	`).Scan(&tablePriv)
	if err == nil && tablePriv != "" {
		addWarn(result, "security_migration_table",
			"schema_migrations has grants to other users — only the app user should have access")
	}

	if !isProd {
		addPass(result, "security_audit", "development mode — security warnings suppressed")
	} else {
		addPass(result, "security_audit", "production security checks completed")
	}
}

// PreMigrationBackup and the helpers it relies on now live in
// pre_migration_backup.go. Kept here as a doc anchor — the function's
// signature changed in the symmetry refactor (now takes *sql.DB and
// returns error so the caller can fail-closed when BACKUP_REQUIRED=1).

// checkSmokeTests runs each registered smoke test to verify that critical
// query+scan patterns work end-to-end. An empty table (sql.ErrNoRows) is
// treated as a pass — the scan pattern is still validated by the query planner.
func checkSmokeTests(db *sql.DB, cfg HealthCheckConfig, result *HealthCheckResult) {
	if len(cfg.SmokeTests) == 0 {
		return
	}

	failed := 0
	for _, st := range cfg.SmokeTests {
		if err := st.Fn(db); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue // Empty table — scan pattern is valid, just no data.
			}
			addFail(result, "smoke_test",
				fmt.Sprintf("%s: %v — query/scan pattern mismatch detected", st.Name, err))
			failed++
		}
	}

	if failed > 0 {
		return
	}

	addPass(result, "smoke_tests",
		fmt.Sprintf("all %d probe(s) passed", len(cfg.SmokeTests)))
}

// --- Helper functions ---

func addPass(r *HealthCheckResult, name, msg string) {
	r.Checks = append(r.Checks, CheckOutcome{Name: name, Status: "pass", Message: msg})
	r.Passed++
}

func addWarn(r *HealthCheckResult, name, msg string) {
	r.Checks = append(r.Checks, CheckOutcome{Name: name, Status: "warn", Message: msg})
	r.Warns++
}

func addFail(r *HealthCheckResult, name, msg string) {
	r.Checks = append(r.Checks, CheckOutcome{Name: name, Status: "fail", Message: msg})
	r.Errors++
}
