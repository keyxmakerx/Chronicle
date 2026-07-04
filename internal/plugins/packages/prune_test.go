package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// prunePkgEnv builds a packageService over a temp media dir with one
// installed system package and the given on-disk version folders (5 bytes
// each). Returns the service and the slug dir.
func prunePkgEnv(t *testing.T, installedVersion string, versions []string) (*packageService, string) {
	t.Helper()
	repo := newFakeRepo()
	mediaDir := t.TempDir()
	svcIface := NewPackageService(repo, newOfflineGitHubClient(), mediaDir, "http://x")
	svc := svcIface.(*packageService)

	slugDir := filepath.Join(svc.packagesDir(), "systems", "drawsteel")
	for _, v := range versions {
		vd := filepath.Join(slugDir, v)
		if err := os.MkdirAll(vd, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(vd, "f.txt"), []byte("xxxxx"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	repo.packages["p1"] = &Package{
		ID: "p1", Type: PackageTypeSystem, Slug: "drawsteel",
		InstalledVersion: installedVersion,
		InstallPath:      filepath.Join(slugDir, installedVersion),
		Status:           StatusApproved,
	}
	return svc, slugDir
}

func staleVersions(res *PruneResult) map[string]bool {
	m := map[string]bool{}
	for _, s := range res.Reclaimable {
		m[s.Version] = true
	}
	return m
}

// TestPrune_FailsClosedWithoutProvider — no loaded-dirs signal → refuse.
func TestPrune_FailsClosedWithoutProvider(t *testing.T) {
	svc, _ := prunePkgEnv(t, "0.13.4", []string{"0.13.3", "0.13.4"})
	if _, err := svc.PruneStaleVersions(context.Background(), 1, true); err == nil {
		t.Fatal("prune must fail closed when the loaded-dirs provider is unwired")
	}
}

// TestPrune_ProtectsNewestInstalledLoaded — the core safety invariant, and
// dry-run deletes nothing.
func TestPrune_ProtectsNewestInstalledLoaded(t *testing.T) {
	svc, slugDir := prunePkgEnv(t, "0.13.0", []string{"0.0.7", "0.12.0", "0.13.0", "0.13.4"})
	// Loader (wrongly) still serving 0.12.0 — must be protected too.
	svc.loadedDirsFn = func() map[string]bool {
		return map[string]bool{filepath.Join(slugDir, "0.12.0"): true}
	}

	res, err := svc.PruneStaleVersions(context.Background(), 1, true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	got := staleVersions(res)
	if !got["0.0.7"] || len(res.Reclaimable) != 1 {
		t.Fatalf("want only 0.0.7 reclaimable (newest 0.13.4, db 0.13.0, loaded 0.12.0 kept), got %v", got)
	}
	if len(res.Removed) != 0 {
		t.Error("dry-run must delete nothing")
	}
	if _, err := os.Stat(filepath.Join(slugDir, "0.0.7")); err != nil {
		t.Error("dry-run must leave the folder on disk")
	}

	// Execute: only 0.0.7 goes; everything protected survives.
	res, err = svc.PruneStaleVersions(context.Background(), 1, false)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0].Version != "0.0.7" || res.BytesFreed != 5 {
		t.Fatalf("want 0.0.7 removed (5 bytes), got %+v", res)
	}
	for _, keep := range []string{"0.12.0", "0.13.0", "0.13.4"} {
		if _, err := os.Stat(filepath.Join(slugDir, keep)); err != nil {
			t.Errorf("protected version %s must survive: %v", keep, err)
		}
	}

	// Idempotent: re-run reclaims nothing.
	res, _ = svc.PruneStaleVersions(context.Background(), 1, false)
	if len(res.Reclaimable) != 0 {
		t.Errorf("re-run must be a no-op, got %v", staleVersions(res))
	}
}

// TestPrune_KeepNewestNAndFoundrySkipped — keep-N honored; foundry-module
// packages never touched.
func TestPrune_KeepNewestNAndFoundrySkipped(t *testing.T) {
	svc, slugDir := prunePkgEnv(t, "0.13.4", []string{"0.11.0", "0.12.0", "0.13.0", "0.13.4"})
	svc.loadedDirsFn = func() map[string]bool { return nil }

	// Foundry package with on-disk version dirs — must be ignored.
	// Derive the on-disk fragment from the PackageTypeFoundryModule enum
	// rather than a bare slug literal so this cross-plugin reference passes
	// through the PackageType constant (M-B2.1 plugin-isolation guard, T-B2).
	repo := svc.repo.(*fakeRepo)
	fDir := filepath.Join(svc.packagesDir(), string(PackageTypeFoundryModule), "0.1.0")
	if err := os.MkdirAll(fDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo.packages["f1"] = &Package{
		ID: "f1", Type: PackageTypeFoundryModule, Slug: "chronicle-sync",
		InstalledVersion: "0.2.0", Status: StatusApproved,
	}

	res, err := svc.PruneStaleVersions(context.Background(), 3, true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	got := staleVersions(res)
	if !got["0.11.0"] || len(res.Reclaimable) != 1 {
		t.Fatalf("keep-3 should reclaim only 0.11.0, got %v", got)
	}
	for _, s := range res.Reclaimable {
		if s.Slug == "chronicle-sync" {
			t.Error("foundry-module dirs must never be reclaimable")
		}
	}
	_ = slugDir
}

// TestPrune_TakesPerPackageInstallLock pins the concurrency fix: prune must
// hold the same per-package installLocks mutex InstallVersion takes, so a
// reclaim can't RemoveAll a version dir an install is mid-extract into.
// While the lock is held (as an in-flight install would), prune must block
// and touch nothing; once released it proceeds and reclaims the stale dir.
func TestPrune_TakesPerPackageInstallLock(t *testing.T) {
	svc, slugDir := prunePkgEnv(t, "0.13.0", []string{"0.0.7", "0.13.0"})
	svc.loadedDirsFn = func() map[string]bool { return nil }

	// Hold the package's install lock, standing in for a running InstallVersion.
	mu := svc.lockForPackage("p1")
	mu.Lock()

	done := make(chan struct{})
	go func() {
		_, _ = svc.PruneStaleVersions(context.Background(), 1, false)
		close(done)
	}()

	// Prune must NOT finish while the lock is held. (If it did, it never
	// took the lock — the bug this test guards against.)
	select {
	case <-done:
		t.Fatal("prune completed while the install lock was held — it did not take the lock")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := os.Stat(filepath.Join(slugDir, "0.0.7")); err != nil {
		t.Fatalf("stale dir removed while install lock held: %v", err)
	}

	// Release the lock; prune should now run to completion and reclaim 0.0.7.
	mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prune did not complete after the install lock was released")
	}
	if _, err := os.Stat(filepath.Join(slugDir, "0.0.7")); !os.IsNotExist(err) {
		t.Errorf("prune should have reclaimed 0.0.7 after lock release, stat err = %v", err)
	}
}
