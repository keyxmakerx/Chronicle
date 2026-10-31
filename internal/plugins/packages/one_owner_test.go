package packages

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOneFoundryModulePackage pins that a second Foundry module package can't
// be added or approved while one is registered: they would share one folder.
func TestOneFoundryModulePackage(t *testing.T) {
	repo := newFakeRepo()
	repo.packages["fm-1"] = &Package{ID: "fm-1", Name: "Chronicle Sync", Type: PackageTypeFoundryModule, Status: StatusApproved}
	repo.packages["fm-2"] = &Package{ID: "fm-2", Name: "Other", Type: PackageTypeFoundryModule, Status: StatusPending, RepoURL: "https://github.com/o/other"}
	svc := NewPackageService(repo, &GitHubClient{}, t.TempDir(), "http://x").(*packageService)
	ctx := context.Background()

	if _, err := svc.AddPackage(ctx, AddPackageInput{RepoURL: "https://github.com/o/second", Type: string(PackageTypeFoundryModule)}); err == nil || !strings.Contains(err.Error(), "Foundry module package is already") {
		t.Fatalf("a second Foundry module package was added: %v", err)
	}
	if err := svc.ReviewPackage(ctx, "fm-2", "admin", ReviewPackageInput{Action: "approve"}); err == nil || !strings.Contains(err.Error(), "Foundry module package is already") {
		t.Fatalf("a second Foundry module package was approved: %v", err)
	}
	if err := svc.ensureSingleFoundryModule(ctx, "fm-1"); err == nil {
		t.Fatal("the pending second package was not counted")
	}
	delete(repo.packages, "fm-2")
	if err := svc.ensureSingleFoundryModule(ctx, "fm-1"); err != nil {
		t.Fatalf("the one package was refused against itself: %v", err)
	}
}

// TestSystemIDBelongsToOnePackage pins that a system package can't install a
// manifest whose id another package's folder already serves, while a new
// version of the same package can.
func TestSystemIDBelongsToOnePackage(t *testing.T) {
	media := t.TempDir()
	svc := NewPackageService(newFakeRepo(), &GitHubClient{}, media, "http://x").(*packageService)
	served := filepath.Join(media, "packages", "systems", "drawsteel-pack", "1.0.0")
	SetSystemDirProvider(svc, func(id string) string {
		if id == "drawsteel" {
			return served
		}
		return ""
	})
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), []byte(`{"id":"drawsteel"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := svc.checkSystemIDOwner(&Package{Slug: "lookalike", Type: PackageTypeSystem}, stage); err == nil {
		t.Fatal("another package took over a served system id")
	}
	if err := svc.checkSystemIDOwner(&Package{Slug: "drawsteel-pack", Type: PackageTypeSystem}, stage); err != nil {
		t.Fatalf("the owning package's own update was refused: %v", err)
	}
	if err := svc.checkSystemIDOwner(&Package{Slug: "drawsteel", Type: PackageTypeSystem}, stage); err == nil {
		t.Fatal("a slug that is a prefix-neighbour of the owner's folder was accepted")
	}
}
