package systems

import (
	"context"
	"strings"
	"testing"
)

// operator_diag_campaign_test.go covers the two campaign diagnostics.
//
// The DEGRADED paths get as much attention as the happy ones, deliberately.
// These exist because the catalog could previously answer a campaign
// question only by implication, and the failure they are built to prevent is a
// confident wrong answer — which is exactly what a diagnostic produces when an
// unread table renders as an empty one.

// fakeCampaignProvider is a scripted CampaignDiagProvider. It returns whatever
// the test sets, including errors, so every degraded branch is reachable
// without a database.
type fakeCampaignProvider struct {
	surf    CampaignSurfaceFacts
	surfErr error
	conf    CampaignConfigFacts
	confErr error
}

func (f *fakeCampaignProvider) SurfaceFacts(context.Context, string) (CampaignSurfaceFacts, error) {
	return f.surf, f.surfErr
}
func (f *fakeCampaignProvider) ConfigFacts(context.Context, string) (CampaignConfigFacts, error) {
	return f.conf, f.confErr
}

// withCampaignProvider installs a provider for the duration of fn and restores
// whatever was there — the provider is package state, so a test that leaked one
// would silently change every later test in the package.
func withCampaignProvider(t *testing.T, p CampaignDiagProvider, fn func()) {
	t.Helper()
	prev := campaignDiagProvider
	campaignDiagProvider = p
	defer func() { campaignDiagProvider = prev }()
	fn()
}

// campaignDiagNames is the set this file covers, used by the loop tests so a
// third diagnostic added later inherits them.
var campaignDiagNames = []string{"campaign.surfaces", "campaign.config"}

func boolPtr(b bool) *bool { return &b }

// ── degraded paths ──────────────────────────────────────────────────────────

// TestCampaignDiagnostics_UnwiredProviderSaysSo is the requirement stated in
// the file header of operator_diag_wiring_test.go: an unwired provider must
// announce itself. Both print a shared sentence that names the state AND
// denies the misreading, because "no data" and "nobody was asked" are one
// careless render apart.
func TestCampaignDiagnostics_UnwiredProviderSaysSo(t *testing.T) {
	withCampaignProvider(t, nil, func() {
		cat := diagnosticCatalog()
		for _, name := range campaignDiagNames {
			got, ok := RunDiagnostic(cat, name, "some-campaign-id")
			if !ok {
				t.Fatalf("%s: not in the catalog", name)
			}
			if !strings.Contains(got, "provider not wired") {
				t.Errorf("%s: an unwired provider must say so, got:\n%s", name, got)
			}
			if !strings.Contains(got, "NOT") {
				t.Errorf("%s: the unwired message must DENY the empty-answer reading, not merely state the fact:\n%s", name, got)
			}
		}
	})
}

// TestCampaignDiagnostics_EmptyArgPrintsUsage covers the bare Run button on the
// admin page — the call an operator makes by accident. It must print usage,
// never a blank pane and never a panic.
func TestCampaignDiagnostics_EmptyArgPrintsUsage(t *testing.T) {
	withCampaignProvider(t, &fakeCampaignProvider{}, func() {
		cat := diagnosticCatalog()
		for _, name := range campaignDiagNames {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked with an empty arg: %v", r)
					}
				}()
				got, _ := RunDiagnostic(cat, name, "")
				if !strings.HasPrefix(got, "## "+name) {
					t.Errorf("output must name itself; got:\n%s", got)
				}
				if !strings.Contains(got, "Usage") {
					t.Errorf("an empty arg must print usage, got:\n%s", got)
				}
				if strings.Contains(got, "%!") {
					t.Errorf("formatting error in output:\n%s", got)
				}
			})
		}
	})
}

