package ai_workspace

// import_nav_test.go pins that the campaign settings route and the Your AI
// page, which render full pages (neither has an IsHTMX fragment branch), are
// only ever reached by navigation (plain anchors under hx-boost) and never by
// an hx-get that would fragment-swap them into a host div, nesting the app
// inside itself.

import (
	"os"
	"regexp"
	"testing"
)

// TestSettingsRouteNeverFragmentTargeted asserts no templ source in this
// plugin issues an hx-get against the settings page or the Your AI page.
// Plain href anchors are the sanctioned way back to them.
func TestSettingsRouteNeverFragmentTargeted(t *testing.T) {
	files := []string{"import_review.templ", "import_result.templ", "tab.templ", "modal.templ", "ai_page.templ", "lookup_answer.templ"}
	re := regexp.MustCompile(`hx-get=\{[^}]*(settings\?tab=|/campaigns/%s/ai"\s*,)`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue // optional surfaces may not exist in older trees
		}
		if re.Match(src) {
			t.Errorf("%s: hx-get targets a full-page route (settings or Your AI) — use a plain <a href> (boosted nav) instead; fragment-swapping it nests the app inside itself", f)
		}
	}
}
