package systems

import (
	"os"
	"path/filepath"
	"testing"
)

// mkVersionDirs creates <base>/<slug>/<ver>/data.txt (5 bytes each) for every
// version and returns the slug dir. Used to exercise computeStaleVersions.
func mkVersionDirs(t *testing.T, base, slug string, versions []string) string {
	t.Helper()
	slugDir := filepath.Join(base, slug)
	for _, v := range versions {
		vd := filepath.Join(slugDir, v)
		if err := os.MkdirAll(vd, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", vd, err)
		}
		if err := os.WriteFile(filepath.Join(vd, "data.txt"), []byte("xxxxx"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return slugDir
}

func versionsOf(stale []staleVersion) map[string]bool {
	m := map[string]bool{}
	for _, s := range stale {
		m[s.Version] = true
	}
	return m
}

// TestComputeStaleVersions_ProtectsNewestInstalledLoaded is the core safety
// case: the newest, the DB-installed, and a currently-loaded (even if OLD)
// version are all kept; only genuinely stale folders are reclaimable.
func TestComputeStaleVersions_ProtectsNewestInstalledLoaded(t *testing.T) {
	base := t.TempDir()
	slugDir := mkVersionDirs(t, base, "drawsteel", []string{"0.0.7", "0.12.0", "0.13.0", "0.13.4"})
	installed := []InstalledPackage{{
		Slug:        "drawsteel",
		Version:     "0.13.0", // DB-installed (NOT the newest)
		InstallPath: filepath.Join(slugDir, "0.13.0"),
	}}
	// Loader is (wrongly) still serving an OLD version — must be protected.
	loaded := map[string]bool{filepath.Join(slugDir, "0.12.0"): true}

	stale := computeStaleVersions(installed, loaded, 1)
	got := versionsOf(stale)

	// Protected: 0.13.4 (newest), 0.13.0 (DB), 0.12.0 (loaded). Reclaim: 0.0.7.
	if !got["0.0.7"] {
		t.Errorf("expected 0.0.7 reclaimable, got %v", got)
	}
	for _, p := range []string{"0.13.4", "0.13.0", "0.12.0"} {
		if got[p] {
			t.Errorf("protected version %s must not be reclaimable, got %v", p, got)
		}
	}
	if len(stale) != 1 {
		t.Fatalf("expected exactly 1 reclaimable, got %d (%v)", len(stale), got)
	}
	if stale[0].Size != 5 {
		t.Errorf("expected 5-byte size for 0.0.7, got %d", stale[0].Size)
	}
}

// TestComputeStaleVersions_KeepNewestN keeps the top N versions.
func TestComputeStaleVersions_KeepNewestN(t *testing.T) {
	base := t.TempDir()
	slugDir := mkVersionDirs(t, base, "drawsteel", []string{"0.0.7", "0.12.0", "0.13.0", "0.13.4"})
	installed := []InstalledPackage{{Slug: "drawsteel", Version: "0.13.4", InstallPath: filepath.Join(slugDir, "0.13.4")}}

	stale := computeStaleVersions(installed, nil, 2) // keep 0.13.4 + 0.13.0
	got := versionsOf(stale)
	if !got["0.12.0"] || !got["0.0.7"] {
		t.Errorf("expected 0.12.0 and 0.0.7 reclaimable, got %v", got)
	}
	if got["0.13.4"] || got["0.13.0"] {
		t.Errorf("top-2 versions must be kept, got %v", got)
	}
}

// TestComputeStaleVersions_NothingWhenAtOrUnderKeep — a package with only the
// versions we always keep yields nothing.
func TestComputeStaleVersions_NothingWhenAtOrUnderKeep(t *testing.T) {
	base := t.TempDir()
	slugDir := mkVersionDirs(t, base, "drawsteel", []string{"0.13.4"})
	installed := []InstalledPackage{{Slug: "drawsteel", Version: "0.13.4", InstallPath: filepath.Join(slugDir, "0.13.4")}}
	if stale := computeStaleVersions(installed, nil, 1); len(stale) != 0 {
		t.Errorf("single version should yield nothing, got %v", stale)
	}
}

// TestComputeStaleVersions_KeepNewestClampedToOne — keepNewest < 1 clamps to 1.
func TestComputeStaleVersions_KeepNewestClampedToOne(t *testing.T) {
	base := t.TempDir()
	slugDir := mkVersionDirs(t, base, "drawsteel", []string{"0.12.0", "0.13.4"})
	installed := []InstalledPackage{{Slug: "drawsteel", Version: "0.13.4", InstallPath: filepath.Join(slugDir, "0.13.4")}}
	stale := computeStaleVersions(installed, nil, 0) // clamped → keep newest only
	got := versionsOf(stale)
	if !got["0.12.0"] || got["0.13.4"] {
		t.Errorf("expected only 0.12.0 reclaimable with keepNewest clamped to 1, got %v", got)
	}
}
