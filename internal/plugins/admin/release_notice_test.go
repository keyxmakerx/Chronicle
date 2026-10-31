package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

func relRecord(version string, security bool, notes ...string) *releaseRecord {
	return &releaseRecord{
		CheckedAt: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC),
		Release:   &ChronicleRelease{Version: version, Tag: "v" + version, Notes: notes, Security: security},
	}
}

func TestBuildReleaseNotice(t *testing.T) {
	tests := []struct {
		name    string
		running string
		rec     *releaseRecord
		later   string
		want    string // the version shown, "" for no notice
	}{
		{"newer patch", "1.0.0", relRecord("1.0.1", true), "", "1.0.1"},
		{"tag-style running version", "v1.0.0", relRecord("1.0.1", false), "", "1.0.1"},
		{"numeric, not text, order", "1.0.9", relRecord("1.0.10", false), "", "1.0.10"},
		{"up to date", "1.0.1", relRecord("1.0.1", false), "", ""},
		{"running is newer", "1.1.0", relRecord("1.0.1", false), "", ""},
		{"a prerelease of the release counts as older", "1.0.1-rc1", relRecord("1.0.1", false), "", "1.0.1"},
		{"branch build reports a commit", "3f2a9c1d0b7e", relRecord("1.0.1", false), "", ""},
		{"dirty commit", "3f2a9c1d0b7e-dirty", relRecord("1.0.1", false), "", ""},
		{"unknown build", "unknown", relRecord("1.0.1", false), "", ""},
		{"empty build", "", relRecord("1.0.1", false), "", ""},
		{"never checked", "1.0.0", nil, "", ""},
		{"no release published", "1.0.0", &releaseRecord{CheckedAt: time.Now()}, "", ""},
		{"Later on this version", "1.0.0", relRecord("1.0.1", false), "1.0.1", ""},
		{"Later on an older version", "1.0.0", relRecord("1.0.2", false), "1.0.1", "1.0.2"},
		{"a stored non-release tag is ignored", "1.0.0", &releaseRecord{CheckedAt: time.Now(),
			Release: &ChronicleRelease{Version: "snapshot-2026-11-01", Tag: "snapshot-2026-11-01"}}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := buildReleaseNotice(tt.running, tt.rec, tt.later)
			if tt.want == "" {
				if n != nil {
					t.Fatalf("want no notice, got %+v", n)
				}
				return
			}
			if n == nil || n.Version != tt.want {
				t.Fatalf("got %+v, want version %s", n, tt.want)
			}
			if strings.HasPrefix(n.Running, "v") {
				t.Errorf("running shown as %q; the notice prints bare numbers", n.Running)
			}
			if n.NotesURL != "https://github.com/keyxmakerx/Chronicle/releases/tag/v"+tt.want {
				t.Errorf("notes link = %q", n.NotesURL)
			}
			if n.GuideURL != checkupUpdateGuide {
				t.Errorf("guide link = %q", n.GuideURL)
			}
		})
	}
}

func TestWhatsNewLine(t *testing.T) {
	tests := []struct {
		lines []string
		want  string
	}{
		{nil, ""},
		{[]string{"", "  "}, ""},
		{[]string{"Fixes pages that showed Director notes to players.", "Calendar export keeps moons"},
			"Fixes pages that showed Director notes to players; Calendar export keeps moons."},
	}
	for _, tt := range tests {
		if got := whatsNewLine(tt.lines); got != tt.want {
			t.Errorf("whatsNewLine(%q) = %q, want %q", tt.lines, got, tt.want)
		}
	}
	long := whatsNewLine([]string{strings.Repeat("é", 400)})
	if r := []rune(long); len(r) != whatsNewMax || !strings.HasSuffix(long, "…") {
		t.Errorf("long notes = %d runes, want %d ending in an ellipsis", len(r), whatsNewMax)
	}
}

// fakeReleases counts calls and answers with rel or err.
type fakeReleases struct {
	rel   *ChronicleRelease
	err   error
	calls int
}

