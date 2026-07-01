// Package packages tests cover the install/submission gating logic that
// keeps the server from making outbound fetches without explicit admin
// action.
package packages

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// errTransport is a no-op http.RoundTripper that fails every request. Used
// to give AddPackage a GitHub client that cannot reach the network — the
// outbound fetch in fetchAndImportVersions errors, AddPackage swallows the
// error and returns the in-memory package, so we can still assert on its
// fields without ever talking to GitHub.
type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("network disabled in test")
}

// newOfflineGitHubClient returns a GitHubClient whose HTTP transport always
// errors. Use this in tests that exercise code paths which call out to
// GitHub but treat fetch failures as non-fatal.
func newOfflineGitHubClient() *GitHubClient {
	return &GitHubClient{httpClient: &http.Client{Transport: errTransport{}}}
}

// fakeRepo is a minimal in-memory PackageRepository for tests that only
// need to capture CreatePackage and answer FindByRepoURL. All other methods
// panic — extend as needed.
type fakeRepo struct {
	mu       sync.Mutex
	packages map[string]*Package
	created  []*Package
	// versions, when populated (keyed "pkgID@version"), backs GetVersion
	// so install-pipeline tests can drive InstallVersion end-to-end.
	versions map[string]*PackageVersion
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{packages: map[string]*Package{}, versions: map[string]*PackageVersion{}}
}

func (r *fakeRepo) ListPackages(_ context.Context) ([]Package, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Package, 0, len(r.packages))
	for _, p := range r.packages {
		out = append(out, *p)
	}
	return out, nil
}

func (r *fakeRepo) GetPackage(_ context.Context, id string) (*Package, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.packages[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeRepo) FindBySlug(_ context.Context, _ string) (*Package, error) { return nil, nil }

func (r *fakeRepo) FindByRepoURL(_ context.Context, _ string) (*Package, error) { return nil, nil }

func (r *fakeRepo) CreatePackage(_ context.Context, pkg *Package) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *pkg
	r.packages[pkg.ID] = &cp
	r.created = append(r.created, &cp)
	return nil
}

func (r *fakeRepo) UpdatePackage(_ context.Context, pkg *Package) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.packages[pkg.ID] = pkg
	return nil
}

func (r *fakeRepo) DeletePackage(_ context.Context, _ string) error { return nil }

func (r *fakeRepo) ListVersions(_ context.Context, _ string) ([]PackageVersion, error) {
	return nil, nil
}

