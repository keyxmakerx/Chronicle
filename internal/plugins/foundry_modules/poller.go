// Package foundry_modules — poller.go is the background goroutine that
// auto-fetches Chronicle Foundry module releases from GitHub. Lets the
// admin skip the "manually upload every .zip" step for the common case
// (canonical release pulled from the canonical repo); manual upload
// stays for forks, pre-releases, and emergency testing.
//
// Lifecycle:
//
//   - Constructed in internal/app/routes.go after the foundry_modules
//     service is wired.
//   - Start(ctx) launches a single goroutine that polls every Interval
//     until ctx is cancelled.
//   - PollOnce(ctx) does a single fetch — called by the goroutine on
//     every tick, AND directly by the admin "Fetch from GitHub now"
//     button so operators can pull a release the moment a tag goes up
//     instead of waiting for the next scheduled tick.
//
// Idempotency: relies on the uk_github_release UNIQUE constraint added
// in migration 002. A release that's already been ingested errors out
// of repo.InsertVersion as ErrVersionExists; the poller treats this as
// a skip, not a failure.

package foundry_modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default values used when the matching env vars are empty / unparseable.
const (
	defaultPollInterval = 6 * time.Hour
	defaultGitHubRepo   = "keyxmakerx/Chronicle-Foundry-Module"
	// httpTimeout caps any single GitHub or asset-download request.
	// Releases are typically < 5 MB; 60s leaves room for slow links.
	httpTimeout = 60 * time.Second
	// assetZipName is the .zip asset name the poller looks for. Matches
	// the Foundry repo's release.yml output. If the asset isn't named
	// chronicle-sync.zip the release is skipped with a logged warning.
	assetZipName = "chronicle-sync.zip"
	// assetManifestName is the optional module.json asset some Foundry
	// releases publish alongside the zip. When present the poller
	// reads it directly; when absent the poller extracts module.json
	// from the zip itself.
	assetManifestName = "module.json"
)

// Poller is the background GitHub release fetcher.
type Poller struct {
	repo        Repository
	storageDir  string
	githubRepo  string // "owner/repo"
	githubToken string // optional; empty = unauthenticated requests
	interval    time.Duration
	events      SecurityEventLogger
	httpClient  *http.Client
}

// NewPoller constructs a Poller. Configuration comes from env vars
// (FOUNDRY_MODULE_POLL_INTERVAL, FOUNDRY_MODULE_GITHUB_REPO,
// GITHUB_TOKEN) so deployments can override without code changes; an
// interval of 0 disables the background goroutine entirely (the admin
// "Fetch now" button still works because PollOnce stays callable).
func NewPoller(repo Repository, events SecurityEventLogger, storageDir string) *Poller {
	interval := parseInterval(os.Getenv("FOUNDRY_MODULE_POLL_INTERVAL"))
	githubRepo := strings.TrimSpace(os.Getenv("FOUNDRY_MODULE_GITHUB_REPO"))
	if githubRepo == "" {
		githubRepo = defaultGitHubRepo
	}
	return &Poller{
		repo:        repo,
		storageDir:  storageDir,
		githubRepo:  githubRepo,
		githubToken: os.Getenv("GITHUB_TOKEN"),
		interval:    interval,
		events:      events,
		httpClient:  &http.Client{Timeout: httpTimeout},
	}
}

// Start launches the polling goroutine. Returns immediately. The
// goroutine runs until ctx is cancelled. A zero interval disables
// the goroutine; ad-hoc PollOnce calls still work.
func (p *Poller) Start(ctx context.Context) {
	if p.interval <= 0 {
		return
	}
	go p.run(ctx)
}