func (f *fakeReleases) LatestChronicleRelease(context.Context) (*ChronicleRelease, error) {
	f.calls++
	return f.rel, f.err
}

func newRelService(store ckStore, src *fakeReleases, running string, now *time.Time) *ReleaseNoticeService {
	s := NewReleaseNoticeService(store, src, func() string { return running })
	s.now = func() time.Time { return *now }
	return s
}

func TestReleaseNoticeService_Check(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	store := ckStore{}
	src := &fakeReleases{rel: &ChronicleRelease{Version: "1.0.1", Tag: "v1.0.1", Security: true}}
	s := newRelService(store, src, "1.0.0", &now)

	s.checkIfDue(ctx)
	if src.calls != 1 {
		t.Fatalf("first check: %d calls", src.calls)
	}
	if n := s.Notice(ctx, "u1"); n == nil || n.Version != "1.0.1" || !n.Security {
		t.Fatalf("notice = %+v", n)
	}

	now = now.Add(23 * time.Hour)
	s.checkIfDue(ctx)
	if src.calls != 1 {
		t.Fatal("checked again within a day")
	}

	// A failed check keeps what was known and retries sooner than a day.
	now = now.Add(2 * time.Hour)
	src.err = errors.New("GitHub answered 503")
	s.checkIfDue(ctx)
	if src.calls != 2 {
		t.Fatalf("daily check: %d calls", src.calls)
	}
	if n := s.Notice(ctx, "u1"); n == nil || n.Version != "1.0.1" {
		t.Fatalf("a failed check hid known news: %+v", n)
	}
	now = now.Add(5 * time.Hour)
	s.checkIfDue(ctx)
	if src.calls != 2 {
		t.Fatal("retried before six hours")
	}
	now = now.Add(time.Hour)
	s.checkIfDue(ctx)
	if src.calls != 3 {
		t.Fatal("no retry after six hours")
	}
}

func TestReleaseNoticeService_NeverAsksFromABranchBuild(t *testing.T) {
	now := time.Now()
	src := &fakeReleases{rel: &ChronicleRelease{Version: "1.0.1", Tag: "v1.0.1"}}
	s := newRelService(ckStore{}, src, "3f2a9c1d0b7e", &now)
	s.checkIfDue(context.Background())
	if src.calls != 0 {
		t.Fatal("a build that isn't a release must not call GitHub")
	}
}

func TestReleaseNoticeService_Later(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := ckStore{}
	src := &fakeReleases{rel: &ChronicleRelease{Version: "1.0.1", Tag: "v1.0.1"}}
	s := newRelService(store, src, "1.0.0", &now)
	s.checkIfDue(ctx)

	for _, bad := range []string{"", "latest", "1.0.1\"><script>", "1.0.1-rc1"} {
		if err := s.Later(ctx, "u1", bad); err == nil {
			t.Errorf("Later(%q) was accepted", bad)
		}
	}
	if err := s.Later(ctx, "u1", "1.0.1"); err != nil {
		t.Fatal(err)
	}
	if n := s.Notice(ctx, "u1"); n != nil {
		t.Fatalf("Later didn't hide the notice: %+v", n)
	}
	if n := s.Notice(ctx, "u2"); n == nil {
		t.Fatal("one admin's Later hid the notice from another")
	}

	// A newer release brings it back.
	src.rel = &ChronicleRelease{Version: "1.0.2", Tag: "v1.0.2"}
	now = now.Add(25 * time.Hour)
	s.checkIfDue(ctx)
	if n := s.Notice(ctx, "u1"); n == nil || n.Version != "1.0.2" {
		t.Fatalf("newer release after Later: %+v", n)
	}
}

