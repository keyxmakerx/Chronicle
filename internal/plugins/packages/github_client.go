package packages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// repoPattern extracts owner/repo from a GitHub URL.
var repoPattern = regexp.MustCompile(`github\.com[/:]([^/]+)/([^/.]+)`)

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
}

// NewGitHubClient creates a GitHub API client. It reads GITHUB_TOKEN
// from the environment for authenticated requests (5000 req/hour vs 60).
func NewGitHubClient() *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      os.Getenv("GITHUB_TOKEN"),
	}
}

// parseRepo extracts owner and repo from a GitHub URL.
// Supports https://github.com/owner/repo and git@github.com:owner/repo.
func parseRepo(repoURL string) (owner, repo string, err error) {
	matches := repoPattern.FindStringSubmatch(repoURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("cannot parse GitHub repo from URL: %s", repoURL)
	}
	return matches[1], strings.TrimSuffix(matches[2], ".git"), nil
}

// ListReleases fetches all releases for a GitHub repository.
// Returns them sorted by published_at descending (newest first).
func (c *GitHubClient) ListReleases(ctx context.Context, repoURL string) ([]GitHubRelease, error) {
	owner, repo, err := parseRepo(repoURL)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=50", owner, repo)

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
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var releases []GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding releases: %w", err)
	}

	return releases, nil
}

// DownloadAsset downloads a release asset (ZIP file) to disk.
// Returns the number of bytes written.
func (c *GitHubClient) DownloadAsset(ctx context.Context, downloadURL, destPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating download request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
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

	n, err := io.Copy(out, resp.Body)
	if err != nil {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("writing file: %w", err)
	}

	return n, nil
}