// TestCampaignDiagnostics_UnknownCampaignIsNotAnEmptyAnswer pins the other way
// a diagnostic can lie: a mistyped id must read as "no such campaign", not as
// "this campaign has nothing".
func TestCampaignDiagnostics_UnknownCampaignIsNotAnEmptyAnswer(t *testing.T) {
	withCampaignProvider(t, &fakeCampaignProvider{}, func() { // all Found=false
		cat := diagnosticCatalog()
		for _, name := range campaignDiagNames {
			got, _ := RunDiagnostic(cat, name, "nope")
			if !strings.Contains(got, "No campaign `nope`") {
				t.Errorf("%s: an unknown campaign must be named as such, got:\n%s", name, got)
			}
			if !strings.Contains(got, "campaigns.list") {
				t.Errorf("%s: point the reader at the discovery diagnostic, got:\n%s", name, got)
			}
		}
	})
}

// ── campaign.surfaces ───────────────────────────────────────────────────────

// rebuildNoticeHandler stands in for the one anonymous closure every live
// calendar route shares (calendarRebuildNotice in internal/app/routes.go).
// It is never compared against — calendarSurfaceMap declares Handler: "" for
// all three rows precisely so a real closure name is never pinned — only
// displayed, so any distinctive string does.
const rebuildNoticeHandler = "github.com/keyxmakerx/chronicle/internal/app.RegisterRoutes.func1"

// liveCalendarRoutes is a stand-in route table: exactly the declared rows,
// each given the one handler they really share.
func liveCalendarRoutes() []RouteFact {
	var out []RouteFact
	for _, row := range calendarSurfaceMap() {
		out = append(out, RouteFact{Method: "GET", Path: row.Path, Handler: rebuildNoticeHandler})
	}
	return out
}

func surfaceFactsWith(routes []RouteFact) CampaignSurfaceFacts {
	return CampaignSurfaceFacts{
		Found: true, CampaignID: "c1", CampaignName: "T",
		Routes:               routes,
		CalendarAddonEnabled: boolPtr(true),
		SidebarCalendarPath:  "/apps/calendar",
	}
}

