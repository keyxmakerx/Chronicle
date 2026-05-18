// C-UPDATER-NOTIFICATION-LINK regression test: pins the
// foundryModuleUpdateBanner's settings link against the canonical
// set of campaign-settings tabs. The original bug (cordinator
// Issue #16) was that the link pointed at ?tab=foundry — a tab
// that does not exist after the C-FMC-5b/5c rename. Clicking it
// loaded the settings page with Alpine's `tab` initialized to a
// no-match value, so every tab content section hid → blank page.
//
// This file lives in package campaigns so it can call the
// unexported foundryModuleUpdateBanner directly.
package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// validSettingsTabs is the authoritative set of campaign-settings
// tab values, mirroring the five `tab = '<value>'` buttons in
// settings.templ (general / features / people / integrations /
// activity). If a tab is renamed or added there, update this set;
// the renaming PR's reviewer should also re-confirm every
// ?tab=<x> URL repo-wide via grep.
var validSettingsTabs = map[string]bool{
	"general":      true,
	"features":     true,
	"people":       true,
	"integrations": true,
	"activity":     true,
}

// TestFoundryModuleUpdateBanner_LinksToValidSettingsTab pins the
// banner's "Update pin" link against validSettingsTabs. If the
// banner regresses to ?tab=foundry (or any non-existent tab),
// this test fails with a clear message naming both sites.
func TestFoundryModuleUpdateBanner_LinksToValidSettingsTab(t *testing.T) {
	component := foundryModuleUpdateBanner("test-campaign-id", FoundryModuleBanner{
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
	// `/campaigns/<id>/settings?tab=...` href; extract its tab
	// value.
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
		t.Fatalf("could not find end of tab value in href: %q", rest[:min(50, len(rest))])
	}
	tab := rest[:end]

	if !validSettingsTabs[tab] {
		t.Errorf("foundryModuleUpdateBanner links to ?tab=%q — not a valid "+
			"settings tab.\n"+
			"Valid tabs (from settings.templ buttons): general, features, "+
			"people, integrations, activity.\n"+
			"The foundry-vtt settings fragment lives inside the "+
			"'integrations' tab (settings.templ around line 125 / 760), "+
			"so this banner should target ?tab=integrations.\n"+
			"\nThis is the cordinator Issue #16 regression. If a tab "+
			"rename made %q valid intentionally, update validSettingsTabs "+
			"in this test.",
			tab, tab)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
