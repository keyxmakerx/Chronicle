package packages

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// buildZip returns an in-memory zip whose entries are the given
// name→content pairs. Used to serve a fake GitHub release asset.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// installTestEnv wires a real packageService against a temp media dir, a
// fakeRepo seeded with one system package + one version, and an httptest
// server serving zipContent as the release asset. Returns the service,
// repo, and the package for assertions.
func installTestEnv(t *testing.T, zipContent []byte) (PackageService, *fakeRepo, *Package) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipContent)
	}))
	t.Cleanup(server.Close)

	repo := newFakeRepo()
	pkg := &Package{
		ID:               "pkg-1",
		Type:             PackageTypeSystem,
		Slug:             "testsys",
		Name:             "Test System",
		InstalledVersion: "0.1.0", // pretend an older version is installed
	}
	repo.packages[pkg.ID] = pkg
	repo.versions["pkg-1@0.2.0"] = &PackageVersion{
		ID: "ver-2", PackageID: "pkg-1", Version: "0.2.0",
		DownloadURL: server.URL + "/testsys-0.2.0.zip",
	}

	gh := &GitHubClient{httpClient: server.Client()}
	svc := NewPackageService(repo, gh, t.TempDir(), "http://chronicle.test")
	return svc, repo, pkg
}

const validManifest = `{"id":"testsys","name":"Test System","version":"0.0.0","api_version":"1"}`

// TestInstallVersion_ManifestValidatorRejects pins the fail-loud contract
// for the shadow-failure class (Draw Steel 0.13.4): when the injected
// full-manifest validator rejects, the install must fail with that error,
// remove the extracted dir, and leave the DB row on the old version.
func TestInstallVersion_ManifestValidatorRejects(t *testing.T) {
	svc, repo, _ := installTestEnv(t, buildZip(t, map[string]string{"manifest.json": validManifest}))

	wantErr := errors.New(`entity preset "big": too many fields (101, max 100)`)
	SetManifestValidator(svc, func(manifestPath string) error {
		if _, err := os.Stat(manifestPath); err != nil {
			t.Errorf("validator should receive an existing manifest path, got %s: %v", manifestPath, err)
		}
		return wantErr
	})

	err := svc.InstallVersion(context.Background(), "pkg-1", "0.2.0")
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected validator error, got %v", err)
	}

	stored, _ := repo.GetPackage(context.Background(), "pkg-1")
	if stored.InstalledVersion != "0.1.0" {
		t.Errorf("DB row must stay on the old version, got %q", stored.InstalledVersion)
	}
	if s, ok := svc.(*packageService); ok {
		destDir := s.installDir(PackageTypeSystem, "testsys", "0.2.0")
		if _, statErr := os.Stat(destDir); !os.IsNotExist(statErr) {
			t.Errorf("destDir must be removed on validation failure, stat err = %v", statErr)
		}
	}
}

// TestInstallVersion_SuccessUpdatesDB is the happy path with a passing
// validator: DB row moves to the new version and the manifest is on disk
// with the rewritten release version.
func TestInstallVersion_SuccessUpdatesDB(t *testing.T) {
	svc, repo, _ := installTestEnv(t, buildZip(t, map[string]string{"manifest.json": validManifest}))
	validated := ""
	SetManifestValidator(svc, func(manifestPath string) error {
		validated = manifestPath
		return nil
	})

	if err := svc.InstallVersion(context.Background(), "pkg-1", "0.2.0"); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	stored, _ := repo.GetPackage(context.Background(), "pkg-1")
	if stored.InstalledVersion != "0.2.0" {
		t.Errorf("InstalledVersion = %q, want 0.2.0", stored.InstalledVersion)
	}
	if stored.InstallPath == "" {
		t.Error("InstallPath should be set")
	}
	if validated == "" {
		t.Error("validator should have been called for a system package")
	}
	// The version rewrite runs before validation, so the validated file
	// must carry the release tag, not the manifest's stale "0.0.0".
	data, err := os.ReadFile(filepath.Join(stored.InstallPath, "manifest.json"))
	if err != nil {
		t.Fatalf("reading installed manifest: %v", err)
	}
	if !bytes.Contains(data, []byte(`"0.2.0"`)) {
		t.Errorf("installed manifest should carry rewritten version 0.2.0, got %s", data)
	}
}

// failingHook is a PostInstallHook that always errors, for pinning the
// hooks-before-DB ordering contract.
type failingHook struct{ typ PackageType }

func (h failingHook) PackageType() PackageType { return h.typ }
func (h failingHook) AfterInstall(_ context.Context, _ *Package, _, _, _ string) error {
	return errors.New("hook exploded")
}

// TestInstallVersion_HookFailureLeavesDBUntouched pins the reordering fix:
// a post-install hook failure must abort BEFORE the DB row update, so the
// catalog never points at a directory the failure path just removed.
func TestInstallVersion_HookFailureLeavesDBUntouched(t *testing.T) {
	svc, repo, _ := installTestEnv(t, buildZip(t, map[string]string{"manifest.json": validManifest}))
	SetManifestValidator(svc, func(string) error { return nil })
	RegisterPostInstallHook(svc, failingHook{typ: PackageTypeSystem})

	err := svc.InstallVersion(context.Background(), "pkg-1", "0.2.0")
	if err == nil {
		t.Fatal("expected hook failure to fail the install")
	}

	stored, _ := repo.GetPackage(context.Background(), "pkg-1")
	if stored.InstalledVersion != "0.1.0" {
		t.Errorf("DB row must stay on the old version after hook failure, got %q", stored.InstalledVersion)
	}
	if stored.InstallPath != "" {
		t.Errorf("InstallPath must stay empty (old state), got %q", stored.InstallPath)
	}
	if s, ok := svc.(*packageService); ok {
		destDir := s.installDir(PackageTypeSystem, "testsys", "0.2.0")
		if _, statErr := os.Stat(destDir); !os.IsNotExist(statErr) {
			t.Errorf("destDir must be removed on hook failure, stat err = %v", statErr)
		}
	}
}

// TestInstallVersion_NilValidatorSkips preserves the no-validator behavior
// (tests / degraded boot): installs proceed without the full check.
func TestInstallVersion_NilValidatorSkips(t *testing.T) {
	svc, repo, _ := installTestEnv(t, buildZip(t, map[string]string{"manifest.json": validManifest}))
	if err := svc.InstallVersion(context.Background(), "pkg-1", "0.2.0"); err != nil {
		t.Fatalf("install without validator should succeed: %v", err)
	}
	stored, _ := repo.GetPackage(context.Background(), "pkg-1")
	if stored.InstalledVersion != "0.2.0" {
		t.Errorf("InstalledVersion = %q, want 0.2.0", stored.InstalledVersion)
	}
}
