package packages

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizePackagesTab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", PackagesTabInstalled},
		{"installed", PackagesTabInstalled},
		{"updates", PackagesTabUpdates},
		{"review", PackagesTabReview},
		{"settings", PackagesTabSettings},
		{"nonsense", PackagesTabInstalled},
		{"UPDATES", PackagesTabInstalled},
	}
	for _, tt := range tests {
		if got := normalizePackagesTab(tt.in); got != tt.want {
			t.Errorf("normalizePackagesTab(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizePanelTabAndFilter(t *testing.T) {
	panel := []struct{ in, want string }{
		{"", PanelTabOverview},
		{"versions", PanelTabVersions},
		{"campaigns", PanelTabCampaigns},
		{"settings", PanelTabSettings},
		{"x", PanelTabOverview},
	}
	for _, tt := range panel {
		if got := normalizePanelTab(tt.in); got != tt.want {
			t.Errorf("normalizePanelTab(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	filter := []struct{ in, want string }{
		{"", FilterActive},
		{"systems", FilterSystems},
		{"foundry", FilterFoundry},
		{"problems", FilterProblems},
		{"retired", FilterRetired},
		{"all", FilterActive},
	}
	for _, tt := range filter {
		if got := normalizeFilter(tt.in); got != tt.want {
			t.Errorf("normalizeFilter(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParsePackagesQueryBoundsSearch(t *testing.T) {
	q := parsePackagesQuery("bad", "bad", "  "+strings.Repeat("a", 500)+"  ", " id ", "bad")
	if q.Tab != PackagesTabInstalled || q.Filter != FilterActive || q.PanelTab != PanelTabOverview {
		t.Errorf("unknown values must fall back to defaults, got %+v", q)
	}
	if len(q.Search) != maxSearchLen {
		t.Errorf("search length = %d, want %d", len(q.Search), maxSearchLen)
	}
	if q.PkgID != "id" {
		t.Errorf("PkgID = %q, want trimmed id", q.PkgID)
	}
}

func TestPkgHref(t *testing.T) {
	tests := []struct {
		name                           string
		tab, filter, search, pkg, ptab string
		want                           string
	}{
		{"bare page", PackagesTabInstalled, FilterActive, "", "", "", "/admin/packages"},
		{"tab only", PackagesTabUpdates, FilterActive, "", "", "", "/admin/packages?tab=updates"},
		{"filter", PackagesTabInstalled, FilterProblems, "", "", "", "/admin/packages?f=problems"},
		{"search is escaped", PackagesTabInstalled, FilterActive, "draw steel", "", "", "/admin/packages?q=draw+steel"},
		{"package overview omits ptab", PackagesTabInstalled, FilterActive, "", "p1", PanelTabOverview, "/admin/packages?pkg=p1"},
		{"package versions", PackagesTabInstalled, FilterActive, "", "p1", PanelTabVersions, "/admin/packages?pkg=p1&ptab=versions"},
		{"ptab without package is dropped", PackagesTabInstalled, FilterActive, "", "", PanelTabVersions, "/admin/packages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pkgHref(tt.tab, tt.filter, tt.search, tt.pkg, tt.ptab); got != tt.want {
				t.Errorf("pkgHref = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueryLinksKeepListState(t *testing.T) {
	q := packagesQuery{Tab: PackagesTabInstalled, Filter: FilterSystems, Search: "ds", PkgID: "p1", PanelTab: PanelTabVersions}
	if got, want := q.rowHref("p2"), "/admin/packages?f=systems&pkg=p2&q=ds"; got != want {
		t.Errorf("rowHref = %q, want %q", got, want)
	}
	if got, want := q.panelHref(PanelTabSettings), "/admin/packages?f=systems&pkg=p1&ptab=settings&q=ds"; got != want {
		t.Errorf("panelHref = %q, want %q", got, want)
	}
	if got, want := q.closeHref(), "/admin/packages?f=systems&q=ds"; got != want {
		t.Errorf("closeHref = %q, want %q", got, want)
	}
	if got, want := q.filterHref(FilterRetired), "/admin/packages?f=retired&q=ds"; got != want {
		t.Errorf("filterHref = %q, want %q", got, want)
	}
}

func ver(v string, pre bool) PackageVersion { return PackageVersion{Version: v, Prerelease: pre} }

func TestNewerVersion(t *testing.T) {
	tests := []struct {
		name     string
		pkg      Package
		versions []PackageVersion
		want     string // "" = nil
	}{
		{"newer stable", Package{InstalledVersion: "0.13.13"}, []PackageVersion{ver("0.14.0", false), ver("0.13.13", false)}, "0.14.0"},
		{"numeric compare, not lexical", Package{InstalledVersion: "0.9.0"}, []PackageVersion{ver("0.10.0", false), ver("0.9.0", false)}, "0.10.0"},
		{"already latest", Package{InstalledVersion: "1.0.0"}, []PackageVersion{ver("1.0.0", false)}, ""},
		{"installed is ahead of the newest listed", Package{InstalledVersion: "2.0.0"}, []PackageVersion{ver("1.0.0", false)}, ""},
		{"never installed", Package{}, []PackageVersion{ver("1.0.0", false)}, ""},
		{"no versions", Package{InstalledVersion: "1.0.0"}, nil, ""},
		{"prerelease skipped when a stable exists", Package{InstalledVersion: "1.0.0"}, []PackageVersion{ver("2.0.0-rc1", true), ver("1.0.0", false)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newerVersion(tt.pkg, tt.versions)
			switch {
			case tt.want == "" && got != nil:
				t.Errorf("got %q, want nil", got.Version)
			case tt.want != "" && (got == nil || got.Version != tt.want):
				t.Errorf("got %v, want %q", got, tt.want)
			}
		})
	}
}

func TestDerivePackageStatusAndLabel(t *testing.T) {
	newer := &PackageVersion{Version: "2.0.0"}
	tests := []struct {
		name      string
		pkg       Package
		newer     *PackageVersion
		want      rowStatus
		wantLabel string
	}{
		{"up to date", Package{Status: StatusApproved, InstalledVersion: "1.0.0"}, nil, statusUpToDate, "Up to date"},
		{"update ready", Package{Status: StatusApproved, InstalledVersion: "1.0.0"}, newer, statusUpdateReady, "Update ready · 2.0.0"},
		{"problem beats update", Package{Status: StatusApproved, InstalledVersion: "1.0.0", LastError: "boom"}, newer, statusProblem, "Problem"},
		{"pinned hides update", Package{Status: StatusApproved, InstalledVersion: "1.0.0", PinnedVersion: "1.0.0"}, newer, statusPinned, "Pinned · 1.0.0"},
		{"not installed", Package{Status: StatusApproved}, nil, statusNotInstalled, "Not installed"},
		{"archived is retired", Package{Status: StatusArchived, InstalledVersion: "1.0.0", LastError: "old"}, nil, statusRetired, "Retired"},
		{"deprecated is retired but says so", Package{Status: StatusDeprecated, InstalledVersion: "1.0.0"}, nil, statusRetired, "Deprecated"},
		{"rejected is declined", Package{Status: StatusRejected}, nil, statusDeclined, "Declined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derivePackageStatus(tt.pkg, tt.newer)
			if got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
			row := PackageRow{Package: tt.pkg, Status: got}
			if got == statusUpdateReady {
				row.Newer = tt.newer
			}
			if l := statusLabel(row); l != tt.wantLabel {
				t.Errorf("label = %q, want %q", l, tt.wantLabel)
			}
		})
	}
}

func TestUpdatePolicyLabel(t *testing.T) {
	tests := []struct {
		in   UpdatePolicy
		want string
	}{
		{UpdateOff, "Off"},
		{UpdateNightly, "Once a day"},
		{UpdateWeekly, "Once a week"},
		{UpdateOnRelease, "As soon as it is released"},
		{UpdatePolicy("surprise"), "Off"},
		{UpdatePolicy(""), "Off"},
	}
	for _, tt := range tests {
		if got := updatePolicyLabel(tt.in); got != tt.want {
			t.Errorf("updatePolicyLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func sampleRows() []PackageRow {
	mk := func(id, name string, typ PackageType, st PackageStatus, status rowStatus) PackageRow {
		return PackageRow{
			Package: Package{ID: id, Name: name, Type: typ, Status: st, RepoURL: "https://github.com/o/" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))},
			Status:  status,
		}
	}
	return []PackageRow{
		mk("ds", "Draw Steel", PackageTypeSystem, StatusApproved, statusUpdateReady),
		mk("dnd", "D&D 5.5e", PackageTypeSystem, StatusApproved, statusUpToDate),
		mk("fm", "Chronicle Sync", PackageTypeFoundryModule, StatusApproved, statusUpdateReady),
		mk("pf", "Pathfinder 2e", PackageTypeSystem, StatusApproved, statusProblem),
		mk("ose", "Old School Essentials", PackageTypeSystem, StatusArchived, statusRetired),
		mk("no", "Declined Thing", PackageTypeSystem, StatusRejected, statusDeclined),
	}
}

func ids(rows []PackageRow) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func TestFilterRowsAndCounts(t *testing.T) {
	rows := sampleRows()
	tests := []struct {
		name           string
		filter, search string
		want           []string
	}{
		{"active hides retired and declined", FilterActive, "", []string{"ds", "dnd", "fm", "pf"}},
		{"systems", FilterSystems, "", []string{"ds", "dnd", "pf"}},
		{"foundry", FilterFoundry, "", []string{"fm"}},
		{"problems", FilterProblems, "", []string{"pf"}},
		{"retired includes declined", FilterRetired, "", []string{"ose", "no"}},
		{"search by name, case-insensitive", FilterActive, "STEEL", []string{"ds"}},
		{"search by repo", FilterActive, "o/pathfinder", []string{"pf"}},
		{"search with no match", FilterActive, "zzz", []string{}},
		{"search within retired", FilterRetired, "old school", []string{"ose"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(filterRows(rows, tt.filter, tt.search)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	want := filterCounts{Active: 4, Systems: 3, Foundry: 1, Problems: 1, Retired: 2}
	if got := countFilters(rows); got != want {
		t.Errorf("countFilters = %+v, want %+v", got, want)
	}
}

func TestUpdateRowsSortedByName(t *testing.T) {
	got := ids(updateRows(sampleRows()))
	if want := []string{"fm", "ds"}; !reflect.DeepEqual(got, want) {
		t.Errorf("updateRows = %v, want %v", got, want)
	}
}

func TestAgoLabel(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	tests := []struct {
		name string
		in   *time.Time
		want string
	}{
		{"nil", nil, "never"},
		{"seconds", at(10 * time.Second), "just now"},
		{"one minute", at(time.Minute), "1 minute ago"},
		{"minutes", at(12 * time.Minute), "12 minutes ago"},
		{"hours", at(5 * time.Hour), "5 hours ago"},
		{"days", at(72 * time.Hour), "3 days ago"},
		{"old becomes a date", at(90 * 24 * time.Hour), "5 Jul 2026"},
	}
	for _, tt := range tests {
		if got := agoLabel(now, tt.in); got != tt.want {
			t.Errorf("%s: agoLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestReleaseNoteLines(t *testing.T) {
	notes := "## What's new\r\n\n- Monster builder shows sources\n* New ancestries\n\nPlain line\n- fourth"
	tests := []struct {
		n    int
		want []string
	}{
		{3, []string{"Monster builder shows sources", "New ancestries", "Plain line"}},
		{1, []string{"Monster builder shows sources"}},
		{10, []string{"Monster builder shows sources", "New ancestries", "Plain line", "fourth"}},
	}
	for _, tt := range tests {
		if got := releaseNoteLines(notes, tt.n); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("n=%d: got %q, want %q", tt.n, got, tt.want)
		}
	}
	if got := releaseNoteLines("", 3); len(got) != 0 {
		t.Errorf("empty notes: got %q", got)
	}

	// GitHub's generated notes, as the Foundry module's releases carry them.
	generated := "## What's Changed\n* Copy the Draw Steel negotiation tracker onto Foundry NPC sheets by @keyxmakerx in https://github.com/o/r/pull/159\n" +
		"* Keep dev-only folders out of the release zip by @keyxmakerx in https://github.com/o/r/pull/160\n\n\n" +
		"**Full Changelog**: https://github.com/o/r/compare/2.0.1...2.0.2"
	want := []string{"Copy the Draw Steel negotiation tracker onto Foundry NPC sheets", "Keep dev-only folders out of the release zip"}
	if got := releaseNoteLines(generated, 3); !reflect.DeepEqual(got, want) {
		t.Errorf("generated notes: got %q, want %q", got, want)
	}
}

func TestPackagesReturnURL(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"no header", "", "/admin/packages"},
		{"keeps panel", "https://x.example/admin/packages?pkg=p1&ptab=versions", "/admin/packages?pkg=p1&ptab=versions"},
		{"keeps tab", "https://x.example/admin/packages?tab=settings", "/admin/packages?tab=settings"},
		{"drops unknown params", "https://x.example/admin/packages?evil=1&tab=updates", "/admin/packages?tab=updates"},
		{"normalises junk values", "https://x.example/admin/packages?tab=zzz&ptab=zzz&f=zzz", "/admin/packages"},
		{"other page falls back", "https://x.example/admin/users?pkg=p1", "/admin/packages"},
		{"unparseable falls back", "http://[::1", "/admin/packages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packagesReturnURL(tt.in); got != tt.want {
				t.Errorf("packagesReturnURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJSONAttrEscapes(t *testing.T) {
	if got := versionVals(`1.0"}`); got != `{"version":"1.0\"}"}` {
		t.Errorf("versionVals did not escape: %s", got)
	}
	if got := csrfHeaders("tok"); got != `{"X-CSRF-Token":"tok"}` {
		t.Errorf("csrfHeaders = %s", got)
	}
}

// fakePageSource feeds buildPackagesPage.
type fakePageSource struct {
	pkgs       []Package
	versions   map[string][]PackageVersion
	usage      map[string][]PackageUsage
	usageErr   map[string]error
	pending    []Package
	settings   *PackageSecuritySettings
	settingsOK bool
}

func (f *fakePageSource) ListPackages(context.Context) ([]Package, error) { return f.pkgs, nil }
func (f *fakePageSource) ListVersions(_ context.Context, id string) ([]PackageVersion, error) {
	return f.versions[id], nil
}
func (f *fakePageSource) GetUsage(_ context.Context, id string) ([]PackageUsage, error) {
	return f.usage[id], f.usageErr[id]
}
func (f *fakePageSource) ListPendingSubmissions(context.Context) ([]Package, error) {
	return f.pending, nil
}
func (f *fakePageSource) GetSecuritySettings(context.Context) (*PackageSecuritySettings, error) {
	f.settingsOK = true
	return f.settings, nil
}

func (f *fakePageSource) GetRetentionSettings(context.Context) (*RetentionSettings, error) {
	r := DefaultRetentionSettings()
	return &r, nil
}

func TestBuildPackagesPage(t *testing.T) {
	src := &fakePageSource{
		pkgs: []Package{
			{ID: "ds", Name: "Draw Steel", Status: StatusApproved, InstalledVersion: "0.13.13"},
			{ID: "pf", Name: "Pathfinder", Status: StatusApproved, InstalledVersion: "0.6.1", LastError: "no asset"},
			{ID: "ms", Name: "Mothership", Status: StatusPending},
		},
		versions: map[string][]PackageVersion{
			"ds": {ver("0.14.0", false), ver("0.13.13", false)},
		},
		usage:    map[string][]PackageUsage{"ds": {{CampaignName: "Shattered Coast"}}},
		usageErr: map[string]error{"pf": errors.New("db down")},
		pending:  []Package{{ID: "ms", Name: "Mothership", Status: StatusPending}},
		settings: &PackageSecuritySettings{RepoPolicy: RepoPolicyGitHubOnly},
	}
	q := parsePackagesQuery("", "", "", "ds", "")
	data, err := buildPackagesPage(context.Background(), src, nil, nil, q, "tok", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if got, want := ids(data.Rows), []string{"ds", "pf"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v (pending submissions belong to Review)", got, want)
	}
	if data.PendingCount != 1 {
		t.Errorf("PendingCount = %d, want 1", data.PendingCount)
	}
	if got, want := ids(data.Updates), []string{"ds"}; !reflect.DeepEqual(got, want) {
		t.Errorf("updates = %v, want %v", got, want)
	}
	if data.Counts.Problems != 1 {
		t.Errorf("Problems = %d, want 1", data.Counts.Problems)
	}
	if data.Selected == nil || data.Selected.ID != "ds" || data.Selected.Newer == nil || data.Selected.Newer.Version != "0.14.0" {
		t.Errorf("selected row wrong: %+v", data.Selected)
	}
	if !data.Rows[0].UsageKnown || len(data.Rows[0].Usage) != 1 {
		t.Errorf("ds usage not loaded: %+v", data.Rows[0])
	}
	if data.Rows[1].UsageKnown {
		t.Error("a failed usage lookup must read as unknown, not as zero campaigns")
	}
	if src.settingsOK || data.Settings != nil {
		t.Error("settings must only be loaded for the Settings tab")
	}

	q = parsePackagesQuery("settings", "", "", "gone", "")
	data, err = buildPackagesPage(context.Background(), src, nil, nil, q, "tok", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if data.Settings == nil {
		t.Error("Settings tab must load the settings")
	}
	if data.Selected != nil {
		t.Error("an unknown package id must open nothing")
	}
}
