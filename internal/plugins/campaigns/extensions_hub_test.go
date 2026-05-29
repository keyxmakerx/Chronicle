// extensions_hub_test.go — C-EXT-HUB Phase 1 tests.
//
// Covers:
//   - HasExtensionDashboard / HasExtensionEntitySetup slug tables
//     (the capability lookup the addonListerAdapter calls).
//   - ExtensionsHubFragment render: 1-element-array case, zero
//     state, mixed enabled/disabled cards. The render is exercised
//     against an in-memory buffer using the generated templ.go to
//     pin the contract templ produces (no template engine reaches
//     the DB; safe for table-driven coverage).
//   - PluginHub redirect now lands on the new hub.
//   - extensionEnabledAttr helper.
//
// Owner gating is enforced at the Echo route layer
// (`RequireRole(RoleOwner)` on the /extensions routes) and is
// exercised by the existing routing/middleware tests; not duplicated
// here.

package campaigns

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHasExtensionDashboard_KnownSlugs(t *testing.T) {
	cases := []struct {
		slug string
		want bool
	}{
		{"calendar", true},
		{"maps", false},
		{"timeline", false}, // joins after C-TIMELINE-V2 lands
		{"notes", false},
		{"unknown-slug", false},
		{"", false},
	}
	for _, c := range cases {
		if got := HasExtensionDashboard(c.slug); got != c.want {
			t.Errorf("HasExtensionDashboard(%q) = %v, want %v", c.slug, got, c.want)
		}
	}
}

func TestHasExtensionEntitySetup_KnownSlugs(t *testing.T) {
	cases := []struct {
		slug string
		want bool
	}{
		{"calendar", true},
		{"timeline", false}, // Phase 4 calendar-only this wave
		{"maps", false},     // maps already has its own setup; not a Phase 4 target
		{"", false},
	}
	for _, c := range cases {
		if got := HasExtensionEntitySetup(c.slug); got != c.want {
			t.Errorf("HasExtensionEntitySetup(%q) = %v, want %v", c.slug, got, c.want)
		}
	}
}

func TestExtensionEnabledAttr(t *testing.T) {
	if got := extensionEnabledAttr(true); got != "true" {
		t.Errorf("extensionEnabledAttr(true) = %q, want %q", got, "true")
	}
	if got := extensionEnabledAttr(false); got != "false" {
		t.Errorf("extensionEnabledAttr(false) = %q, want %q", got, "false")
	}
}

func TestExtensionsHubFragment_EmptyState(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Test"}}
	var buf bytes.Buffer
	if err := ExtensionsHubFragment(cc, nil, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render empty fragment: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "No extensions available yet") {
		t.Errorf("empty-state copy missing from render; got:\n%s", html)
	}
	if strings.Contains(html, "data-extension-card") {
		t.Errorf("empty state should not render any extension cards; got:\n%s", html)
	}
}

// TestExtensionsHubFragment_SingleElementArray is the single-element-
// array fixture the dispatch standing-pattern calls out — surfaces
// any rendering branch that assumes len>1 (e.g. grid-cols collapse,
// trailing-comma errors in JSON serialization).
func TestExtensionsHubFragment_SingleElementArray(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Test"}}
	addons := []PluginHubAddon{
		{
			AddonID:      1,
			Slug:         "calendar",
			Name:         "Calendar",
			Icon:         "fa-calendar-days",
			Category:     "plugin",
			Enabled:      true,
			Installed:    true,
			HasDashboard: true,
		},
	}
	var buf bytes.Buffer
	if err := ExtensionsHubFragment(cc, addons, "csrf-token").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render single-element fragment: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, `data-extension-slug="calendar"`) {
		t.Errorf("calendar card slug attr missing; got:\n%s", html)
	}
	if !strings.Contains(html, `data-extension-enabled="true"`) {
		t.Errorf("enabled-state attr missing for enabled card; got:\n%s", html)
	}
	// Phase 2: the expand affordance is now wired to the hub
	// fragment route. The button hx-gets /extensions/calendar/dashboard
	// into the panel below the catalog (one-card-at-a-time swap).
	if !strings.Contains(html, `data-extension-dashboard-expand`) {
		t.Errorf("expand affordance missing on HasDashboard card; got:\n%s", html)
	}
	if !strings.Contains(html, `hx-get="/campaigns/camp-1/extensions/calendar/dashboard"`) {
		t.Errorf("expand affordance should hx-get the calendar dashboard fragment; got:\n%s", html)
	}
	if !strings.Contains(html, `hx-target="#extensions-hub-dashboard-panel"`) {
		t.Errorf("expand affordance should target the panel slot; got:\n%s", html)
	}
	if !strings.Contains(html, "Open dashboard") {
		t.Errorf("expand affordance copy should read 'Open dashboard'; got:\n%s", html)
	}
	// Toggle should target the existing addons endpoint with
	// redirect_to=extensions-hub so the in-page refresh fires.
	if !strings.Contains(html, `/campaigns/camp-1/addons/1/toggle`) {
		t.Errorf("toggle endpoint URL wrong; got:\n%s", html)
	}
	if !strings.Contains(html, `name="redirect_to" value="extensions-hub"`) {
		t.Errorf("redirect_to=extensions-hub missing from toggle form; got:\n%s", html)
	}
}

func TestExtensionsHubFragment_MixedEnabledDisabledAndNotInstalled(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "c-2"}}
	addons := []PluginHubAddon{
		{AddonID: 1, Slug: "calendar", Name: "Calendar", Installed: true, Enabled: true, HasDashboard: true},
		{AddonID: 2, Slug: "maps", Name: "Interactive Maps", Installed: true, Enabled: false},
		{AddonID: 3, Slug: "family-tree", Name: "Family Tree", Installed: false, Enabled: false},
	}
	var buf bytes.Buffer
	if err := ExtensionsHubFragment(cc, addons, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render mixed: %v", err)
	}
	html := buf.String()
	// Installed cards render a toggle form; the not-installed card
	// renders the inert "Soon" badge instead.
	formCount := strings.Count(html, "<form")
	if formCount != 2 {
		t.Errorf("expected 2 toggle forms (calendar+maps); got %d in:\n%s", formCount, html)
	}
	if !strings.Contains(html, "Soon</span>") {
		t.Errorf("uninstalled addon should render Soon badge; got:\n%s", html)
	}
	// Only the HasDashboard=true card carries the expand affordance.
	if c := strings.Count(html, "data-extension-dashboard-expand"); c != 1 {
		t.Errorf("expand-affordance count = %d, want 1 (only HasDashboard cards); got:\n%s", c, html)
	}
}

// TestPluginHubRedirect_LandsOnExtensionsHub pins the redirect change:
// the legacy /plugins URL must redirect to the new top-level
// Extensions hub, not the retired Settings>Features tab.
func TestPluginHubRedirect_LandsOnExtensionsHub(t *testing.T) {
	h := &Handler{}
	e := echo.New()
	req := httptest.NewRequest("GET", "/campaigns/c-1/plugins", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("c-1")
	// PluginHub reads campaign context via GetCampaignContext; mirror
	// what the middleware would have set.
	cc := &CampaignContext{Campaign: &Campaign{ID: "c-1"}}
	c.Set("campaign_context", cc)

	if err := h.PluginHub(c); err != nil {
		t.Fatalf("PluginHub returned error: %v", err)
	}
	if rec.Code != 303 {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	want := "/campaigns/c-1/extensions"
	if loc != want {
		t.Errorf("redirect Location=%q, want %q (Features-tab path is retired)", loc, want)
	}
}