func TestReleaseNoticeService_OffSwitch(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := ckStore{}
	src := &fakeReleases{rel: &ChronicleRelease{Version: "1.0.1", Tag: "v1.0.1"}}
	s := newRelService(store, src, "1.0.0", &now)
	if !s.CheckOn(ctx) {
		t.Fatal("the check is on until an admin turns it off")
	}
	s.checkIfDue(ctx)

	if err := s.SetCheckOn(ctx, false); err != nil {
		t.Fatal(err)
	}
	if s.Notice(ctx, "u1") != nil {
		t.Fatal("notice shown with the check off")
	}
	now = now.Add(48 * time.Hour)
	s.checkIfDue(ctx)
	if src.calls != 1 {
		t.Fatal("called GitHub with the check off")
	}

	// Turning it back on checks at once rather than showing old news.
	if err := s.SetCheckOn(ctx, true); err != nil {
		t.Fatal(err)
	}
	s.checkIfDue(ctx)
	if src.calls != 2 {
		t.Fatal("turning the check on didn't check")
	}
}

// The off switch also stops the check-up's own comparison with GitHub, and
// hides a result it made before.
func TestCheckup_ChronicleCheckOff(t *testing.T) {
	ctx := context.Background()
	store := ckStore{}
	on := false
	checker := &countingChecker{}
	s := NewCheckupService(CheckupDeps{
		Store:            store,
		Chronicle:        checker,
		ChronicleCheckOn: func(context.Context) bool { return on },
	})
	if _, err := s.run(ctx); err != nil {
		t.Fatal(err)
	}
	if checker.calls != 0 {
		t.Fatal("check-up called GitHub with the check off")
	}
	saveRecord(t, store, &CheckupRecord{At: time.Now(), Chronicle: &VersionCheckResult{Known: true, Behind: 3}})
	if in := s.inputs(ctx, time.Now()); in.Record.Chronicle != nil {
		t.Fatal("an old comparison still shows with the check off")
	}
	on = true
	if in := s.inputs(ctx, time.Now()); in.Record.Chronicle == nil {
		t.Fatal("comparison hidden with the check on")
	}
}

type countingChecker struct{ calls int }

func (c *countingChecker) CheckChronicle(context.Context) VersionCheckResult {
	c.calls++
	return VersionCheckResult{Known: true}
}

var emojiPattern = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)

func TestReleaseNoticeCard_Render(t *testing.T) {
	n := buildReleaseNotice("1.0.0", relRecord("1.0.1", true, "Fixes pages that showed Director notes to players", "<b>bold</b>"), "")
	var buf bytes.Buffer
	if err := releaseNoticeCard(n).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		"Chronicle 1.0.1 is out.", "1.0.0", "It includes security fixes.",
		"What&rsquo;s new: Fixes pages that showed Director notes to players",
		">How to update</a>", ">Release notes</a>", ">Later</button>",
		`hx-post="/admin/release-notice/later"`, "fa-regular fa-bell",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in\n%s", want, html)
		}
	}
	if strings.Contains(html, "<b>bold</b>") {
		t.Error("release notes reached the page unescaped")
	}
	if emojiPattern.MatchString(html) {
		t.Error("the notice has an emoji")
	}
	if strings.Contains(html, ">Update<") {
		t.Error("the notice offers an Update button Chronicle can't honour")
	}
	var vals map[string]string
	m := regexp.MustCompile(`hx-vals="([^"]+)"`).FindStringSubmatch(html)
	if m == nil || json.Unmarshal([]byte(strings.ReplaceAll(m[1], "&#34;", `"`)), &vals) != nil || vals["version"] != "1.0.1" {
		t.Errorf("Later posts %v", m)
	}

	buf.Reset()
	n.Security = false
	_ = releaseNoticeCard(n).Render(context.Background(), &buf)
	if !strings.Contains(buf.String(), "Its notes list no security fixes.") {
		t.Error("no security sentence when the release has none")
	}

	buf.Reset()
	_ = releaseNoticeCard(nil).Render(context.Background(), &buf)
	if strings.TrimSpace(buf.String()) != "" {
		t.Error("a nil notice rendered something")
	}
}
