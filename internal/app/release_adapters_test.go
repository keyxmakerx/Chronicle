package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewestStableRelease(t *testing.T) {
	releases := []githubRelease{
		{TagName: "snapshot-2026-11-01"},
		{TagName: "v1.1.0-rc1", Prerelease: true},
		{TagName: "v1.2.0", Draft: true},
		{TagName: "v1.0.9", Body: "## What's new\n- Old"},
		{TagName: "v1.0.10", Body: "## Security fixes\n- Pages no longer show Director notes to players by @someone in https://github.com/x/y/pull/1\n- Calendar export keeps moons\n- Third line\n\n**Full Changelog**: https://github.com/x/y/compare/a...b"},
		{TagName: "1.0.2-beta"},
	}
	got := newestStableRelease(releases)
	if got == nil || got.Version != "1.0.10" || got.Tag != "v1.0.10" {
		t.Fatalf("got %+v, want 1.0.10 (numeric order, drafts, prereleases and snapshots skipped)", got)
	}
	if !got.Security {
		t.Error("a Security heading wasn't read as security fixes")
	}
	want := []string{"Pages no longer show Director notes to players", "Calendar export keeps moons"}
	if len(got.Notes) != len(want) || got.Notes[0] != want[0] || got.Notes[1] != want[1] {
		t.Errorf("notes = %q, want %q", got.Notes, want)
	}
	if newestStableRelease([]githubRelease{{TagName: "snapshot-1"}, {TagName: "v2.0.0", Prerelease: true}}) != nil {
		t.Error("found a release where none is published")
	}
}

func TestHasSecurityHeading(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"## Security\n- fix", true},
		{"### Security fixes\n- fix", true},
		{"# security", true},
		{"## What's new\n- Better security for invites", false},
		{"Security: none", false},
		{"## Securityish", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := hasSecurityHeading(tt.body); got != tt.want {
			t.Errorf("hasSecurityHeading(%q) = %v, want %v", tt.body, got, tt.want)
		}
	}
}

func TestChronicleReleaseSource(t *testing.T) {
	var path, auth string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.1","body":"- Fix"},{"tag_name":"v1.0.0"}]`))
	}))
	defer srv.Close()
	s := &chronicleReleaseSource{client: srv.Client(), token: "t0k", apiBase: srv.URL}

	rel, err := s.LatestChronicleRelease(context.Background())
	if err != nil || rel == nil || rel.Version != "1.0.1" {
		t.Fatalf("rel = %+v, err = %v", rel, err)
	}
	if path != "/repos/keyxmakerx/Chronicle/releases" || auth != "Bearer t0k" {
		t.Errorf("asked %s with auth %q", path, auth)
	}

	status = http.StatusForbidden
	if _, err := s.LatestChronicleRelease(context.Background()); err == nil {
		t.Error("a refused request read as no release")
	}
}
