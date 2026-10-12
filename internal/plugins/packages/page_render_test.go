package packages

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
)

func renderToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

// TestVersionListKeepsForeignHooks pins the attributes the Foundry plugin's
// banner and fragments depend on: the campaigns trigger id, its hx-get and
// hx-target, and the empty target div. They must stay byte-identical.
func TestVersionListKeepsForeignHooks(t *testing.T) {
	pkg := &Package{ID: "pkg-1", Type: PackageTypeFoundryModule, InstalledVersion: "v0.1.9"}
	versions := []PackageVersion{
		{Version: "v0.1.10", PublishedAt: time.Now()},
		{Version: "v0.1.9", PublishedAt: time.Now()},
	}
	out := renderToString(t, VersionList(pkg, versions, "", "tok"))

	for _, want := range []string{
		`id="fvtt-campaigns-trigger-v0-1-10"`,
		`hx-get="/admin/foundry-vtt/version/v0.1.10/campaigns"`,
		`hx-target="#fvtt-campaigns-pkg-1-v0-1-10"`,
		`<div id="fvtt-campaigns-pkg-1-v0-1-10"></div>`,
		`id="fvtt-campaigns-trigger-v0-1-9"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("VersionList output is missing %q", want)
		}
	}
}

func TestVersionListSystemPackageHasNoForeignHooks(t *testing.T) {
	pkg := &Package{ID: "sys-1", Type: PackageTypeSystem, InstalledVersion: "1.0.0", PinnedVersion: "1.0.0"}
	versions := []PackageVersion{
		{Version: "1.1.0", PublishedAt: time.Now(), ReleaseNotes: "Fixes <b>things</b>"},
		{Version: "1.0.0", PublishedAt: time.Now()},
	}
	out := renderToString(t, VersionList(pkg, versions, "", "tok"))

	if strings.Contains(out, "fvtt-") || strings.Contains(out, "/admin/foundry-vtt/") {
		t.Error("a system package must not render the Foundry hooks")
	}
	if !strings.Contains(out, "Install") || !strings.Contains(out, "Unpin") {
		t.Error("expected Install for the newer version and Unpin on the pinned one")
	}
	if strings.Contains(out, "<b>things</b>") {
		t.Error("release notes must be escaped")
	}
}

func TestVersionListOlderVersionSaysSwitch(t *testing.T) {
	pkg := &Package{ID: "p", Type: PackageTypeSystem, InstalledVersion: "1.1.0"}
	out := renderToString(t, VersionList(pkg, []PackageVersion{
		{Version: "1.1.0", PublishedAt: time.Now()},
		{Version: "1.0.0", PublishedAt: time.Now()},
	}, "", "tok"))
	if !strings.Contains(out, "Switch to this") {
		t.Error("an older version should offer Switch to this")
	}
}

func pageDataForRender(selected string, ptab string) PackagesPageData {
	rows := sampleRows()
	rows[0].InstalledVersion = "0.13.13"
	rows[0].Newer = &PackageVersion{Version: "0.14.0", ReleaseNotes: "- Better builder"}
	rows[0].UsageKnown = true
	rows[3].LastError = "release has no package file"
	data := PackagesPageData{
		Query:     parsePackagesQuery("", "", "", selected, ptab),
		CSRFToken: "tok",
		Now:       time.Now(),
		Rows:      rows,
		Counts:    countFilters(rows),
		Updates:   updateRows(rows),
	}
	data.Visible = filterRows(rows, FilterActive, "")
	for i := range data.Rows {
		if data.Rows[i].ID == selected {
			data.Selected = &data.Rows[i]
		}
	}
	return data
}

func TestInstalledTabWithoutSelection(t *testing.T) {
	out := renderToString(t, installedTab(pageDataForRender("", "")))
	for _, want := range []string{
		"Choose a package on the left to see its details.",
		`href="/admin/packages?pkg=ds"`,
		"Update ready · 0.14.0",
		`href="/admin/packages?f=problems"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(out, "Old School Essentials") {
		t.Error("retired packages must not appear under Active")
	}
}

func TestInstalledTabWithSelectionHidesListOnPhones(t *testing.T) {
	out := renderToString(t, installedTab(pageDataForRender("ds", "")))
	if !strings.Contains(out, "All packages") {
		t.Error("the open panel needs its back link")
	}
	if strings.Contains(out, "Choose a package on the left") {
		t.Error("the placeholder must not render beside an open package")
	}
	if !strings.Contains(out, "0.14.0 is ready.") || !strings.Contains(out, `hx-put="/admin/packages/ds/version"`) {
		t.Error("overview should offer Install for the newer version through the existing endpoint")
	}
}

func TestPanelTabsRender(t *testing.T) {
	tests := []struct {
		ptab  string
		wants []string
	}{
		{PanelTabVersions, []string{"Install or switch to any version"}},
		{PanelTabCampaigns, []string{"is coming", "No campaign uses this package."}},
		{PanelTabSettings, []string{`hx-put="/admin/packages/ds/auto-update"`, `hx-put="/admin/packages/ds/repo"`, `hx-post="/admin/packages/ds/archive"`, `hx-delete="/admin/packages/ds"`}},
	}
	for _, tt := range tests {
		t.Run(tt.ptab, func(t *testing.T) {
			out := renderToString(t, installedTab(pageDataForRender("ds", tt.ptab)))
			for _, w := range tt.wants {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q", w)
				}
			}
		})
	}
}

