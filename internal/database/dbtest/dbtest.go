// Package dbtest gives database-backed tests one shared way to get a
// throwaway, fully migrated MariaDB schema.
//
// Every real-database test needs the same steps: find a server from
// CHRONICLE_TEST_DB_DSN, create a uniquely named schema on it, migrate it, and
// drop it afterwards. The schema is always a fresh one rather than the DSN's
// own database because `make test-int-local` hands over a server-only DSN
// that selects no database at all. Keeping the steps in one place stops the
// copies drifting apart, which is how a missing-database failure once went
// unnoticed in one package.
//
// The package is test support only: nothing imports it from non-test code,
// and it imports no plugin, so a plugin test can use it without creating a
// dependency edge on another plugin.
package dbtest

import (
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// PluginMigrations is one plugin's migration set to apply after the core
// schema. Callers pass them in dependency order: a plugin whose tables carry
// foreign keys into another plugin's tables lists that other plugin first.
type PluginMigrations struct {
	// Slug names the plugin in the migration ledger and in failure messages.
	Slug string
	// FS is rooted at the directory that directly holds the numbered SQL files.
	// A plugin's embedded migrations need fs.Sub(MigrationsFS,
	// database.PluginMigrationsSubdir) first; PluginDir already returns the
	// right shape for a sibling plugin.
	FS fs.FS
}

// Options tunes how Open builds the schema. The zero value is the common case:
// fall back to the DB_* variables when no DSN is set, apply core migrations
// only, and skip rather than fail when a migration cannot apply.
type Options struct {
	// Prefix starts the scratch schema name, so a leaked schema can be traced
	// to the package that made it (the name is Prefix + six random digits).
	Prefix string

	// RequireDSN skips the test when CHRONICLE_TEST_DB_DSN is unset instead of
	// falling back to the DB_* variables. For tests that should only run when
	// someone deliberately started a test server.
	RequireDSN bool

	// Plugins are applied, in order, after the core migrations.
	Plugins []PluginMigrations

	// StrictMigrations makes a migration that fails to apply fatal instead of a
	// skip. Use it for a package that owns the migrations being applied: a skip
	// there would hide exactly the breakage the test exists to catch.
	StrictMigrations bool
}

// Open returns a handle to a new scratch schema with the core migrations (and
// any opts.Plugins) applied. It skips the test when no server is reachable and
// drops the schema when the test ends.
func Open(t *testing.T, opts Options) *sql.DB {
	t.Helper()

	cfg := serverConfig(t, opts.RequireDSN)

	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s: %v — run `make docker-up` or `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("%s%06d", opts.Prefix, rand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// A migration problem is an environment skip by default, but fatal where
	// the caller owns the migrations being applied.
	fail := t.Skipf
	if opts.StrictMigrations {
		fail = t.Fatalf
	}

	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(RepoRoot(t), "db", "migrations")); err != nil {
		fail("core migrations did not apply: %v", err)
	}
	if len(opts.Plugins) > 0 {
		schemas := make([]database.PluginSchema, len(opts.Plugins))
		for i, p := range opts.Plugins {
			schemas[i] = database.PluginSchema{Slug: p.Slug, MigrationsFS: p.FS}
		}
		for _, res := range database.RunPluginMigrations(db, schemas) {
			if !res.Healthy {
				fail("%s plugin migrations did not apply: %v", res.Slug, res.Error)
			}
		}
	}
	return db
}

// serverConfig resolves the server to connect to: the DSN when set, otherwise
// (unless requireDSN) the DB_* variables with the dev defaults. None of them
// is expected to name a database. ParseTime is forced on so DATETIME columns
// scan into time.Time.
func serverConfig(t *testing.T, requireDSN bool) *mysql.Config {
	t.Helper()

	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		if requireDSN {
			t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the row-level tests")
		}
		cfg := mysql.NewConfig()
		cfg.User = getenvDefault("DB_USER", "chronicle")
		cfg.Passwd = getenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = getenvDefault("DB_HOST", "127.0.0.1:3306")
		raw = cfg.FormatDSN()
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
	}
	cfg.ParseTime = true
	return cfg
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// RepoRoot returns the repository root, found by walking up from the test's
// working directory to go.mod, so callers need not count ".." segments.
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above the working directory")
		}
		dir = parent
	}
}

// PluginDir returns another plugin's migrations read straight off disk. Tests
// use it instead of importing that plugin's package, because plugins must not
// depend on each other even in test fixtures (see the wire import guard).
func PluginDir(t *testing.T, slug string) PluginMigrations {
	t.Helper()
	return PluginMigrations{
		Slug: slug,
		FS:   os.DirFS(filepath.Join(RepoRoot(t), "internal", "plugins", slug, "migrations")),
	}
}
