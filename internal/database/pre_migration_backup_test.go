// Tests for PreMigrationBackup and its helpers. The full happy path
// requires mysqldump and (optionally) redis-cli on PATH; those are
// gated by t.Skip when the binaries are missing so the suite still
// passes in minimal environments. The pure helpers (sha256AndSize,
// writeManifest, splitHostPort, rotatePreMigrationBackups) are
// covered without any external tools.
package database

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSha256AndSize pins the streaming hash + size helper used to
// verify every artifact before the manifest is written.
func TestSha256AndSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	payload := []byte("chronicle-pre-migration-payload")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	gotSum, gotSize, err := sha256AndSize(path)
	if err != nil {
		t.Fatalf("sha256AndSize: %v", err)
	}
	wantSum := sha256.Sum256(payload)
	if gotSum != hex.EncodeToString(wantSum[:]) {
		t.Errorf("sha = %s, want %s", gotSum, hex.EncodeToString(wantSum[:]))
	}
	if gotSize != int64(len(payload)) {
		t.Errorf("size = %d, want %d", gotSize, len(payload))
	}
}

// TestSplitHostPort confirms host:port parsing handles each form the
// real DB_HOST env var takes in practice.
func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in        string
		wantHost  string
		wantPort  string
		fallback  string
	}{
		{"localhost:3306", "localhost", "3306", "3306"},
		{"db.internal:3307", "db.internal", "3307", "3306"},
		{"justhost", "justhost", "3306", "3306"},
	}
	for _, c := range cases {
		h, p := splitHostPort(c.in, c.fallback)
		if h != c.wantHost || p != c.wantPort {
			t.Errorf("splitHostPort(%q) = (%q, %q), want (%q, %q)",
				c.in, h, p, c.wantHost, c.wantPort)
		}
	}
}

// TestWriteManifest_HappyPath confirms the on-disk format matches the
// operator-script convention (so scripts/restore.sh can consume it
// transparently). Verifies the chronicle_pre_migrate=1 flag, version
// fields, and per-artifact lines.
func TestWriteManifest_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfg := HealthCheckConfig{BackupDir: dir}
	timestamp := "20260426T120000Z"
	artifacts := []artifact{
		{kind: "db", basename: "chronicle_pre_migrate_db_20260426T120000Z.sql.gz",
			size: 1024, sha256: strings.Repeat("a", 64)},
		{kind: "media", basename: "chronicle_pre_migrate_media_20260426T120000Z.tar.gz",
			size: 4096, sha256: strings.Repeat("b", 64)},
		{kind: "redis", basename: "chronicle_pre_migrate_redis_20260426T120000Z.rdb",
			size: 256, sha256: strings.Repeat("c", 64)},
	}

	t.Setenv("CHRONICLE_VERSION", "0.0.2")
	manifestPath, err := writeManifest(cfg, timestamp, 22, artifacts)
	if err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	got := string(body)

	requiredLines := []string{
		"chronicle_manifest_version=1",
		"chronicle_pre_migrate=1",
		"timestamp=20260426T120000Z",
		"chronicle_version=0.0.2",
		"migration_version=22",
		"db_file=chronicle_pre_migrate_db_20260426T120000Z.sql.gz",
		"media_file=chronicle_pre_migrate_media_20260426T120000Z.tar.gz",
		"redis_file=chronicle_pre_migrate_redis_20260426T120000Z.rdb",
		"sha256=" + strings.Repeat("a", 64),
		"sha256=" + strings.Repeat("b", 64),
		"sha256=" + strings.Repeat("c", 64),
	}
	for _, line := range requiredLines {
		if !strings.Contains(got, line) {
			t.Errorf("manifest missing %q\nfull:\n%s", line, got)
		}
	}

	// Manifest must have 0600 perms — it carries sha256s of the dump
	// and is in the same directory as the dump itself, so leaking it
	// to other local users would be a small but real exposure.
	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("manifest mode = %o, want 0600", info.Mode().Perm())
	}
}

// TestWriteManifest_DefaultsUnknownVersion confirms the chronicle_version
// line falls back to "unknown" when the env var is unset.
func TestWriteManifest_DefaultsUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHRONICLE_VERSION", "")
	manifestPath, err := writeManifest(HealthCheckConfig{BackupDir: dir}, "ts", 0, nil)
	if err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	body, _ := os.ReadFile(manifestPath)
	if !strings.Contains(string(body), "chronicle_version=unknown") {
		t.Errorf("expected chronicle_version=unknown, got:\n%s", body)
	}
}

