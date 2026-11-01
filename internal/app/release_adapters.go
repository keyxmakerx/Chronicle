package app

// release_adapters.go answers the admin new-version notice's question,
// "what is the newest published Chronicle release?", from GitHub, so the
// admin package needs neither an HTTP client nor the packages plugin.

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

	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/semver"
)

// releaseNoteLineCount is how many lines of the notes the notice quotes.
const releaseNoteLineCount = 2

// chronicleReleaseSource lists the repository's releases. GITHUB_TOKEN raises
// the rate limit when set.
type chronicleReleaseSource struct {
	client  *http.Client
	token   string
	apiBase string
}

func newChronicleReleaseSource() *chronicleReleaseSource {
	return &chronicleReleaseSource{
		client:  &http.Client{Timeout: 15 * time.Second},
		token:   os.Getenv("GITHUB_TOKEN"),
		apiBase: "https://api.github.com",
	}
}

// githubRelease is the part of GitHub's release object the notice reads.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (s *chronicleReleaseSource) LatestChronicleRelease(ctx context.Context) (*admin.ChronicleRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=30", s.apiBase, chronicleRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %d", resp.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("reading GitHub's answer: %w", err)
	}
	return newestStableRelease(releases), nil
}

// newestStableRelease picks the highest plain release number, by the same
// ordering the packages plugin uses. Drafts, prereleases and tags that aren't
// a release number (the weekly snapshot-* tags) are skipped; nil when none
// is left.
func newestStableRelease(releases []githubRelease) *admin.ChronicleRelease {
	var best *githubRelease
	for i := range releases {
		r := &releases[i]
		if r.Draft || r.Prerelease || !admin.IsStableVersion(r.TagName) {
			continue
		}
		if best == nil || semver.Less(best.TagName, r.TagName) {
			best = r
		}
	}
	if best == nil {
		return nil
	}
	return &admin.ChronicleRelease{
		Version:  strings.TrimPrefix(best.TagName, "v"),
		Tag:      best.TagName,
		Notes:    packages.ReleaseNoteLines(best.Body, releaseNoteLineCount),
		Security: hasSecurityHeading(best.Body),
	}
}

// securityHeading is a Markdown heading that names security fixes, such as
// "## Security" or "### Security fixes". Release notes mark security fixes
// this way; the notice says "It includes security fixes" only when one is
// there.
var securityHeading = regexp.MustCompile(`(?im)^#{1,6}\s+security\b`)

func hasSecurityHeading(body string) bool { return securityHeading.MatchString(body) }
