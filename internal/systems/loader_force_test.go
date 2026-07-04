package systems

import (
	"os"
	"path/filepath"
	"testing"
)

// writeManifestDir creates <base>/<name>/manifest.json with the given id and
// version and returns the dir. Minimal valid manifest (status defaults to
// coming_soon, so no instantiation is attempted — registration is the test
// subject).
func writeManifestDir(t *testing.T, base, name, id, version string) string {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	m := `{"id":"` + id + `","name":"Force Test","version":"` + version + `","api_version":"1"}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(m), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

// TestForceLoad_RollbackBeatsNewerLoaded pins the rollback fix: a normal
// load of an OLDER version is skipped by the WS-6 highest-version policy,
// but a forced load (explicit admin install) replaces the newer copy.
func TestForceLoad_RollbackBeatsNewerLoaded(t *testing.T) {
	base := t.TempDir()
	l := NewSystemLoader(base)

	newer := writeManifestDir(t, base, "0.2.0", "forcesys", "0.2.0")
	older := writeManifestDir(t, base, "0.1.0", "forcesys", "0.1.0")

	if err := l.loadSingleSystem(newer); err != nil {
		t.Fatalf("loading newer: %v", err)
	}

	// Normal (non-forced) load of the older copy must be skipped — the
	// pre-existing WS-6 behavior that made rollbacks silent no-ops.
	if err := l.loadSingleSystem(older); err != nil {
		t.Fatalf("non-forced older load errored: %v", err)
	}
	if got := l.Get("forcesys").Version; got != "0.2.0" {
		t.Fatalf("non-forced older load must NOT replace newer, got version %s", got)
	}

	// Forced load (explicit install) replaces regardless of version.
	if err := l.loadSingleSystemOpts(older, true); err != nil {
		t.Fatalf("forced older load errored: %v", err)
	}
	if got := l.Get("forcesys").Version; got != "0.1.0" {
		t.Errorf("forced load must replace with the installed version, got %s", got)
	}
	if got := l.Dir("forcesys"); got != older {
		t.Errorf("loader must serve the forced dir, got %s", got)
	}
}
