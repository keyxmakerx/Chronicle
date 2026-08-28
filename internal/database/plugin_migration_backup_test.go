package database

import (
	"testing"
	"testing/fstest"
)

// plugin_migration_backup_test.go proves the detection half of the
// pre-plugin-migration backup gate against a real MariaDB, in the same
// harness as the recovery tests (scratch schema per test, SKIP when no
// server answers, skipped under -short). Named TestPluginMigration_* so the
// Fresh-DB Migration Replay CI job runs it — the gate exists for the CALV5
// wipe, and a detection helper that only ever ran against a mock would be
// the exact class of false confidence this branch was built to end.
func TestPluginMigration_PendingDetectionForBackupGate(t *testing.T) {
	if testing.Short() {
		t.Skip("pending-detection test requires a database; skipped under -short")
	}
	db := newMigrationScratchSchema(t)

	const slug = "sweepbackupgate"
	v1 := fstest.MapFS{
		"001_first.up.sql":   {Data: []byte("CREATE TABLE IF NOT EXISTS sweep_bg (id INT NOT NULL PRIMARY KEY);")},
		"001_first.down.sql": {Data: []byte("DROP TABLE IF EXISTS sweep_bg;")},
	}
	schema := PluginSchema{Slug: slug, MigrationsFS: v1}

	// A fresh database has no tracking rows: everything with migrations is
	// pending, and the helper must say so — this is the state in which the
	// wipe would run, so "pending" here is what triggers the backup.
	pending, err := PendingPluginMigrations(db, []PluginSchema{schema})
	if err != nil {
		t.Fatalf("pending on fresh schema: %v", err)
	}
	if len(pending) != 1 || pending[0] != slug {
		t.Fatalf("fresh schema: pending = %v, want [%s]", pending, slug)
	}

	// After the runner applies it, the same question must answer no —
	// otherwise every restart takes a backup and the pending-gate's whole
	// point (no backup-on-every-restart storm) is lost.
	if res := runOnePlugin(t, db, schema); !res.Healthy {
		t.Fatalf("applying v1: %v", res.Error)
	}
	pending, err = PendingPluginMigrations(db, []PluginSchema{schema})
	if err != nil {
		t.Fatalf("pending after apply: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("after apply: pending = %v, want none", pending)
	}

	// Shipping a NEW migration for an existing plugin — the CALV5 shape:
	// calendar was at version 18 everywhere, and the wipe is version 19 —
	// must flip the plugin back to pending.
	v2 := fstest.MapFS{
		"001_first.up.sql":   v1["001_first.up.sql"],
		"001_first.down.sql": v1["001_first.down.sql"],
		"002_wipe.up.sql":    {Data: []byte("DELETE FROM sweep_bg;")},
		"002_wipe.down.sql":  {Data: []byte("-- intentionally empty")},
	}
	pending, err = PendingPluginMigrations(db, []PluginSchema{{Slug: slug, MigrationsFS: v2}})
	if err != nil {
		t.Fatalf("pending with new migration: %v", err)
	}
	if len(pending) != 1 || pending[0] != slug {
		t.Fatalf("new migration shipped: pending = %v, want [%s]", pending, slug)
	}

	// Plugins with no migrations never trigger a backup.
	pending, err = PendingPluginMigrations(db, []PluginSchema{{Slug: "bare"}, {Slug: "empty", MigrationsFS: fstest.MapFS{}}})
	if err != nil {
		t.Fatalf("pending on migration-less plugins: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("migration-less plugins: pending = %v, want none", pending)
	}
}