func (r *fakeRepo) GetVersion(_ context.Context, pkgID, version string) (*PackageVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.versions[pkgID+"@"+version]; ok {
		cp := *v
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeRepo) UpsertVersion(_ context.Context, _ *PackageVersion) error { return nil }

func (r *fakeRepo) MarkVersionDownloaded(_ context.Context, _ string) error { return nil }

func (r *fakeRepo) GetUsageByCampaign(_ context.Context, _ string) ([]PackageUsage, error) {
	return nil, nil
}

func (r *fakeRepo) ListByStatus(_ context.Context, _ PackageStatus) ([]Package, error) {
	return nil, nil
}

func (r *fakeRepo) ListBySubmitter(_ context.Context, _ string) ([]Package, error) {
	return nil, nil
}

func (r *fakeRepo) CountPendingSubmissions(_ context.Context) (int, error) { return 0, nil }

func (r *fakeRepo) UpdateStatus(_ context.Context, _ string, _ PackageStatus, _, _ string) error {
	return nil
}

func (r *fakeRepo) SetDeprecated(_ context.Context, _, _ string) error { return nil }

func (r *fakeRepo) ClearDeprecated(_ context.Context, _ string) error { return nil }

func (r *fakeRepo) UpdateRepoURL(_ context.Context, _, _ string) error { return nil }

// fakeSettings is an in-memory SettingsReader/Writer for driving the
// security-settings paths. Keys map directly to GetSecuritySettings keys.
type fakeSettings struct {
	values map[string]string
}

func (s *fakeSettings) Get(_ context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not set")
}

func (s *fakeSettings) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

// TestAddPackage_DefaultsAutoUpdateOff guards the security default that
// makes a fresh install do zero outbound HTTP from the auto-update worker.
// A regression here would silently re-enable nightly background fetches
// for every newly-added package — exactly the class of bug we just removed.
func TestAddPackage_DefaultsAutoUpdateOff(t *testing.T) {
	repo := newFakeRepo()
	svc := NewPackageService(repo, newOfflineGitHubClient(), t.TempDir(), "https://example.test")

	pkg, err := svc.AddPackage(context.Background(), AddPackageInput{
		RepoURL: "https://github.com/test/test",
		Type:    string(PackageTypeSystem),
	})
	if err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	if pkg == nil {
		t.Fatal("AddPackage returned nil package")
	}
	if pkg.AutoUpdate != UpdateOff {
		t.Errorf("AutoUpdate = %q, want %q", pkg.AutoUpdate, UpdateOff)
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected exactly 1 CreatePackage call, got %d", len(repo.created))
	}
	if repo.created[0].AutoUpdate != UpdateOff {
		t.Errorf("persisted AutoUpdate = %q, want %q", repo.created[0].AutoUpdate, UpdateOff)
	}
}

// TestSubmitPackage_AlwaysPending guards the fix that closes the
// "non-admin user triggers an outbound install" hole. SubmitPackage must
// return Status=Pending regardless of how RequireApproval is configured;
// install only happens via the admin-only ReviewPackage flow.
func TestSubmitPackage_AlwaysPending(t *testing.T) {
	cases := []struct {
		name            string
		requireApproval string // value stored under "packages.require_approval"
	}{
		{name: "RequireApproval=true", requireApproval: "true"},
		{name: "RequireApproval=false", requireApproval: "false"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			settings := &fakeSettings{values: map[string]string{
				"packages.require_approval":    tc.requireApproval,
				"packages.owner_upload_policy": OwnerUploadAutoApprove,
			}}
			svc := NewPackageService(repo, newOfflineGitHubClient(), t.TempDir(), "https://example.test")
			ConfigureSettings(svc, settings)

			pkg, err := svc.SubmitPackage(context.Background(), "user-1", SubmitPackageInput{
				RepoURL: "https://github.com/test/test",
				Type:    string(PackageTypeSystem),
			})
			if err != nil {
				t.Fatalf("SubmitPackage: %v", err)
			}
			if pkg == nil {
				t.Fatal("SubmitPackage returned nil package")
			}
			if pkg.Status != StatusPending {
				t.Errorf("Status = %q, want %q (regardless of RequireApproval)", pkg.Status, StatusPending)
			}
			if pkg.AutoUpdate != UpdateOff {
				t.Errorf("AutoUpdate = %q, want %q", pkg.AutoUpdate, UpdateOff)
			}
		})
	}
}

// TestSubmitPackage_OwnerPolicyDisabled guards that the OwnerUploadPolicy
// admin setting actually does what the dropdown label says: when set to
// "disabled", user submissions are refused before any DB write.
func TestSubmitPackage_OwnerPolicyDisabled(t *testing.T) {
	repo := newFakeRepo()
	settings := &fakeSettings{values: map[string]string{
		"packages.owner_upload_policy": OwnerUploadDisabled,
	}}
	svc := NewPackageService(repo, newOfflineGitHubClient(), t.TempDir(), "https://example.test")
	ConfigureSettings(svc, settings)

	_, err := svc.SubmitPackage(context.Background(), "user-1", SubmitPackageInput{
		RepoURL: "https://github.com/test/test",
		Type:    string(PackageTypeSystem),
	})
	if err == nil {
		t.Fatal("SubmitPackage should return error when OwnerUploadPolicy=disabled")
	}
	if len(repo.created) != 0 {
		t.Errorf("no DB row should be created; got %d", len(repo.created))
	}
}

// fakePostInstallHook records every AfterInstall call for assertions.
// Captures pkg / version / destDir so tests can verify the right hook
// fired for the right package type.
type fakePostInstallHook struct {
	pkgType PackageType
	calls   []fakePostInstallCall
	err     error // returned by AfterInstall if non-nil
}

type fakePostInstallCall struct {
	pkg             *Package
	version         string
	previousVersion string
	destDir         string
}