func TestRemoveDisabledWhileInUse(t *testing.T) {
	data := pageDataForRender("ds", PanelTabSettings)
	data.Selected.Usage = []PackageUsage{{CampaignName: "A"}, {CampaignName: "B"}}
	out := renderToString(t, installedTab(data))
	if !strings.Contains(out, "Blocked while 2 campaigns use it.") {
		t.Error("expected the explanation")
	}
	if strings.Contains(out, `hx-delete="/admin/packages/ds"`) {
		t.Error("Remove must not be wired while campaigns use the package")
	}
}

func TestUpdatesAndReviewEmptyStates(t *testing.T) {
	data := pageDataForRender("", "")
	data.Updates = nil
	if out := renderToString(t, updatesTab(data)); !strings.Contains(out, "Everything is up to date.") {
		t.Error("updates empty state missing")
	}
	if out := renderToString(t, reviewTab(data)); !strings.Contains(out, "Nothing waiting for review.") {
		t.Error("review empty state missing")
	}
	data.Pending = []Package{{ID: "ms", Name: "Mothership", Type: PackageTypeSystem, RepoURL: "https://github.com/j/ms", CreatedAt: time.Now()}}
	out := renderToString(t, reviewTab(data))
	if !strings.Contains(out, `hx-post="/admin/packages/ms/review"`) || !strings.Contains(out, "Decline") {
		t.Error("review row must post to the existing review endpoint")
	}
}

func TestSettingsTabKeepsFormContractAndCleanup(t *testing.T) {
	data := pageDataForRender("", "")
	data.Query.Tab = PackagesTabSettings
	data.Settings = &PackageSecuritySettings{RepoPolicy: RepoPolicyGitHubOnly, OwnerUploadPolicy: OwnerUploadDisabled, MaxFileSize: 25 * 1024 * 1024, ScanContent: true}
	out := renderToString(t, settingsTab(data))
	for _, want := range []string{
		`action="/admin/packages/settings"`,
		`name="_csrf"`,
		`name="repo_policy" value="github_only" checked`,
		`name="owner_upload_policy" value="disabled" checked`,
		`name="require_approval"`,
		`name="validate_manifest"`,
		`name="scan_content" value="true" checked`,
		`name="max_file_size_mb"`,
		`value="25"`,
		`hx-get="/admin/packages/prune"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("settings tab missing %q", want)
		}
	}
}

func TestSettingsTabOldVersionsCard(t *testing.T) {
	data := pageDataForRender("", "")
	data.Query.Tab = PackagesTabSettings
	data.Settings = &PackageSecuritySettings{RepoPolicy: RepoPolicyGitHubOnly, OwnerUploadPolicy: OwnerUploadDisabled, MaxFileSize: 25 * 1024 * 1024}
	data.Retention = RetentionSettings{Mode: RetentionKeepNewest, KeepNewest: 4, UnusedDays: 30}
	out := renderToString(t, settingsTab(data))
	for _, want := range []string{
		"Old versions, site-wide",
		`name="retention_mode" value="manual"`,
		`name="retention_mode" value="keep_newest" checked`,
		`name="retention_keep_newest" value="4"`,
		`name="retention_unused_days" value="30"`,
		"every version of the Foundry module",
		`hx-get="/admin/packages/prune"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("settings tab missing %q", want)
		}
	}
}

func TestPanelOldVersionsGroup(t *testing.T) {
	site := RetentionSettings{Mode: RetentionKeepNewest, KeepNewest: 2, UnusedDays: 30}
	sys := PackageRow{Package: Package{ID: "p1", Name: "X", Type: PackageTypeSystem}}
	out := renderToString(t, panelSettings(sys, site, "tok"))
	for _, want := range []string{`hx-put="/admin/packages/p1/retention"`, `name="mode" value="site" checked`, "Use the site rule", "Its own rule"} {
		if !strings.Contains(out, want) {
			t.Errorf("panel missing %q", want)
		}
	}

	own := sys
	own.RetentionKeepNewest = intp(5)
	out = renderToString(t, panelSettings(own, site, "tok"))
	if !strings.Contains(out, `name="mode" value="own" checked`) || !strings.Contains(out, `name="keep_newest" value="5"`) {
		t.Error("an override must pre-select its own rule with its number")
	}

	fm := PackageRow{Package: Package{ID: "f1", Name: "F", Type: PackageTypeFoundryModule}}
	out = renderToString(t, panelSettings(fm, site, "tok"))
	if strings.Contains(out, "/retention") || !strings.Contains(out, "never removed") {
		t.Error("foundry module must show the never-removed note and no override form")
	}
}

func TestVersionListShowsRuleText(t *testing.T) {
	pkg := &Package{ID: "p", Type: PackageTypeSystem}
	out := renderToString(t, VersionList(pkg, nil, "Old versions follow the site rule: keep the newest 2.", "tok"))
	if !strings.Contains(out, "follow the site rule: keep the newest 2.") {
		t.Error("versions tab must state the actual rule")
	}
}

// TestProblemPanelOffersNewerRelease pins that a package whose last update
// failed offers Install for the newer release beside Try again, which only
// re-checks.
func TestProblemPanelOffersNewerRelease(t *testing.T) {
	data := pageDataForRender("pf", "")
	data.Selected.Newer = &PackageVersion{Version: "0.7.0"}
	out := renderToString(t, installedTab(data))
	for _, want := range []string{"Last update failed.", "Install 0.7.0", `hx-put="/admin/packages/pf/version"`, "Try again"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}

	data.Selected.Newer = nil
	out = renderToString(t, installedTab(data))
	if strings.Contains(out, `hx-put="/admin/packages/pf/version"`) {
		t.Error("with no newer release the failed panel offers only Try again")
	}
}
