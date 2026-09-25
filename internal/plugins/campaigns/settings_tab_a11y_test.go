// settings_tab_a11y_test.go pins #725: the campaign settings tab bar
// carries proper ARIA tab semantics and roving-tabindex arrow-key
// navigation, matching the Availability page's view switcher, with no
// visual change.

package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestSettingsTabBar_TabSemanticsAndArrowKeyNav(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Test"}}
	tabs := []SettingsTab{
		{ID: "general", Label: "General", Icon: "fa-solid fa-gear", Content: templ.Raw("general content")},
		{ID: "people", Label: "People", Icon: "fa-solid fa-users", Content: templ.Raw("people content")},
		{ID: "integrations", Label: "Integrations", Icon: "fa-solid fa-plug", Content: templ.Raw("integrations content")},
		{ID: "activity", Label: "Activity", Icon: "fa-solid fa-clock-rotate-left", Content: templ.Raw("activity content")},
	}
	var buf bytes.Buffer
	if err := CampaignSettingsPage(cc, nil, "csrf", "", "general", tabs).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `role="tablist"`) {
		t.Errorf("tab bar must declare role=tablist; got:\n%s", html)
	}
	if c := strings.Count(html, `role="tab"`); c != len(tabs) {
		t.Errorf("expected %d role=tab buttons, got %d; got:\n%s", len(tabs), c, html)
	}
	if c := strings.Count(html, `role="tabpanel"`); c != len(tabs) {
		t.Errorf("expected %d role=tabpanel panels, got %d; got:\n%s", len(tabs), c, html)
	}

	for _, tab := range tabs {
		if !strings.Contains(html, `id="`+tab.ID+`-tab"`) {
			t.Errorf("tab button for %q missing a stable id; got:\n%s", tab.ID, html)
		}
		if !strings.Contains(html, `aria-controls="`+tab.ID+`-panel"`) {
			t.Errorf("tab button for %q missing aria-controls pointing at its panel; got:\n%s", tab.ID, html)
		}
		if !strings.Contains(html, `aria-labelledby="`+tab.ID+`-tab"`) {
			t.Errorf("panel for %q missing aria-labelledby pointing back at its tab; got:\n%s", tab.ID, html)
		}
	}

	// Only the active tab is a Tab-key stop (roving tabindex); the rest
	// are reached by arrow key, not by tabbing past each one. templ
	// HTML-escapes the Alpine expression's quotes (' -> &#39;).
	if !strings.Contains(html, `tab === &#39;general&#39; ? &#39;0&#39; : &#39;-1&#39;`) {
		t.Errorf("tabs must use a roving tabindex bound to the active tab; got:\n%s", html)
	}

	if !strings.Contains(html, "moveTab(1)") || !strings.Contains(html, "moveTab(-1)") {
		t.Errorf("tab buttons must handle Left/Right arrow-key navigation; got:\n%s", html)
	}
}