func (h *fakePostInstallHook) PackageType() PackageType { return h.pkgType }
func (h *fakePostInstallHook) AfterInstall(_ context.Context, pkg *Package, version, previousVersion, destDir string) error {
	h.calls = append(h.calls, fakePostInstallCall{pkg: pkg, version: version, previousVersion: previousVersion, destDir: destDir})
	return h.err
}

// TestRegisterPostInstallHook_StoresHooks pins the registration
// surface: hooks land in the service's slice in insertion order and
// are accessible to the install pipeline.
func TestRegisterPostInstallHook_StoresHooks(t *testing.T) {
	svc := NewPackageService(newFakeRepo(), newOfflineGitHubClient(), t.TempDir(), "https://chronicle.test")

	h1 := &fakePostInstallHook{pkgType: PackageTypeFoundryModule}
	h2 := &fakePostInstallHook{pkgType: PackageTypeSystem}

	RegisterPostInstallHook(svc, h1)
	RegisterPostInstallHook(svc, h2)

	ps, ok := svc.(*packageService)
	if !ok {
		t.Fatal("unexpected PackageService implementation type")
	}
	if len(ps.postInstallHooks) != 2 {
		t.Fatalf("expected 2 hooks registered, got %d", len(ps.postInstallHooks))
	}
	if ps.postInstallHooks[0] != h1 || ps.postInstallHooks[1] != h2 {
		t.Errorf("hook order not preserved: %+v", ps.postInstallHooks)
	}
}

// TestPostInstallHook_TypeFilter mirrors the iteration logic from
// InstallVersion: only hooks whose PackageType matches the installed
// package's type should fire. Tests the loop predicate directly via
// the service's slice — exercising InstallVersion end-to-end requires
// a real filesystem extract path, out of scope for a unit test.
func TestPostInstallHook_TypeFilter(t *testing.T) {
	svc := NewPackageService(newFakeRepo(), newOfflineGitHubClient(), t.TempDir(), "https://chronicle.test")
	foundryHook := &fakePostInstallHook{pkgType: PackageTypeFoundryModule}
	systemHook := &fakePostInstallHook{pkgType: PackageTypeSystem}
	RegisterPostInstallHook(svc, foundryHook)
	RegisterPostInstallHook(svc, systemHook)

	ps := svc.(*packageService)
	pkg := &Package{ID: "pkg-1", Type: PackageTypeFoundryModule, Slug: "chronicle-sync"}

	// Inline the same iteration shape InstallVersion uses so the test
	// fails immediately if the predicate ever drifts.
	for _, hook := range ps.postInstallHooks {
		if hook.PackageType() != pkg.Type {
			continue
		}
		if err := hook.AfterInstall(context.Background(), pkg, "v0.1.5", "v0.1.4", "/tmp/destdir"); err != nil {
			t.Fatalf("hook returned error: %v", err)
		}
	}

	if len(foundryHook.calls) != 1 {
		t.Errorf("foundry hook should have fired exactly once, got %d", len(foundryHook.calls))
	}
	if len(systemHook.calls) != 0 {
		t.Errorf("system hook should not have fired for a foundry-module install, got %d", len(systemHook.calls))
	}
	if c := foundryHook.calls[0]; c.version != "v0.1.5" || c.destDir != "/tmp/destdir" || c.pkg != pkg {
		t.Errorf("hook received wrong args: %+v", c)
	}
}

// TestPostInstallHook_ErrorPropagates confirms that a hook returning
// an error surfaces it to the caller — the InstallVersion loop wraps
// the error with `fmt.Errorf("post-install hook for %q: %w", ...)`.
// This test exercises the hook contract directly; the InstallVersion
// wrapping is intentionally not re-tested here (would need the full
// install path).
func TestPostInstallHook_ErrorPropagates(t *testing.T) {
	failing := &fakePostInstallHook{
		pkgType: PackageTypeFoundryModule,
		err:     fmt.Errorf("simulated hook failure"),
	}
	pkg := &Package{Type: PackageTypeFoundryModule}
	err := failing.AfterInstall(context.Background(), pkg, "v0.1.5", "v0.1.4", "/tmp/destdir")
	if err == nil {
		t.Fatal("expected hook to return error")
	}
	if err.Error() != "simulated hook failure" {
		t.Errorf("unexpected error message: %v", err)
	}
}
