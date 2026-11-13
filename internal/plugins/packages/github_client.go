package packages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// repoPattern extracts owner/repo from a GitHub URL. It is anchored to the
// whole URL, so "github.com/owner/repo" appearing inside another host's path
// doesn't parse: the repository shown is always the one fetched. Repository
// names may contain dots (Chronicle-DnD-5.5e), so only GitHub's name
// characters end the match; a trailing ".git" is stripped by parseRepo.
var repoPattern = regexp.MustCompile(`^(?:https://(?:www\.)?github\.com/|git@github\.com:)([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)(?:[/?#].*)?$`)

// GitHubRelease represents a release from the GitHub Releases API.
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a downloadable file in a GitHub release.
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// GitHubClient fetches release information from GitHub repositories.
type GitHubClient struct {
	httpClient *http.Client
	token      string // Optional GitHub token for higher rate limits.
	// hostAllowed decides which hosts a download may reach, on the first
	// request and on every redirect; nil means isGitHubDownloadURL.
	hostAllowed func(*url.URL) bool
	// apiBase is the GitHub API root; empty means https://api.github.com.
	apiBase string
}

// GitHubAPIError is a failed release lookup, worded for the admin page.
// GitHub's response body is never part of the message: on an outage it is
// an HTML error page, and on other failures it says nothing an admin can act
// on. Status is 0 when GitHub could not be reached at all.
type GitHubAPIError struct {
	Status      int
	RateLimited bool
	Cause       error // transport error, for the logs only
}

func (e *GitHubAPIError) Error() string {
	switch {
	case e.Status == 0:
		return "GitHub didn't respond; it will be tried again later"
	case e.Status >= 500:
		return fmt.Sprintf("GitHub had a temporary problem (HTTP %d); it will be tried again later", e.Status)
	case e.RateLimited:
		return "GitHub's rate limit was reached; it resets within the hour, and a GITHUB_TOKEN raises it"
	case e.Status == http.StatusNotFound:
		return "GitHub has no such repository, or it is private"
	case e.Status == http.StatusUnauthorized:
		return "GitHub refused the GITHUB_TOKEN"
	default:
		return fmt.Sprintf("GitHub refused the request (HTTP %d)", e.Status)
	}
}

func (e *GitHubAPIError) Unwrap() error { return e.Cause }

// isTemporaryGitHubError reports whether err is GitHub being down or slow
// (a 5xx answer or no answer), which fixes itself and needs nothing from an
// admin. A rate limit is not temporary in this sense: if it keeps happening
// the server needs a token.
func isTemporaryGitHubError(err error) bool {
	var ge *GitHubAPIError
	if !errors.As(err, &ge) {
		return false
	}
	return ge.Status == 0 || ge.Status >= 500
}

// NewGitHubClient creates a GitHub API client. It reads GITHUB_TOKEN
// from the environment for authenticated requests (5000 req/hour vs 60).
func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      os.Getenv("GITHUB_TOKEN"),
	}
}

// isGitHubDownloadURL reports whether u is an HTTPS address on GitHub or the
// hosts it serves release files from. Download addresses come from the
// GitHub API's answer, so this keeps a tampered or unexpected answer, or a
// redirect, from sending the server (and its GitHub token) anywhere else.
func isGitHubDownloadURL(u *url.URL) bool {
	if u.Scheme != "https" || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "github.com", "api.github.com", "codeload.github.com":
		return true
	}
	return strings.HasSuffix(host, ".githubusercontent.com")
}

func (c *GitHubClient) downloadHostAllowed(u *url.URL) bool {
	if c.hostAllowed != nil {
		return c.hostAllowed(u)
	}
	return isGitHubDownloadURL(u)
}

// parseRepo extracts owner and repo from a GitHub URL.
// Supports https://github.com/owner/repo and git@github.com:owner/repo.
func parseRepo(repoURL string) (owner, repo string, err error) {
	matches := repoPattern.FindStringSubmatch(repoURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("cannot parse GitHub repo from URL: %s", repoURL)
	}
	repo = strings.TrimSuffix(matches[2], ".git")
	// The repo name becomes the package slug and a directory name, so a
	// dot-only name must never get through.
	if strings.Trim(repo, ".") == "" {
		return "", "", fmt.Errorf("cannot parse GitHub repo from URL: %s", repoURL)
	}
	return matches[1], repo, nil
}

// ListReleases fetches all releases for a GitHub repository.
// Returns them sorted by published_at descending (newest first).
func (c *GitHubClient) ListReleases(ctx context.Context, repoURL string) ([]GitHubRelease, error) {
	owner, repo, err := parseRepo(repoURL)
	if err != nil {
		return nil, err
	}

	base := c.apiBase
	if base == "" {
		base = "https://api.github.com"
	}
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=50", base, owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &GitHubAPIError{Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, &GitHubAPIError{
			Status: resp.StatusCode,
			RateLimited: resp.StatusCode == http.StatusTooManyRequests ||
				(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"),
		}
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding releases: %w", err)
	}

	return releases, nil
}

// maxPackageDownloadBytes bounds one release download; see extractZip for
// the bounds on what it unpacks to.
const maxPackageDownloadBytes = 100 << 20

// DownloadAsset downloads a release asset (ZIP file) to disk, refusing one
// larger than maxPackageDownloadBytes. Returns the number of bytes written.
func (c *GitHubClient) DownloadAsset(ctx context.Context, downloadURL, destPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating download request: %w", err)
	}
	if !c.downloadHostAllowed(req.URL) {
		return 0, fmt.Errorf("download address %q is not on GitHub", req.URL.Host)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	client := *c.httpClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if !c.downloadHostAllowed(next.URL) {
			return fmt.Errorf("download redirected off GitHub to %q", next.URL.Host)
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("downloading asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("creating file %s: %w", destPath, err)
	}
	defer out.Close()

	n, err := io.Copy(out, io.LimitReader(resp.Body, maxPackageDownloadBytes+1))
	if err != nil {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("writing file: %w", err)
	}
	if n > maxPackageDownloadBytes {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("the release download is larger than %d MB", maxPackageDownloadBytes>>20)
	}

	return n, nil
}
