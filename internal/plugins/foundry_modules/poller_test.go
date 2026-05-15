package foundry_modules

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// fakeGitHub stands in for the GitHub API. Two URLs: /releases returns
// a canned list; asset URLs return a synthesized zip.
func fakeGitHub(t *testing.T, releases []gitHubRelease, zipBytes []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case bytes.HasSuffix([]byte(r.URL.Path), []byte("/releases")):
			_ = json.NewEncoder(w).Encode(releases)
		default:
			// Asset download path. Serve the zip regardless.
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		}
	}))
}

// buildModuleZip produces a zip containing a single module.json.
func buildModuleZip(t *testing.T, manifest map[string]any) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("module.json")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if err := json.NewEncoder(f).Encode(manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// stubRepoForPoller is a minimal in-memory repo used by the poller
// tests. The real mockRepo in service_test.go has many more methods
// than poll requires; a focused stub keeps test setup short.
type stubRepoForPoller struct {
	inserts          []*ModuleVersion
	getVersionByGHID func(int64) *ModuleVersion
	getByVersion     func(string) *ModuleVersion
}

func (s *stubRepoForPoller) InsertVersion(_ context.Context, v *ModuleVersion) error {
	s.inserts = append(s.inserts, v)
	return nil
}
func (s *stubRepoForPoller) GetVersion(_ context.Context, v string) (*ModuleVersion, error) {
	if s.getByVersion != nil {
		return s.getByVersion(v), nil
	}
	return nil, nil
}
func (s *stubRepoForPoller) GetVersionByID(_ context.Context, _ string) (*ModuleVersion, error) {
	return nil, nil
}
func (s *stubRepoForPoller) GetVersionByGitHubReleaseID(_ context.Context, id int64) (*ModuleVersion, error) {
	if s.getVersionByGHID != nil {
		return s.getVersionByGHID(id), nil
	}
	return nil, nil
}
func (s *stubRepoForPoller) ListVersions(_ context.Context, _ bool) ([]*ModuleVersion, error) {
	return nil, nil
}
func (s *stubRepoForPoller) SetVersionStatus(_ context.Context, _ string, _ ModuleStatus) error {
	return nil
}
func (s *stubRepoForPoller) LatestAvailable(_ context.Context) (*ModuleVersion, error) {
	return nil, nil
}
func (s *stubRepoForPoller) GetCampaignToken(_ context.Context, _ string) (*CampaignToken, error) {
	return nil, nil
}
func (s *stubRepoForPoller) UpsertCampaignToken(_ context.Context, _ *CampaignToken) error {
	return nil
}
func (s *stubRepoForPoller) BumpCampaignTokenVersion(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (s *stubRepoForPoller) CampaignsUsingVersion(_ context.Context, _ string) ([]CampaignUsage, error) {
	return nil, nil
}
func (s *stubRepoForPoller) CampaignsOlderThan(_ context.Context, _ string, _ func(a, b string) bool) ([]CampaignUsage, error) {
	return nil, nil
}

// stubEvents discards events; tests only care about the catalog state
// after a poll, not the audit log.
type stubEvents struct{}

func (stubEvents) LogEvent(_ context.Context, _, _, _, _, _ string, _ map[string]any) error {
	return nil
}

// newPollerForTest wires a Poller against the test HTTP server.
// Overrides the canonical API base URL by hijacking listReleases via
// the githubRepo field — the test server hosts /repos/.../releases at
// the same shape, just on a different host.
func newPollerForTest(t *testing.T, repo Repository, server *httptest.Server) *Poller {
	t.Helper()
	dir, _ := os.MkdirTemp("", "foundry-modules-poller-*")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	p := &Poller{
		repo:       repo,
		storageDir: dir,
		// listReleases hits https://api.github.com/repos/<repo>/releases.
		// Replace the hostname with the test server's by stashing the
		// full repo URL — listReleases concatenates "https://api.github.com/"
		// + repo + "/releases", but for tests we want the path under
		// the test server. Easier path: monkey-patch via subclass… no
		// such mechanism in Go. Instead, expose the httpClient and let
		// the test server respond to any path.
		githubRepo: "test/repo",
		interval:   0,
		events:     stubEvents{},
		httpClient: server.Client(),
	}
	// Replace the default transport so URLs in the poller resolve to
	// the test server regardless of hostname.
	p.httpClient = &http.Client{
		Timeout:   httpTimeout,
		Transport: &redirectTransport{base: server.URL},
	}
	return p
}

// redirectTransport rewrites every request URL to point at base,
// preserving the path. Lets the poller's hardcoded api.github.com
// URLs hit the test server.
type redirectTransport struct {
	base string
}

func (t *redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Build the new URL: base + original path.
	newURL := t.base + r.URL.Path
	if r.URL.RawQuery != "" {
		newURL += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, newURL, r.Body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header.Clone()
	return http.DefaultTransport.RoundTrip(req)
}

// --- tests ---

// PollOnce against a server with one release should ingest one row.
func TestPollOnce_IngestsNewRelease(t *testing.T) {
	zipBytes := buildModuleZip(t, map[string]any{
		"id": "chronicle-sync", "version": "0.1.5",
	})
	releases := []gitHubRelease{{
		ID:          12345,
		TagName:     "v0.1.5",
		Name:        "0.1.5",
		PublishedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		Assets: []releaseAsset{
			{Name: assetZipName, BrowserDownloadURL: "http://example/asset", Size: int64(len(zipBytes))},
		},
	}}
	srv := fakeGitHub(t, releases, zipBytes)
	t.Cleanup(srv.Close)

	repo := &stubRepoForPoller{}
	p := newPollerForTest(t, repo, srv)

	n, errs := p.PollOnce(context.Background())
	if n != 1 {
		t.Errorf("expected 1 new version, got %d (errs: %v)", n, errs)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	if len(repo.inserts) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(repo.inserts))
	}
	got := repo.inserts[0]
	if got.Version != "0.1.5" {
		t.Errorf("version: got %q, want 0.1.5", got.Version)
	}
	if got.Source != SourceGitHubRelease {
		t.Errorf("source: got %q, want %q", got.Source, SourceGitHubRelease)
	}
	if got.GitHubReleaseID == nil || *got.GitHubReleaseID != 12345 {
		t.Errorf("github_release_id: got %v, want 12345", got.GitHubReleaseID)
	}
	if got.GitHubReleaseTag != "v0.1.5" {
		t.Errorf("github_release_tag: got %q, want v0.1.5", got.GitHubReleaseTag)
	}
}

// Idempotency: a second poll over the same release should not double-ingest.
func TestPollOnce_AlreadyIngestedSkipped(t *testing.T) {
	zipBytes := buildModuleZip(t, map[string]any{
		"id": "chronicle-sync", "version": "0.1.5",
	})
	releases := []gitHubRelease{{
		ID:          12345,
		TagName:     "v0.1.5",
		PublishedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		Assets:      []releaseAsset{{Name: assetZipName, BrowserDownloadURL: "http://example/asset", Size: int64(len(zipBytes))}},
	}}
	srv := fakeGitHub(t, releases, zipBytes)
	t.Cleanup(srv.Close)

	repo := &stubRepoForPoller{
		getVersionByGHID: func(id int64) *ModuleVersion {
			if id == 12345 {
				return &ModuleVersion{Version: "0.1.5", Source: SourceGitHubRelease}
			}
			return nil
		},
	}
	p := newPollerForTest(t, repo, srv)

	n, errs := p.PollOnce(context.Background())
	if n != 0 {
		t.Errorf("expected 0 new versions (idempotent), got %d", n)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	if len(repo.inserts) != 0 {
		t.Errorf("expected no inserts on re-poll, got %d", len(repo.inserts))
	}
}

// Release without a chronicle-sync.zip asset should be skipped (not
// an error — non-Foundry releases on the same repo are fine).
func TestPollOnce_ReleaseWithoutZipAssetSkipped(t *testing.T) {
	releases := []gitHubRelease{{
		ID:      99,
		TagName: "v0.0.1-docs",
		Assets:  []releaseAsset{{Name: "README.md", BrowserDownloadURL: "http://example/readme", Size: 100}},
	}}
	srv := fakeGitHub(t, releases, nil)
	t.Cleanup(srv.Close)

	repo := &stubRepoForPoller{}
	p := newPollerForTest(t, repo, srv)

	n, errs := p.PollOnce(context.Background())
	if n != 0 || len(errs) != 0 {
		t.Errorf("expected (0, nil), got (%d, %v)", n, errs)
	}
	if len(repo.inserts) != 0 {
		t.Errorf("expected no inserts, got %d", len(repo.inserts))
	}
}

// GitHub 500 / network failure must not crash the poller — it returns
// the error in errs and counts zero new versions.
func TestPollOnce_GitHubFailureReportedNotPanicking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server exploded`))
	}))
	t.Cleanup(srv.Close)

	repo := &stubRepoForPoller{}
	p := newPollerForTest(t, repo, srv)

	n, errs := p.PollOnce(context.Background())
	if n != 0 {
		t.Errorf("expected 0 new versions on error, got %d", n)
	}
	if len(errs) == 0 {
		t.Error("expected at least one error reported")
	}
}

// parseInterval covers the env-var parsing fall-throughs.
func TestParseInterval(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", defaultPollInterval},
		{"0", 0},
		{"1h", time.Hour},
		{"30m", 30 * time.Minute},
		{"nonsense", defaultPollInterval},
	}
	for _, tc := range cases {
		if got := parseInterval(tc.in); got != tc.want {
			t.Errorf("parseInterval(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
