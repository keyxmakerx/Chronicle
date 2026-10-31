package packages

import (
	"context"
	"strings"
	"testing"
)

// TestInstallVersion_RefusesReplacedDownload pins that a version is bound to
// its first download: installing it again from the same bytes works, and
// from different bytes (a moved tag or replaced asset) is refused.
func TestInstallVersion_RefusesReplacedDownload(t *testing.T) {
	first := buildZip(t, map[string]string{"manifest.json": validManifest})
	svc, repo, _ := installTestEnv(t, first)
	SetManifestValidator(svc, func(string) error { return nil })
	ctx := context.Background()

	if err := svc.InstallVersion(ctx, "pkg-1", "0.2.0"); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if repo.digests["ver-2"] == "" {
		t.Fatal("the first download's digest was not recorded")
	}
	if err := svc.InstallVersion(ctx, "pkg-1", "0.2.0"); err != nil {
		t.Fatalf("reinstall of the same bytes: %v", err)
	}

	recorded := repo.digests["ver-2"]
	repo.digests["ver-2"] = strings.Repeat("0", 64)
	err := svc.InstallVersion(ctx, "pkg-1", "0.2.0")
	if err == nil || !strings.Contains(err.Error(), "no longer the file first downloaded") {
		t.Fatalf("a download that differs from the first was installed: %v", err)
	}
	if repo.digests["ver-2"] == recorded {
		t.Fatal("the recorded digest was overwritten")
	}
}

// TestInstallVersion_RefusesSlugThatIsAPath pins that a slug which would name
// a path rather than one file is refused before anything is downloaded.
func TestInstallVersion_RefusesSlugThatIsAPath(t *testing.T) {
	svc, repo, _ := installTestEnv(t, buildZip(t, map[string]string{"manifest.json": validManifest}))
	for _, slug := range []string{"../x", `a\b`, "a/b"} {
		repo.packages["pkg-1"].Slug = slug
		if err := svc.InstallVersion(context.Background(), "pkg-1", "0.2.0"); err == nil || !strings.Contains(err.Error(), "file name") {
			t.Errorf("slug %q: %v", slug, err)
		}
	}
}
