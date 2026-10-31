package packages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRepo(t *testing.T) {
	tests := []struct {
		url, owner, repo string
		wantErr          bool
	}{
		{url: "https://github.com/keyxmakerx/Chronicle-Draw-Steel", owner: "keyxmakerx", repo: "Chronicle-Draw-Steel"},
		{url: "https://github.com/keyxmakerx/Chronicle-DnD-5.5e", owner: "keyxmakerx", repo: "Chronicle-DnD-5.5e"},
		{url: "https://github.com/keyxmakerx/Chronicle-DnD-5.5e.git", owner: "keyxmakerx", repo: "Chronicle-DnD-5.5e"},
		{url: "https://github.com/keyxmakerx/Chronicle-DnD-5.5e/releases", owner: "keyxmakerx", repo: "Chronicle-DnD-5.5e"},
		{url: "git@github.com:keyxmakerx/my_pack.v2.git", owner: "keyxmakerx", repo: "my_pack.v2"},
		{url: "https://github.com/keyxmakerx/..", wantErr: true},
		{url: "https://github.com/keyxmakerx/.git", wantErr: true},
		{url: "https://github.com/keyxmakerx", wantErr: true},
		// The repository shown must be the one fetched.
		{url: "https://gitlab.com/x/github.com/evil/pack", wantErr: true},
		{url: "https://evilgithub.com/evil/pack", wantErr: true},
		{url: "https://github.com.evil.example/evil/pack", wantErr: true},
		{url: "http://github.com/keyxmakerx/Chronicle-Draw-Steel", wantErr: true},
	}
	for _, tt := range tests {
		owner, repo, err := parseRepo(tt.url)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseRepo(%q) = %q/%q, want error", tt.url, owner, repo)
			}
			continue
		}
		if err != nil || owner != tt.owner || repo != tt.repo {
			t.Errorf("parseRepo(%q) = %q/%q, %v; want %q/%q", tt.url, owner, repo, err, tt.owner, tt.repo)
		}
	}
}

// TestIsGitHubDownloadURL pins where a download, and its redirects, may go.
func TestIsGitHubDownloadURL(t *testing.T) {
	tests := map[string]bool{
		"https://api.github.com/repos/o/r/zipball/v1":               true,
		"https://github.com/o/r/releases/download/v1/pack.zip":      true,
		"https://codeload.github.com/o/r/legacy.zip/refs/tags/v1":   true,
		"https://objects.githubusercontent.com/github-production/x": true,
		"https://release-assets.githubusercontent.com/github-x/y":   true,
		"http://github.com/o/r/releases/download/v1/pack.zip":       false,
		"https://github.com:8443/o/r":                               false,
		"https://example.com/pack.zip":                              false,
		"https://githubusercontent.com.evil.example/pack.zip":       false,
		"https://169.254.169.254/latest/meta-data":                  false,
	}
	for raw, want := range tests {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := isGitHubDownloadURL(u); got != want {
			t.Errorf("isGitHubDownloadURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

// TestDownloadAsset_RefusesRedirectOffGitHub pins that a redirect can't take
// a download, and the token sent with it, off GitHub.
func TestDownloadAsset_RefusesRedirectOffGitHub(t *testing.T) {
	var hitOther bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hitOther = true }))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x.zip", http.StatusFound)
	}))
	defer first.Close()
	c := &GitHubClient{httpClient: first.Client(), hostAllowed: func(u *url.URL) bool { return u.Host == strings.TrimPrefix(first.URL, "http://") }}
	if _, err := c.DownloadAsset(context.Background(), first.URL, filepath.Join(t.TempDir(), "x.zip")); err == nil || hitOther {
		t.Fatalf("redirect off the allowed host was followed: err=%v hit=%v", err, hitOther)
	}
}
