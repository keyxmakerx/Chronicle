package admin

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/semver"
)

// The new-version notice: one line on the admin Home page telling site
// admins that a newer Chronicle release is out. Chronicle can't replace its
// own container, so the notice links to the upgrade steps instead of offering
// an Update button. It only speaks when both sides of the comparison are real
// release numbers; a branch build reports a commit, and comparing a commit to
// a release number would be a guess.

// ChronicleRelease is the newest published (non-draft, non-prerelease)
// Chronicle release, as the daily check found it.
type ChronicleRelease struct {
	// Version is the release number without a leading "v", e.g. "1.0.1".
	Version string `json:"version"`
	// Tag is the git tag as published, e.g. "v1.0.1"; it builds the notes link.
	Tag string `json:"tag"`
	// Notes are the first lines of the release notes.
	Notes []string `json:"notes,omitempty"`
	// Security is true when the notes have a heading naming security fixes.
	Security bool `json:"security"`
}

// ChronicleReleaseSource looks up the newest published Chronicle release.
// It answers nil, nil when none is published. Wired from internal/app.
type ChronicleReleaseSource interface {
	LatestChronicleRelease(ctx context.Context) (*ChronicleRelease, error)
}

// releaseRecord is the stored outcome of the last check. A failed check keeps
// the release the previous check found, so a GitHub outage neither shows an
// error nor hides news already known.
type releaseRecord struct {
	CheckedAt time.Time         `json:"checkedAt"`
	Failed    bool              `json:"failed,omitempty"`
	Release   *ChronicleRelease `json:"release,omitempty"`
}

// ReleaseNotice is the notice as the dashboard renders it.
type ReleaseNotice struct {
	Version  string // the newer release, e.g. "1.0.1"
	Running  string // what this server runs, e.g. "1.0.0"
	Security bool
	WhatsNew string // "" when the notes had nothing to quote
	NotesURL string
	GuideURL string
}

// releaseVersionPattern is a release number, optionally with a pre-release
// suffix: what CHRONICLE_VERSION holds on a server built from a version tag.
var releaseVersionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// stableVersionPattern is a plain release number; only these are announced.
var stableVersionPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// IsReleaseVersion reports whether v names a release rather than a commit,
// "unknown" or nothing. Only a release server is told about new releases.
func IsReleaseVersion(v string) bool { return releaseVersionPattern.MatchString(v) }

// IsStableVersion reports whether v is a plain release number such as
// "1.0.1" or "v1.0.1", with no pre-release suffix.
func IsStableVersion(v string) bool { return stableVersionPattern.MatchString(v) }

// chronicleReleasesURL is where the notes of each release are published.
const chronicleReleasesURL = "https://github.com/keyxmakerx/Chronicle/releases/tag/"

// buildReleaseNotice decides whether the notice shows and with what. later
// is the version this admin pressed Later on; it hides that version and any
// older one, never a newer one.
func buildReleaseNotice(running string, rec *releaseRecord, later string) *ReleaseNotice {
	if !IsReleaseVersion(running) || rec == nil || rec.Release == nil {
		return nil
	}
	rel := rec.Release
	if !IsStableVersion(rel.Version) || !IsStableVersion(rel.Tag) {
		return nil
	}
	if semver.Compare(rel.Version, running) <= 0 {
		return nil
	}
	if later != "" && !semver.Less(later, rel.Version) {
		return nil
	}
	return &ReleaseNotice{
		Version:  strings.TrimPrefix(rel.Version, "v"),
		Running:  strings.TrimPrefix(running, "v"),
		Security: rel.Security,
		WhatsNew: whatsNewLine(rel.Notes),
		NotesURL: chronicleReleasesURL + url.PathEscape(rel.Tag),
		GuideURL: checkupUpdateGuide,
	}
}

// whatsNewMax bounds the quoted notes so a long first line stays one line
// on a phone-width dashboard; the full notes are one link away.
const whatsNewMax = 200

// whatsNewLine joins the first lines of the notes into one sentence.
func whatsNewLine(lines []string) string {
	var parts []string
	for _, l := range lines {
		l = strings.TrimRight(strings.TrimSpace(l), ".;")
		if l != "" {
			parts = append(parts, l)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, "; ")
	if r := []rune(s); len(r) > whatsNewMax {
		return strings.TrimSpace(string(r[:whatsNewMax-1])) + "…"
	}
	return s + "."
}