// TestCampaignSurfaces_MatchesTheLiveTable is the happy path: every declared
// row resolves to the handler the map expects.
func TestCampaignSurfaces_MatchesTheLiveTable(t *testing.T) {
	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(liveCalendarRoutes())}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if strings.Contains(got, "NOT REGISTERED") {
			t.Errorf("every declared route was supplied; nothing should be missing:\n%s", got)
		}
		if strings.Contains(got, "Trust the live handler") {
			t.Errorf("no handler disagreed:\n%s", got)
		}
		for _, want := range []string{
			"**CURRENT** `/campaigns/:id/apps/calendar`",
			"**CURRENT** `/campaigns/:id/calendar`",
			"/campaigns/c1/apps/calendar",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
		if strings.Contains(got, "nothing classifies") {
			t.Errorf("every supplied route is either declared or discovered:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_HandlerSurfacesIsEmpty pins that the discovery map is
// empty rather than missing: the V2 shell it used to cover was deleted
// outright with the pre-V5 plugin, not merely made unreachable, so there is
// nothing left to discover by handler. A future addition belongs here, keyed
// on a stable handler name — see the function's own comment.
func TestCampaignSurfaces_HandlerSurfacesIsEmpty(t *testing.T) {
	if got := handlerSurfaces(); len(got) != 0 {
		t.Errorf("want no handler-keyed surfaces, got %v", got)
	}
	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(liveCalendarRoutes())}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if strings.Contains(got, "DISCOVERED from the live table") {
			t.Errorf("with an empty discovery map there is nothing to discover:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_MissingRouteIsLoud. A map entry the binary does not have
// is a claim that a page exists when it does not — the exact error this family
// is built to stop making.
func TestCampaignSurfaces_MissingRouteIsLoud(t *testing.T) {
	routes := liveCalendarRoutes()[1:] // drop /apps/calendar
	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(routes)}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "NOT REGISTERED IN THIS BUILD") {
			t.Errorf("a declared-but-absent route must be flagged:\n%s", got)
		}
		if !strings.Contains(got, "do not conclude the surface exists") {
			t.Errorf("the flag must tell the reader what NOT to conclude:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_UnclassifiedRouteIsListed. A calendar page nobody
// classified must appear; hiding it would make the map look complete.
func TestCampaignSurfaces_UnclassifiedRouteIsListed(t *testing.T) {
	routes := append(liveCalendarRoutes(),
		RouteFact{Method: "GET", Path: "/campaigns/:id/calendar/v9", Handler: "pkg.(*Handler).ShowV9-fm"})
	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(routes)}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "nothing classifies") || !strings.Contains(got, "/campaigns/:id/calendar/v9") {
			t.Errorf("an unclassified calendar GET must be listed:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_UnreadableTableIsNotAnAbsence.
func TestCampaignSurfaces_UnreadableTableIsNotAnAbsence(t *testing.T) {
	f := surfaceFactsWith(nil)
	f.RoutesNote = "**The live route table came back EMPTY**"
	withCampaignProvider(t, &fakeCampaignProvider{surf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "NOT CHECKED") {
			t.Errorf("with no table, registration must read as unchecked, not as missing:\n%s", got)
		}
		if strings.Contains(got, "NOT REGISTERED IN THIS BUILD") {
			t.Errorf("an unread table must never produce a not-registered verdict:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_DisabledAddonGatesOnlyTheListPage. Before the rebuild
// a disabled addon made every calendar route unreachable. Now /apps/calendar
// and /calendar redirect regardless of the addon's state, but /calendars and
// the calendar JSON API (calendar.RegisterRoutes' own RequireAddon) are still
// gated on it; the diagnostic must say so precisely, not claim either "gates
// everything" or "gates nothing".
func TestCampaignSurfaces_DisabledAddonGatesOnlyTheListPage(t *testing.T) {
	f := surfaceFactsWith(liveCalendarRoutes())
	f.CalendarAddonEnabled = boolPtr(false)
	withCampaignProvider(t, &fakeCampaignProvider{surf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "calendar addon: **disabled**") {
			t.Errorf("the addon state must be stated:\n%s", got)
		}
		if !strings.Contains(got, "`/calendars` itself 404s while disabled") {
			t.Errorf("disabled must be read as gating the list page specifically:\n%s", got)
		}
		if !strings.Contains(got, "redirect either way") {
			t.Errorf("the redirect routes must be stated as ungated:\n%s", got)
		}
		if !strings.Contains(got, "as does the calendar JSON API") {
			t.Errorf("disabled must be read as gating the JSON API too:\n%s", got)
		}
		if !strings.Contains(got, "**CURRENT** `/campaigns/:id/apps/calendar`") {
			t.Errorf("the route table must still print despite the disabled addon:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_HandAuthoredSidebarLinkIsShown. A `type:"link"` item can
// point at any URL, including a retired surface that no longer resolves to
// anything — the diagnostic must print it regardless.
func TestCampaignSurfaces_HandAuthoredSidebarLinkIsShown(t *testing.T) {
	f := surfaceFactsWith(liveCalendarRoutes())
	f.SidebarItems = []SidebarItemFact{
		{Type: "addon", Slug: "calendar", Visible: true},
		{Type: "link", Label: "Old calendar", URL: "/campaigns/c1/calendar/v2", Visible: true},
	}
	withCampaignProvider(t, &fakeCampaignProvider{surf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "Old calendar → `/campaigns/c1/calendar/v2`") {
			t.Errorf("a hand-authored link must be printed:\n%s", got)
		}
	})
}

// ── campaign.config ─────────────────────────────────────────────────────────

// TestCampaignConfig_PlacedSkyboxIsNamedAndDistinguished. This is the whole
// reason campaign.config exists — and the answer has to disambiguate the two
// things called "skybox", because the operator uses one word for both.
func TestCampaignConfig_PlacedSkyboxIsNamedAndDistinguished(t *testing.T) {
	f := CampaignConfigFacts{
		Found: true, CampaignID: "c1", CampaignName: "T",
		DefaultDashboardBlocks: []string{"welcome_banner", "quick_actions", "category_grid", "recent_pages"},
		Layouts: []LayoutFact{
			{Surface: "`campaigns.dashboard_layout`", Stored: true, Blocks: []string{"welcome_banner", "skybox", "skybox"}},
		},
	}
	withCampaignProvider(t, &fakeCampaignProvider{conf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.config", "c1")
		if !strings.Contains(got, "`skybox` ×2") {
			t.Errorf("duplicates must be counted, not collapsed:\n%s", got)
		}
		// The two things nicknamed "skybox" must stay distinguished, and
		// the text must say what the placement renders TODAY — the real
		// sky pane (issue #763), not the rebuilding notice it used to
		// answer with before the sky pane was wired in.
		if !strings.Contains(got, "sky pane placement") || !strings.Contains(got, "distinct from the v4 sky band") {
			t.Errorf("the two things called skybox must be distinguished:\n%s", got)
		}
		if strings.Contains(got, "rebuilding notice") {
			t.Errorf("the skybox placement now renders the real sky pane, not the rebuilding notice:\n%s", got)
		}
		if !strings.Contains(got, "no migration seeds one") {
			t.Errorf("the 'it can only be hand-placed' claim must be stated:\n%s", got)
		}
		if !strings.Contains(got, "welcome_banner") {
			t.Errorf("the default layout must be printed for comparison:\n%s", got)
		}
	})
}

// TestCampaignConfig_NullLayoutIsNotAnEmptyOne. "Not customised" and "customised
// to nothing" are different states with different remedies.
func TestCampaignConfig_NullLayoutIsNotAnEmptyOne(t *testing.T) {
	f := CampaignConfigFacts{
		Found: true, CampaignID: "c1", CampaignName: "T",
		Layouts: []LayoutFact{
			{Surface: "null", Stored: false},
			{Surface: "empty", Stored: true},
			{Surface: "broken", Stored: true, ParseErr: "invalid character 'x'"},
		},
	}
	withCampaignProvider(t, &fakeCampaignProvider{conf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.config", "c1")
		for _, want := range []string{
			"not customised (column is NULL)",
			"stored but contains **no blocks**",
			"could not be parsed",
			"invalid character 'x'",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})
}

// TestCampaignConfig_DisabledAddonIsMarked.
func TestCampaignConfig_DisabledAddonIsMarked(t *testing.T) {
	f := CampaignConfigFacts{
		Found: true, CampaignID: "c1", CampaignName: "T",
		Addons: []AddonFact{
			{Slug: "calendar", Name: "Calendar", Enabled: false, Installed: true, Status: "stable"},
			{Slug: "notes", Name: "Journal", Enabled: true, Installed: true},
		},
	}
	withCampaignProvider(t, &fakeCampaignProvider{conf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.config", "c1")
		if !strings.Contains(got, "✗ disabled `calendar`") {
			t.Errorf("a disabled addon must be marked:\n%s", got)
		}
		if !strings.Contains(got, "✓ enabled `notes`") {
			t.Errorf("an enabled addon must be marked:\n%s", got)
		}
		if !strings.Contains(got, "gates the calendar pages and the calendar plugin's JSON API (404)") {
			t.Errorf("a disabled calendar addon must be read as gating the pages and the JSON API:\n%s", got)
		}
		if !strings.Contains(got, "still redirect to the list page either way") {
			t.Errorf("the redirect routes must be stated as ungated:\n%s", got)
		}
	})
}

// TestCampaignConfig_UnreadableAddonListIsNotAnEmptyOne.
func TestCampaignConfig_UnreadableAddonListIsNotAnEmptyOne(t *testing.T) {
	f := CampaignConfigFacts{Found: true, CampaignID: "c1", CampaignName: "T", AddonsNote: "the addon list failed: boom"}
	withCampaignProvider(t, &fakeCampaignProvider{conf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.config", "c1")
		if !strings.Contains(got, "the addon list failed: boom") {
			t.Errorf("the read failure must be printed:\n%s", got)
		}
		if strings.Contains(got, "Every addon-gated feature is therefore off") {
			t.Errorf("a failed read must not render as 'no addons enabled':\n%s", got)
		}
	})
}

// ── catalog placement ───────────────────────────────────────────────────────

// TestCampaignDiagnosticsAreInTheCatalogInRankOrder. The catalog is a menu an
// assistant reads to choose; the 2026-08-11 ranking is a judgement about which
// question is asked most, and it should survive a later edit.
func TestCampaignDiagnosticsAreInTheCatalogInRankOrder(t *testing.T) {
	idx := map[string]int{}
	for i, d := range diagnosticCatalog() {
		idx[d.Name] = i
	}
	for i := 1; i < len(campaignDiagNames); i++ {
		prev, cur := campaignDiagNames[i-1], campaignDiagNames[i]
		p, okp := idx[prev]
		c, okc := idx[cur]
		if !okp || !okc {
			t.Fatalf("catalog is missing %q or %q", prev, cur)
		}
		if p > c {
			t.Errorf("%s (rank %d) must precede %s (rank %d) in the catalog", prev, i, cur, i+1)
		}
	}
	if idx["campaigns.list"] > idx["campaign.surfaces"] {
		t.Error("campaigns.list supplies the argument these two need and must come first")
	}
}

// TestCampaignDiagnosticsAreCampaignScopedForTheBatchWorkspace. The AI-workflow
// review step offers a campaign picker only for calls whose campaign slot it
// recognises; a diagnostic missing from campaignSlot silently loses that
// substitution and runs against a placeholder id.
func TestCampaignDiagnosticsAreCampaignScopedForTheBatchWorkspace(t *testing.T) {
	for _, name := range campaignDiagNames {
		if !CampaignSlotIsAmbiguous(name, "<campaignId>") {
			t.Errorf("%s: a placeholder campaign must be reported ambiguous so the review step offers the picker", name)
		}
		if CampaignSlotIsAmbiguous(name, "real-id") {
			t.Errorf("%s: a real campaign id must not be reported ambiguous", name)
		}
		if got := WithCampaign(name, "<campaignId>", "c1"); got != "c1" {
			t.Errorf("%s takes the campaign and nothing else: got %q", name, got)
		}
	}
}

// TestDeployCheckPointsAtCampaignDiagnostics pins that host.deploy-check's
// Desc refuses a marker-hit-means-it-renders reading and points to the
// campaign diagnostics instead: a marker in the build proves it shipped,
// never that it renders.
func TestDeployCheckPointsAtCampaignDiagnostics(t *testing.T) {
	var desc string
	for _, d := range diagnosticCatalog() {
		if d.Name == "host.deploy-check" {
			desc = d.Desc
		}
	}
	if desc == "" {
		t.Fatal("host.deploy-check is not in the catalog")
	}
	if !strings.Contains(desc, "nothing about whether it RENDERS") {
		t.Errorf("the Desc must refuse the render reading:\n%s", desc)
	}
	if !strings.Contains(desc, "campaign.config") || !strings.Contains(desc, "campaign.surfaces") {
		t.Errorf("the Desc must name the diagnostics that DO answer it:\n%s", desc)
	}

	out := renderHostDeployCheckFrom(deployCheckSources{}, "some-marker")
	if !strings.Contains(out, "IT PROVES NOTHING ABOUT WHETHER IT RENDERS") {
		t.Errorf("the marker section itself must carry the caveat, where somebody is looking at a tick:\n%s", out)
	}
	if !strings.Contains(out, "campaign.config") || !strings.Contains(out, "campaign.surfaces") {
		t.Errorf("the marker section must name the diagnostics that answer the render question:\n%s", out)
	}
}
