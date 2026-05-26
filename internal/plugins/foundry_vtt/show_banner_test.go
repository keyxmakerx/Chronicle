// show_banner_test.go — C-UPDATER-NOTIFICATION-LINK regression test
// (cordinator Issue #16). Pins the banner's "Update pin" settings
// link against the canonical set of campaign-settings tab values.
//
// Previously lived at internal/plugins/campaigns/foundry_banner_link_test.go
// alongside the foundryModuleUpdateBanner templ; followed the templ to
// its new home in foundry_vtt per NW-2.2 Chunk D. Same regression
// coverage, new file location.

package foundry_vtt

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// validSettingsTabs mirrors the five `tab = '<value>'` buttons in
// campaigns/settings.templ (general / features / people / integrations /
// activity). If a tab is renamed or added there, update this set; the
// renaming PR's reviewer should also re-confirm every ?tab=<x> URL
// repo-wide via grep.
var validSettingsTabs = map[string]bool{
	"general":      true,
	"features":     true,
	"people":       true,
	"integrations": true,
	"activity":     true,
}

// TestCampaignShowFoundryBanner_LinksToValidSettingsTab pins the
// banner's "Update pin" link against validSettingsTabs. The original
// bug (cordinator Issue #16) was that the link pointed at ?tab=foundry
// — a tab that does not exist after the C-FMC-5b/5c rename. Clicking
// it loaded settings with Alpine's tab initialized to a no-match value,
// hiding every tab section → blank page.
//
// If the banner regresses to a non-existent tab, this test fails with
// a clear message naming both sites.
func TestCampaignShowFoundryBanner_LinksToValidSettingsTab(t *testing.T) {
	component := CampaignShowFoundryBanner("test-campaign-id", BannerStatus{
		HasUpdate:      true,
		LatestVersion:  "0.2.0",
		CurrentVersion: "0.1.0",
	})
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render banner: %v", err)
	}
	html := buf.String()

	// Locate the settings link. The banner has exactly one
	// /campaigns/<id>/settings?tab=... href; extract its tab value.
	const prefix = "/campaigns/test-campaign-id/settings?tab="
	idx := strings.Index(html, prefix)
	if idx < 0 {
		t.Fatalf("banner does not contain expected settings link prefix %q\n"+
			"rendered HTML:\n%s", prefix, html)
	}
	rest := html[idx+len(prefix):]
	// The tab value runs until the next quote or non-tab char.
	end := strings.IndexAny(rest, "\"'& <")
	if end < 0 {
		t.Fatalf("could not find end of tab value in href: %q", rest[:minInt(50, len(rest))])
	}
	tab := rest[:end]

	if !validSettingsTabs[tab] {
		t.Errorf("CampaignShowFoundryBanner links to ?tab=%q — not a valid "+
			"settings tab.\n"+
			"Valid tabs (from campaigns/settings.templ buttons): general, "+
			"features, people, integrations, activity.\n"+
			"The foundry-vtt settings fragment lives inside the "+
			"'integrations' tab, so this banner should target ?tab=integrations.\n"+
			"\nThis is the cordinator Issue #16 regression. If a tab "+
			"rename made %q valid intentionally, update validSettingsTabs "+
			"in this test.",
			tab, tab)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
