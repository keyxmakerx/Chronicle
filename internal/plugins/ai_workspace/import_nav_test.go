package ai_workspace

// import_nav_test.go — pins the "website inside the website" bug class
// (operator, 2026-06-12): the campaign settings route renders a FULL page
// (campaigns.Settings has no IsHTMX fragment branch), so it must only ever be
// reached by NAVIGATION (plain anchors under hx-boost), never by an hx-get
// that fragment-swaps it into a host div. Canceling a failed AI import did
// exactly that and nested the entire app inside the review host.

import (
	"os"
	"regexp"
	"testing"
)

// TestSettingsRouteNeverFragmentTargeted asserts no templ source in this
// plugin issues an hx-get against the settings page. Plain href anchors are
// the sanctioned way back to the tab.
func TestSettingsRouteNeverFragmentTargeted(t *testing.T) {
	files := []string{"import_review.templ", "import_result.templ", "tab.templ", "modal.templ"}
	re := regexp.MustCompile(`hx-get=\{[^}]*settings\?tab=`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue // optional surfaces may not exist in older trees
		}
		if re.Match(src) {
			t.Errorf("%s: hx-get targets the full-page settings route — use a plain <a href> (boosted nav) instead; fragment-swapping it nests the app inside itself", f)
		}
	}
}
