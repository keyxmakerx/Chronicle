package packages

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// releasesTransport answers every request with one GitHub release listing.
type releasesTransport struct{ body string }

func (t releasesTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(t.body)), Header: http.Header{}}, nil
}

// TestReleaseCheckKeepsConcurrentAdminChanges pins that a release check,
// which holds the package row across a GitHub call, writes back only when it
// checked: an admin's pin and update policy set during the call survive.
func TestReleaseCheckKeepsConcurrentAdminChanges(t *testing.T) {
	repo := newFakeRepo()
	repo.packages["pkg-1"] = &Package{ID: "pkg-1", Slug: "pack", RepoURL: "https://github.com/o/pack", AutoUpdate: UpdateOnRelease, InstalledVersion: "1.0.0"}
	gh := &GitHubClient{httpClient: &http.Client{Transport: releasesTransport{body: `[{"tag_name":"v1.1.0"}]`}}}
	svc := NewPackageService(repo, gh, t.TempDir(), "http://x").(*packageService)

	stale, _ := repo.GetPackage(context.Background(), "pkg-1")
	// The admin pins and turns updates off while the check is in flight.
	_ = repo.SetPinnedVersion(context.Background(), "pkg-1", "1.0.0")
	_ = repo.SetAutoUpdate(context.Background(), "pkg-1", UpdateOff)

	if _, err := svc.fetchAndImportVersions(context.Background(), stale); err != nil {
		t.Fatalf("fetchAndImportVersions: %v", err)
	}
	got, _ := repo.GetPackage(context.Background(), "pkg-1")
	if got.PinnedVersion != "1.0.0" || got.AutoUpdate != UpdateOff {
		t.Fatalf("the check undid the admin's changes: pin %q policy %q", got.PinnedVersion, got.AutoUpdate)
	}
	if got.LastCheckedAt == nil {
		t.Fatal("the check time was not recorded")
	}
}