// TestPreMigrationBackup_NoOpWhenBackupDirEmpty pins the legacy
// behavior: an empty BackupDir disables the entire subsystem.
func TestPreMigrationBackup_NoOpWhenBackupDirEmpty(t *testing.T) {
	if err := PreMigrationBackup(nil, HealthCheckConfig{}); err != nil {
		t.Errorf("expected nil error when BackupDir is empty, got %v", err)
	}
}

// TestPreMigrationBackup_FailClosedNoMysqldump confirms BackupRequired
// converts a missing-tool warning into an error. Stubs PATH so
// LookPath fails. Without the failure surfacing, an operator who set
// BACKUP_REQUIRED=1 in production but forgot to install mariadb-client
// would think they had a safety net when they didn't.
func TestPreMigrationBackup_FailClosedNoMysqldump(t *testing.T) {
	// Wipe PATH so LookPath("mysqldump") fails deterministically. Save
	// and restore via t.Setenv (Go test framework restores at end).
	t.Setenv("PATH", "")
	if _, err := exec.LookPath("mysqldump"); err == nil {
		t.Skip("mysqldump still resolvable with empty PATH; skipping")
	}

	dir := t.TempDir()
	err := PreMigrationBackup(nil, HealthCheckConfig{
		BackupDir:      dir,
		BackupRequired: true,
	})
	if err == nil {
		t.Fatal("expected error when BACKUP_REQUIRED=1 and mysqldump missing")
	}
	if !strings.Contains(err.Error(), "BACKUP_REQUIRED") {
		t.Errorf("error should mention BACKUP_REQUIRED: %v", err)
	}
}

// TestPreMigrationBackup_FailOpenNoMysqldump confirms the default
// (BackupRequired=false) returns nil even when mysqldump is missing.
// Backwards-compat with deployments that don't have mariadb-client.
func TestPreMigrationBackup_FailOpenNoMysqldump(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := exec.LookPath("mysqldump"); err == nil {
		t.Skip("mysqldump still resolvable with empty PATH; skipping")
	}

	dir := t.TempDir()
	if err := PreMigrationBackup(nil, HealthCheckConfig{BackupDir: dir}); err != nil {
		t.Errorf("expected nil error in default mode without mysqldump, got %v", err)
	}
}

// TestRotatePreMigrationBackups_RemovesOldFiles plants files at known
// timestamps and confirms only those past the cutoff are removed.
// Covers the new 4-pattern glob (db/media/redis/manifest) plus the
// legacy `chronicle_pre_migrate_<TS>.sql.gz` (no `_db_` infix).
func TestRotatePreMigrationBackups_RemovesOldFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()

	// Old (10 days ago).
	oldTS := now.Add(-10 * 24 * time.Hour).Format("20060102T150405Z")
	// Recent (1 day ago).
	recentTS := now.Add(-1 * 24 * time.Hour).Format("20060102T150405Z")

	plant := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// One of each pattern, at both ages.
	plant("chronicle_pre_migrate_db_" + oldTS + ".sql.gz")
	plant("chronicle_pre_migrate_db_" + recentTS + ".sql.gz")
	plant("chronicle_pre_migrate_media_" + oldTS + ".tar.gz")
	plant("chronicle_pre_migrate_redis_" + oldTS + ".rdb")
	plant("chronicle_pre_migrate_manifest_" + oldTS + ".txt")
	// Legacy: pre-symmetry filename with no _db_ infix.
	plant("chronicle_pre_migrate_" + oldTS + ".sql.gz")
	// Unrelated file we must not touch.
	plant("operator_dump.sql.gz")

	rotatePreMigrationBackups(HealthCheckConfig{
		BackupDir:    dir,
		BackupMaxAge: 7 * 24 * time.Hour,
	})

	mustExist := func(name string) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should still exist: %v", name, err)
		}
	}
	mustNotExist := func(name string) {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should be gone, Stat err = %v", name, err)
		}
	}

	mustExist("chronicle_pre_migrate_db_" + recentTS + ".sql.gz")
	mustExist("operator_dump.sql.gz")
	mustNotExist("chronicle_pre_migrate_db_" + oldTS + ".sql.gz")
	mustNotExist("chronicle_pre_migrate_media_" + oldTS + ".tar.gz")
	mustNotExist("chronicle_pre_migrate_redis_" + oldTS + ".rdb")
	mustNotExist("chronicle_pre_migrate_manifest_" + oldTS + ".txt")
	mustNotExist("chronicle_pre_migrate_" + oldTS + ".sql.gz")
}
