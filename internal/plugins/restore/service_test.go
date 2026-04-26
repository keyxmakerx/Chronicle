// Tests for the restore service. The service shells out to
// scripts/restore.sh in production; tests substitute small shims so
// success / failure / timeout / single-flight paths are exercised
// without ever touching mariadb or the real restore tooling.
package restore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeManifest writes a fake chronicle_manifest_<TS>.txt that
// ListManifests + ResolveManifestPath should accept. Returns the
// basename so tests can pass it to RunRestore.
func writeManifest(t *testing.T, dir string) string {
	t.Helper()
	name := "chronicle_manifest_20260101T120000.txt"
	body := "chronicle_manifest_version=1\n" +
		"chronicle_version=0.0.1\n" +
		"migration_version=22\n" +
		"db_file=chronicle_db_20260101T120000.sql.gz sha256=abc size=100\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return name
}

// writeShim writes a tiny shell script that exits with the given code.
// Restore.sh's interface (--manifest --yes --force) is opaque to the
// shim — we just want to exercise the lifecycle around the shell-out.
func writeShim(t *testing.T, body string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "shim.sh")
	content := "#!/bin/sh\n" + body + "\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// TestRunRestore_Success pins the happy path: shim exits 0, RunResult
// reports success, ManifestName is captured, IsRunning settles back.
func TestRunRestore_Success(t *testing.T) {
	backupDir := t.TempDir()
	manifest := writeManifest(t, backupDir)
	script := writeShim(t, `echo restored ok`, 0)
	svc := NewService(Config{ScriptPath: script, BackupDir: backupDir, Timeout: 5 * time.Second})

	r, err := svc.RunRestore(context.Background(), manifest)
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if !r.Succeeded() {
		t.Errorf("expected success, got %+v", r)
	}
	if r.ManifestName != manifest {
		t.Errorf("ManifestName = %q, want %q", r.ManifestName, manifest)
	}
	if svc.IsRunning() {
		t.Errorf("IsRunning should be false after RunRestore returns")
	}
}

// TestRunRestore_RejectsBadManifest confirms the validator rejects
// path traversal before even reaching the shell-out.
func TestRunRestore_RejectsBadManifest(t *testing.T) {
	backupDir := t.TempDir()
	script := writeShim(t, `true`, 0)
	svc := NewService(Config{ScriptPath: script, BackupDir: backupDir, Timeout: 5 * time.Second})

	for _, bad := range []string{
		"../etc/passwd",
		"sub/file.txt",
		"chronicle_manifest_20260101T120000.txt", // doesn't exist on disk yet
		"chronicle_db_20260101T120000.sql.gz",    // wrong prefix
		"",
	} {
		if _, err := svc.RunRestore(context.Background(), bad); err == nil {
			t.Errorf("expected rejection for manifest %q", bad)
		}
	}
}

// TestRunRestore_NonZeroExit pins the failure path: shim exits 3,
// RunResult.ExitCode reflects it, Succeeded() is false.
func TestRunRestore_NonZeroExit(t *testing.T) {
	backupDir := t.TempDir()
	manifest := writeManifest(t, backupDir)
	script := writeShim(t, `echo failing >&2`, 3)
	svc := NewService(Config{ScriptPath: script, BackupDir: backupDir, Timeout: 5 * time.Second})

	r, err := svc.RunRestore(context.Background(), manifest)
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if r.Succeeded() {
		t.Errorf("expected failure")
	}
	if r.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", r.ExitCode)
	}
}

// TestRunRestore_Timeout confirms a long-running script is killed at
// the configured timeout and the whole process tree dies. Sleeps 5s
// but caps the test at <1s so the test itself doesn't hang.
func TestRunRestore_Timeout(t *testing.T) {
	backupDir := t.TempDir()
	manifest := writeManifest(t, backupDir)
	script := writeShim(t, `sleep 5`, 0)
	svc := NewService(Config{ScriptPath: script, BackupDir: backupDir, Timeout: 100 * time.Millisecond})

	start := time.Now()
	r, err := svc.RunRestore(context.Background(), manifest)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if !r.TimedOut {
		t.Errorf("expected TimedOut")
	}
	if elapsed > time.Second {
		t.Errorf("timeout did not stop child within reasonable bound: %s", elapsed)
	}
}

// TestRunRestore_SingleFlight confirms two simultaneous invocations
// don't both spawn restore.sh — the second returns ErrAlreadyRunning.
// This is the in-process backstop layered under the route's rate limit.
func TestRunRestore_SingleFlight(t *testing.T) {
	backupDir := t.TempDir()
	manifest := writeManifest(t, backupDir)
	script := writeShim(t, `sleep 0.3`, 0)
	svc := NewService(Config{ScriptPath: script, BackupDir: backupDir, Timeout: 5 * time.Second})

	var wg sync.WaitGroup
	var firstErr, secondErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, firstErr = svc.RunRestore(context.Background(), manifest)
	}()
	time.Sleep(50 * time.Millisecond)

	_, secondErr = svc.RunRestore(context.Background(), manifest)
	if secondErr != ErrAlreadyRunning {
		t.Errorf("second concurrent RunRestore should return ErrAlreadyRunning, got %v", secondErr)
	}
	wg.Wait()
	if firstErr != nil {
		t.Errorf("first RunRestore returned %v", firstErr)
	}
}