// PollOnce performs a single poll cycle, returning the number of new
// versions ingested and a list of human-readable error messages.
// Never returns a Go error — partial failures are reported via the
// errors slice so the admin "Fetch now" handler can render them
// alongside whatever did succeed.
func (p *Poller) PollOnce(ctx context.Context) (int, []string) {
	releases, err := p.listReleases(ctx)
	if err != nil {
		msg := fmt.Sprintf("listing releases failed: %v", err)
		p.logPollResult(ctx, 0, []string{msg})
		return 0, []string{msg}
	}
	newCount := 0
	var errs []string
	for _, rel := range releases {
		switch err := p.ingestRelease(ctx, rel); {
		case err == nil:
			newCount++
		case errors.Is(err, ErrVersionExists):
			// Already ingested. Idempotent skip, not an error.
		case errors.Is(err, errSkipRelease):
			// The release intentionally doesn't match our shape
			// (no chronicle-sync.zip asset, etc.). Log but don't
			// count as an error — these aren't operator-actionable.
		default:
			errs = append(errs, fmt.Sprintf("release %s: %v", rel.TagName, err))
		}
	}
	p.logPollResult(ctx, newCount, errs)
	return newCount, errs
}

func (p *Poller) run(ctx context.Context) {
	// Tick immediately on start so a fresh deploy populates the
	// catalog without waiting for the first interval to elapse.
	_, _ = p.PollOnce(ctx)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = p.PollOnce(ctx)
		}
	}
}

// errSkipRelease signals "this release doesn't match our shape" — for
// example, it's a non-Chronicle Foundry module release on the same
// repo, or the zip asset is named something we don't recognize. The
// poll counts this as a skip rather than a failure.
var errSkipRelease = errors.New("release skipped")

// gitHubRelease mirrors the subset of GitHub's release API the poller
// reads. We deliberately ignore most fields — there's no reason to
// drag the full schema into the binary.
type gitHubRelease struct {
	ID          int64         `json:"id"`
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// listReleases hits GitHub's per-repo releases endpoint. Includes
// pre-releases by default — the operator's 0.1.x line is published
// as prerelease and they want it in the catalog.
func (p *Poller) listReleases(ctx context.Context) ([]gitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", p.githubRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if p.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.githubToken)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var rels []gitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, err
	}
	return rels, nil
}

