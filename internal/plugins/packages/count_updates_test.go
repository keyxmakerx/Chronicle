package packages

import (
	"context"
	"testing"
)

// CountUpdatesReady must count exactly the packages the Updates tab lists.
func TestCountUpdatesReady(t *testing.T) {
	repo := newFakeRepo()
	repo.listed = []PackageVersion{{Version: "1.2.0"}, {Version: "1.0.0"}} // newest first, as the repository returns them
	for _, p := range []Package{
		{ID: "a", Slug: "a", Status: StatusApproved, InstalledVersion: "1.0.0"},                         // update ready
		{ID: "b", Slug: "b", Status: StatusApproved, InstalledVersion: "1.2.0"},                         // up to date
		{ID: "c", Slug: "c", Status: StatusApproved, InstalledVersion: "1.0.0", PinnedVersion: "1.0.0"}, // pinned
		{ID: "d", Slug: "d", Status: StatusApproved, InstalledVersion: "1.0.0", LastError: "boom"},      // failing
		{ID: "e", Slug: "e", Status: StatusApproved},                                                    // not installed
	} {
		p := p
		repo.packages[p.ID] = &p
	}
	svc := NewPackageService(repo, newOfflineGitHubClient(), t.TempDir(), "https://example.test")
	n, err := svc.CountUpdatesReady(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("CountUpdatesReady = %d, want 1", n)
	}
}