// TestListManifests_ParsesAndSorts pins the manifest parser. We write
// a manifest with all fields, scan, and confirm the parsed values
// match. Two manifests are written so the sort order is exercised.
func TestListManifests_ParsesAndSorts(t *testing.T) {
	backupDir := t.TempDir()
	older := "chronicle_manifest_20260101T100000.txt"
	newer := "chronicle_manifest_20260102T100000.txt"
	body := "chronicle_manifest_version=1\n" +
		"timestamp=20260101T100000\n" +
		"chronicle_version=0.0.2\n" +
		"migration_version=22\n" +
		"db_file=chronicle_db_x.sql.gz sha256=abc size=100\n" +
		"media_file=chronicle_media_x.tar.gz sha256=def size=200\n" +
		"redis_file=chronicle_redis_x.rdb sha256=ghi size=300\n"
	if err := os.WriteFile(filepath.Join(backupDir, older), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, newer), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Set explicit mtimes so sort order is deterministic.
	now := time.Now()
	os.Chtimes(filepath.Join(backupDir, older), now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	os.Chtimes(filepath.Join(backupDir, newer), now, now)

	// Drop a non-manifest file to confirm the filter excludes it.
	if err := os.WriteFile(filepath.Join(backupDir, "chronicle_db_x.sql.gz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &service{cfg: Config{ScriptPath: "/dev/null", BackupDir: backupDir}}
	got, err := svc.ListManifests()
	if err != nil {
		t.Fatalf("ListManifests: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(got))
	}
	if got[0].Name != newer {
		t.Errorf("expected newer first, got %v", got)
	}
	if got[0].ChronicleVersion != "0.0.2" {
		t.Errorf("ChronicleVersion = %q, want 0.0.2", got[0].ChronicleVersion)
	}
	if got[0].MigrationVersion != "22" {
		t.Errorf("MigrationVersion = %q, want 22", got[0].MigrationVersion)
	}
	if got[0].DBFile != "chronicle_db_x.sql.gz" {
		t.Errorf("DBFile = %q, want %q", got[0].DBFile, "chronicle_db_x.sql.gz")
	}
	if got[0].MediaFile != "chronicle_media_x.tar.gz" {
		t.Errorf("MediaFile = %q, want chronicle_media_x.tar.gz", got[0].MediaFile)
	}
	if got[0].RedisFile != "chronicle_redis_x.rdb" {
		t.Errorf("RedisFile = %q", got[0].RedisFile)
	}
}

// TestResolveManifestPath_RejectsTraversal pins the security guard for
// the manifest selector. Any of these inputs would let a confused or
// malicious admin point restore.sh at an arbitrary file.
func TestResolveManifestPath_RejectsTraversal(t *testing.T) {
	backupDir := t.TempDir()
	for _, bad := range []string{
		"../etc/passwd",
		"chronicle_manifest_../whatever.txt",
		"sub/chronicle_manifest_x.txt",
		"chronicle_manifest_x", // missing .txt
		"manifest.txt",         // missing prefix
		"",
	} {
		if _, err := ResolveManifestPath(backupDir, bad); err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
	}
}

// TestParseManifestInto_HandlesComments confirms parser tolerates
// extra unrecognized lines without erroring. Future manifest format
// changes should be backwards-compatible at least at the parser level.
func TestParseManifestInto_HandlesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chronicle_manifest_x.txt")
	body := "chronicle_manifest_version=1\n" +
		"# random comment line\n" +
		"unknown_key=ignore-me\n" +
		"chronicle_version=99.99.99\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var s ManifestSummary
	if err := parseManifestInto(&s, path); err != nil {
		t.Fatalf("parseManifestInto: %v", err)
	}
	if s.ChronicleVersion != "99.99.99" {
		t.Errorf("ChronicleVersion = %q, want 99.99.99", s.ChronicleVersion)
	}
}

// TestFirstToken pins the line-parsing helper used to pull
// `db_file=NAME` out of `db_file=NAME sha256=…`.
func TestFirstToken(t *testing.T) {
	cases := []struct {
		line, prefix, want string
	}{
		{"db_file=foo.sql.gz sha256=abc size=100", "db_file=", "foo.sql.gz"},
		{"db_file=just-this", "db_file=", "just-this"},
		{"db_file=", "db_file=", ""},
	}
	for _, c := range cases {
		if got := firstToken(c.line, c.prefix); got != c.want {
			t.Errorf("firstToken(%q,%q) = %q, want %q", c.line, c.prefix, got, c.want)
		}
	}
}

// TestCapBuf confirms the local capBuf truncation defense behaves the
// same as the backup plugin's. (Duplicated tests mirror duplicated
// implementation; both go away when the helper is extracted.)
func TestCapBuf_Truncates(t *testing.T) {
	b := newCapBuf(5)
	b.Write([]byte(strings.Repeat("x", 100)))
	if b.String() != "xxxxx" {
		t.Errorf("buffer = %q, want %q", b.String(), "xxxxx")
	}
}
