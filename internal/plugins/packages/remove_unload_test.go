package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveAndArchiveStopServing pins that removing a system package takes
// every version folder with it and stops serving it now, that archiving stops
// serving it, and that unarchiving serves the installed version again.
func TestRemoveAndArchiveStopServing(t *testing.T) {
	media := t.TempDir()
	root := filepath.Join(media, "packages", "systems", "pack")
	for _, v := range []string{"1.0.0", "1.1.0"} {
		if err := os.MkdirAll(filepath.Join(root, v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(media, "packages", "systems", "other", "1.0.0")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := newFakeRepo()
	installed := filepath.Join(root, "1.1.0")
	repo.packages["pkg-1"] = &Package{ID: "pkg-1", Slug: "pack", Type: PackageTypeSystem, InstallPath: installed, Status: StatusApproved}
	svc := NewPackageService(repo, &GitHubClient{}, media, "http://x").(*packageService)
	var unloaded, loaded []string
	SetOnSystemUnload(svc, func(dir string) { unloaded = append(unloaded, dir) })
	SetOnSystemInstall(svc, func(dir string) { loaded = append(loaded, dir) })
	ctx := context.Background()

	if err := svc.ArchivePackage(ctx, "pkg-1"); err != nil {
		t.Fatal(err)
	}
	if len(unloaded) != 1 || unloaded[0] != root {
		t.Fatalf("archive unloaded %v, want [%s]", unloaded, root)
	}
	if err := svc.UnarchivePackage(ctx, "pkg-1"); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0] != installed {
		t.Fatalf("unarchive loaded %v, want [%s]", loaded, installed)
	}

	if err := svc.RemovePackage(ctx, "pkg-1"); err != nil {
		t.Fatal(err)
	}
	if len(unloaded) != 2 || unloaded[1] != root {
		t.Fatalf("remove unloaded %v", unloaded)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("a version folder survived removal: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("another package's folder was touched: %v", err)
	}
}

// A slug that can't name one folder never becomes a path to delete.
func TestPackageRootRefusesOddSlugs(t *testing.T) {
	svc := &packageService{mediaDir: t.TempDir()}
	for _, slug := range []string{"", "..", "../x", "a/b", `a\b`, "/abs"} {
		if got := svc.packageRoot(&Package{Slug: slug, Type: PackageTypeSystem}); got != "" {
			t.Errorf("slug %q gave root %q", slug, got)
		}
	}
}