// ingestRelease downloads the zip + manifest for one release and
// inserts a catalog row. Returns ErrVersionExists if the release was
// already ingested (idempotent skip), errSkipRelease if the release
// doesn't match the shape (no zip asset, malformed manifest, etc.).
func (p *Poller) ingestRelease(ctx context.Context, rel gitHubRelease) error {
	// Find the zip asset. Skip silently if missing — non-fatal because
	// some releases on the repo could be doc-only / non-Foundry.
	zipAsset := findAsset(rel.Assets, assetZipName)
	if zipAsset == nil {
		return errSkipRelease
	}

	// Pull the zip.
	zipBytes, err := p.downloadAsset(ctx, zipAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", zipAsset.Name, err)
	}
	if int64(len(zipBytes)) != zipAsset.Size && zipAsset.Size > 0 {
		return fmt.Errorf("download %s: size mismatch (got %d, expected %d)", zipAsset.Name, len(zipBytes), zipAsset.Size)
	}

	// Manifest: prefer the standalone module.json asset (faster + more
	// reliable than zip extraction); fall back to extracting it from
	// the zip itself if the asset isn't published alongside.
	var manifestBytes []byte
	if manifestAsset := findAsset(rel.Assets, assetManifestName); manifestAsset != nil {
		manifestBytes, err = p.downloadAsset(ctx, manifestAsset.BrowserDownloadURL)
		if err != nil {
			return fmt.Errorf("download %s: %w", manifestAsset.Name, err)
		}
	}

	// Parse manifest. extractManifest accepts the zip and finds the
	// embedded module.json; if we already downloaded the standalone
	// manifest we parse it directly.
	var (
		parsed   *ParsedManifest
		manifest json.RawMessage
	)
	if len(manifestBytes) > 0 {
		var pm ParsedManifest
		if err := json.Unmarshal(manifestBytes, &pm); err != nil {
			return fmt.Errorf("parse module.json asset: %w", err)
		}
		if pm.ID == "" || pm.Version == "" {
			return errSkipRelease
		}
		parsed = &pm
		manifest = manifestBytes
	} else {
		p2, mBytes, err := extractManifest(zipBytes)
		if err != nil {
			return errSkipRelease
		}
		parsed = p2
		manifest = mBytes
	}

	// Idempotency pre-check: hit the catalog by github_release_id
	// before writing anything to disk. The UNIQUE constraint on
	// uk_github_release would catch a duplicate anyway, but
	// pre-checking avoids needless disk writes.
	if existing, _ := p.repo.GetVersionByGitHubReleaseID(ctx, rel.ID); existing != nil {
		return ErrVersionExists
	}
	// Also pre-check by version string — if the operator manually
	// uploaded the same version a github release covers, treat as
	// a skip rather than a conflict.
	if existing, _ := p.repo.GetVersion(ctx, parsed.Version); existing != nil {
		return ErrVersionExists
	}

	// Storage path. Same convention as manual upload — sanitized
	// version string under storageDir.
	sanitized := sanitizeFilename(parsed.Version)
	if sanitized == "" {
		return errSkipRelease
	}
	if err := os.MkdirAll(p.storageDir, 0o755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}
	path := filepath.Join(p.storageDir, sanitized+".zip")
	if err := os.WriteFile(path, zipBytes, 0o644); err != nil {
		return fmt.Errorf("write zip: %w", err)
	}

	sum := sha256.Sum256(zipBytes)
	releaseID := rel.ID
	row := &ModuleVersion{
		ID:            generateID(),
		Version:       parsed.Version,
		FilePath:      path,
		FileSize:      int64(len(zipBytes)),
		ContentSHA256: hex.EncodeToString(sum[:]),
		ManifestJSON:  manifest,
		Status:        StatusAvailable,
		// Release notes prefer the GitHub release body (operator-
		// authored summary) over the manifest description (typically
		// boilerplate). Strip windows line endings so the templ render
		// stays clean.
		ReleaseNotes:     firstNonEmpty(strings.ReplaceAll(rel.Body, "\r\n", "\n"), parsed.Description),
		UploadedAt:       rel.PublishedAt.UTC(),
		Source:           SourceGitHubRelease,
		GitHubReleaseTag: rel.TagName,
		GitHubReleaseID:  &releaseID,
	}
	if parsed.Compatibility != nil {
		row.CompatibilityMinimum = parsed.Compatibility.Minimum
		row.CompatibilityVerified = parsed.Compatibility.Verified
		row.CompatibilityMaximum = parsed.Compatibility.Maximum
	}

	if err := p.repo.InsertVersion(ctx, row); err != nil {
		// Best-effort cleanup of the on-disk file so a failed insert
		// doesn't leave orphaned bytes. Mirrors UploadVersion's
		// cleanup path.
		_ = os.Remove(path)
		return err
	}
	return nil
}

// downloadAsset fetches a release asset, returning its bytes. Asset URLs
// are 302-redirected by GitHub; the http client follows the redirect by
// default so this is a simple GET.
func (p *Poller) downloadAsset(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if p.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.githubToken)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset %s: %s", url, resp.Status)
	}
	// 50 MB hard cap. Module zips are typically < 5 MB; the cap is
	// belt-and-suspenders against a misconfigured release that
	// publishes a huge asset.
	return io.ReadAll(io.LimitReader(resp.Body, 50<<20))
}

func findAsset(assets []releaseAsset, name string) *releaseAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// parseInterval turns the FOUNDRY_MODULE_POLL_INTERVAL env value into
// a duration. Empty / unparseable values get the default (6h). The
// literal string "0" disables the poller.
func parseInterval(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultPollInterval
	}
	if raw == "0" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultPollInterval
	}
	return d
}

// logPollResult writes a security_events row summarizing the poll.
// One event per cycle, not per release, so the dashboard doesn't get
// flooded on a fresh deploy where the poller ingests many releases.
func (p *Poller) logPollResult(ctx context.Context, newCount int, errs []string) {
	if p.events == nil {
		return
	}
	details := map[string]any{
		"new_versions": newCount,
		"repo":         p.githubRepo,
	}
	if len(errs) > 0 {
		details["errors"] = errs
	}
	_ = p.events.LogEvent(ctx,
		EventFoundryModuleGitHubPoll,
		"", "", "", "", details,
	)
}

