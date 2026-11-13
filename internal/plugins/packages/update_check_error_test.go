package packages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gitHubOutagePage stands in for the HTML page GitHub serves during an outage.
const gitHubOutagePage = `<!DOCTYPE html><html><head><title>Unicorn! &middot; GitHub</title><style>body{}</style></head></html>`

// TestListReleases_FailureMessages pins that a failed release lookup is
// worded for the admin page, never GitHub's response body, and which
// failures count as GitHub being temporarily down.
func TestListReleases_FailureMessages(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		header    map[string]string
		temporary bool
		want      string
	}{
		{name: "gateway timeout", status: http.StatusGatewayTimeout, temporary: true, want: "temporary problem (HTTP 504)"},
		{name: "server error", status: http.StatusInternalServerError, temporary: true, want: "temporary problem (HTTP 500)"},
		{name: "rate limited", status: http.StatusForbidden, header: map[string]string{"X-RateLimit-Remaining": "0"}, want: "rate limit"},
		{name: "too many requests", status: http.StatusTooManyRequests, want: "rate limit"},
		{name: "missing repo", status: http.StatusNotFound, want: "no such repository"},
		{name: "bad token", status: http.StatusUnauthorized, want: "GITHUB_TOKEN"},
		{name: "forbidden", status: http.StatusForbidden, want: "refused the request (HTTP 403)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(gitHubOutagePage))
			}))
			defer srv.Close()

			c := &GitHubClient{httpClient: srv.Client(), apiBase: srv.URL}
			_, err := c.ListReleases(context.Background(), "https://github.com/o/r")
			if err == nil {
				t.Fatal("want an error")
			}
			msg := err.Error()
			if strings.ContainsAny(msg, "<>") || strings.Contains(msg, "Unicorn") {
				t.Errorf("message carries GitHub's response body: %q", msg)
			}
			if !strings.Contains(msg, tt.want) {
				t.Errorf("message = %q, want it to mention %q", msg, tt.want)
			}
			if got := isTemporaryGitHubError(err); got != tt.temporary {
				t.Errorf("temporary = %v, want %v", got, tt.temporary)
			}
		})
	}
}

// TestListReleases_NoAnswerIsTemporary: GitHub not answering at all is an
// outage too.
func TestListReleases_NoAnswerIsTemporary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	c := &GitHubClient{httpClient: http.DefaultClient, apiBase: base}
	_, err := c.ListReleases(context.Background(), "https://github.com/o/r")
	if !isTemporaryGitHubError(err) {
		t.Fatalf("err = %v, want a temporary GitHub error", err)
	}
	if !strings.Contains(err.Error(), "didn't respond") {
		t.Errorf("message = %q", err.Error())
	}
}

// TestRunAutoUpdates_CheckFailureBanner pins what a failed scheduled check
// leaves on /admin/packages: nothing for an outage, a plain message for a
// real problem, and a working check clears only its own banner.
func TestRunAutoUpdates_CheckFailureBanner(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		lastError string
		want      string
	}{
		{name: "outage leaves no banner", status: http.StatusGatewayTimeout, want: ""},
		{name: "outage keeps an earlier banner", status: http.StatusBadGateway, lastError: "install 1.0.0 failed: boom", want: "install 1.0.0 failed: boom"},
		{name: "missing repo is reported", status: http.StatusNotFound, want: "update check failed: GitHub has no such repository, or it is private"},
		{name: "working check clears a check banner", status: http.StatusOK, lastError: "update check failed: GitHub API returned 504: <!DOCTYPE html>", want: ""},
		{name: "working check keeps an install banner", status: http.StatusOK, lastError: "install 1.0.0 failed: boom", want: "install 1.0.0 failed: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				if tt.status == http.StatusOK {
					_, _ = w.Write([]byte(`[]`))
					return
				}
				_, _ = w.Write([]byte(gitHubOutagePage))
			}))
			defer srv.Close()

			repo := newFakeRepo()
			repo.packages["p"] = &Package{
				ID: "p", Slug: "p", Status: StatusApproved, AutoUpdate: UpdateNightly,
				RepoURL: "https://github.com/o/r", LastError: tt.lastError,
			}
			gh := &GitHubClient{httpClient: srv.Client(), apiBase: srv.URL}
			svc := NewPackageService(repo, gh, t.TempDir(), "http://chronicle.test")

			if err := svc.RunAutoUpdates(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := repo.packages["p"].LastError; got != tt.want {
				t.Errorf("last error = %q, want %q", got, tt.want)
			}
		})
	}
}
