package systems

import (
	"context"
	"strings"
	"testing"
)

// operator_diag_campaign_test.go covers the four campaign diagnostics.
//
// The DEGRADED paths get as much attention as the happy ones, deliberately.
// These four exist because the catalog could previously answer a campaign
// question only by implication, and the failure they are built to prevent is a
// confident wrong answer — which is exactly what a diagnostic produces when an
// unread table renders as an empty one.

// fakeCampaignProvider is a scripted CampaignDiagProvider. It returns whatever
// the test sets, including errors, so every degraded branch is reachable
// without a database.
type fakeCampaignProvider struct {
	cal     CampaignCalendarFacts
	calErr  error
	surf    CampaignSurfaceFacts
	surfErr error
	conf    CampaignConfigFacts
	confErr error

	// lastUserID records what the diagnostic passed through, so the arg parsing
	// is asserted at the seam rather than inferred from the rendered text.
	lastUserID string
}

func (f *fakeCampaignProvider) CalendarFacts(_ context.Context, _, userID string) (CampaignCalendarFacts, error) {
	f.lastUserID = userID
	return f.cal, f.calErr
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
// fifth diagnostic added later inherits them.
var campaignDiagNames = []string{"calendar.render", "calendar.config", "campaign.surfaces", "campaign.config"}

func boolPtr(b bool) *bool { return &b }

// ── degraded paths ──────────────────────────────────────────────────────────

// TestCampaignDiagnostics_UnwiredProviderSaysSo is the requirement stated in
// the file header of operator_diag_wiring_test.go: an unwired provider must
// announce itself. All four print a shared sentence that names the state AND
// denies the misreading, because "no calendars" and "nobody was asked" are one
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

// ── calendar.render ─────────────────────────────────────────────────────────

// twoCalendarCampaign is the shape the 2026-08-11 operator most likely had: one
// in-world calendar (the campaign default) and one real-world calendar beside
// it. The real-world one takes the second seat and gets NO sky, by [SKY-1].
func twoCalendarCampaign() CampaignCalendarFacts {
	return CampaignCalendarFacts{
		Found: true, CampaignID: "c1", CampaignName: "Test",
		AddonEnabled: boolPtr(true), SpineInstalled: true,
		ListVia: "ListCalendars — the OWNER branch (no viewer supplied)",
		All: []DiagCalendar{
			{ID: "in", Name: "Harptos", Mode: "fantasy", IsDefault: true, Months: 12, MoonsRendered: 2, MoonRowsStored: 2, MoonNames: []string{"Selune", "Tears"}},
			{ID: "rw", Name: "Earth", Mode: "reallife", Months: 12, MoonsRendered: 1, MoonRowsStored: 0, MoonNames: []string{"Moon"}, SynthesizedMoon: true},
		},
	}
}

// asViewer attaches a Player viewer to a campaign fixture, honouring the
// provider contract that Visible is non-nil once a viewer is named.
func asViewer(f CampaignCalendarFacts) CampaignCalendarFacts {
	f.Viewer = ViewerFacts{Supplied: true, Found: true, UserID: "u1", MemberRole: 1, Role: 1}
	f.ListVia = "ListVisibleCalendars(role=1) — the non-owner branch"
	f.Visible = append([]DiagCalendar{}, f.All...)
	return f
}

func TestCalendarRender_RealWorldBlockGetsNoSkyAndSaysWhy(t *testing.T) {
	f := twoCalendarCampaign()
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")

		for _, want := range []string{
			"PRIMARY — **Harptos**",
			"campaign default AND in-world",
			"REAL-WORLD — **Earth**",
			"SkyOn: **false**, ShelfHidden: **true**",
			"no sky band at all",
			"Adding a moon to it changes nothing visible here",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
		// The Almanac gate is (!ShelfHidden || SkyOn): the real-world seat fails
		// BOTH halves, so its register is not built even though it has a moon.
		if !strings.Contains(got, "Almanac register built: **true**") {
			t.Error("the Primary (shelf shown, 2 moons) must build its Almanac")
		}
		if !strings.Contains(got, "Almanac register built: **false**") {
			t.Error("the real-world Block (shelf hidden, no sky) must NOT build its Almanac, even with a moon")
		}
		if !strings.Contains(got, "the calendar HAS moons and the register was still not built") {
			t.Error("an unbuilt register on a calendar that HAS moons must be explained, not merely reported")
		}
	})
}

// TestCalendarRender_SoleRealWorldCalendarIsPromoted pins the correction the
// 2026-08-11 report had to make to one of its own lanes: with no in-world
// calendar, benchClassify promotes the real-world one to PRIMARY, and it DOES
// get a sky. That flips the remedy from "signed behaviour, adding data will not
// help" to "adding one moon fixes it", so the trace has to distinguish them.
func TestCalendarRender_SoleRealWorldCalendarIsPromoted(t *testing.T) {
	f := CampaignCalendarFacts{
		Found: true, CampaignID: "c1", CampaignName: "Test",
		AddonEnabled: boolPtr(true), SpineInstalled: true,
		All: []DiagCalendar{
			{ID: "rw", Name: "Earth", Mode: "reallife", IsDefault: true, Months: 12, MoonsRendered: 1, MoonNames: []string{"Moon"}, SynthesizedMoon: true},
		},
	}
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")
		if !strings.Contains(got, "PRIMARY — **Earth**") {
			t.Errorf("a sole real-world calendar must take the PRIMARY seat:\n%s", got)
		}
		if !strings.Contains(got, "NOTHING BUT REAL-WORLD CALENDARS") {
			t.Errorf("the clause must name WHY it was promoted:\n%s", got)
		}
		if !strings.Contains(got, "SkyOn: **true**") {
			t.Errorf("the promoted Primary carries the sky:\n%s", got)
		}
		if strings.Contains(got, "REAL-WORLD — ") {
			t.Errorf("with one calendar there is no second seat:\n%s", got)
		}
		if !strings.Contains(got, "has **no database row**") {
			t.Errorf("the synthesized Moon must be named as having no row:\n%s", got)
		}
	})
}

// TestCalendarRender_SectionProvenance covers the distinction [BR2-4] turns on:
// nil (never chosen → all four closed) is NOT the same as an empty stored list
// (chose to close nothing → all four open).
func TestCalendarRender_SectionProvenance(t *testing.T) {
	tests := []struct {
		name        string
		stored      []string
		neverChosen bool
		wantState   string
		wantWhy     string
	}{
		{"never chosen", nil, true, "`rsvp` — **CLOSED**", "NO STORED ROW"},
		{"closed nothing", []string{}, false, "`rsvp` — **OPEN**", "a stored row"},
		{"closed rsvp only", []string{"rsvp"}, false, "`rsvp` — **CLOSED**", "a stored row"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := asViewer(twoCalendarCampaign())
			f.SectionsStored, f.SectionsNeverChosen = tc.stored, tc.neverChosen
			withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
				got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1:u1")
				if !strings.Contains(got, tc.wantState) {
					t.Errorf("want %q in:\n%s", tc.wantState, got)
				}
				if !strings.Contains(got, tc.wantWhy) {
					t.Errorf("want provenance %q in:\n%s", tc.wantWhy, got)
				}
			})
		})
	}
}

// TestCalendarRender_ClosedRsvpNamesBothCauses guards the sentence that answers
// the operator's first observation. It has to say two things at once: the panel
// IS there and collapsed, AND the integration they asked for does not exist at
// any disclosure state. Saying only the first would send them to click a chevron
// that does not answer their question.
func TestCalendarRender_ClosedRsvpNamesBothCauses(t *testing.T) {
	f := asViewer(twoCalendarCampaign())
	f.SectionsNeverChosen = true
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1:u1")
		for _, want := range []string{
			"only link to `/schedule` in the entire product",
			"zero RSVP markup by design",
			"Opening the chevron does not merge them",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})
}

// TestCalendarRender_AddonDisabledStopsTheTrace: a disabled addon removes every
// calendar route at the middleware, so everything below it would be a
// description of a page nobody can open.
func TestCalendarRender_AddonDisabledStopsTheTrace(t *testing.T) {
	f := twoCalendarCampaign()
	f.AddonEnabled = boolPtr(false)
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")
		if !strings.Contains(got, "DISABLED for this campaign") {
			t.Errorf("a disabled addon must be stated:\n%s", got)
		}
		if strings.Contains(got, "PRIMARY — ") {
			t.Errorf("the trace must STOP: describing seats on an unreachable page is a wrong answer:\n%s", got)
		}
	})
}

// TestCalendarRender_UnknownAddonStateIsNotFalse: a failed addon read must not
// be reported as "disabled". It is the difference between "your feature is off"
// and "we could not tell".
func TestCalendarRender_UnknownAddonStateIsNotFalse(t *testing.T) {
	f := twoCalendarCampaign()
	f.AddonEnabled, f.AddonNote = nil, "addons table unreachable"
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")
		if !strings.Contains(got, "calendar addon: **UNKNOWN**") || !strings.Contains(got, "addons table unreachable") {
			t.Errorf("an unreadable addon state must render as UNKNOWN with its reason:\n%s", got)
		}
		if strings.Contains(got, "DISABLED for this campaign") {
			t.Errorf("unknown must never render as disabled:\n%s", got)
		}
		// The trace continues: not knowing the gate is no reason to withhold
		// everything behind it.
		if !strings.Contains(got, "PRIMARY — ") {
			t.Errorf("an unknown gate should not suppress the rest of the trace:\n%s", got)
		}
	})
}

// TestCalendarRender_SpineNotInstalledExplainsTheEmptyPage: a degraded plugin
// renders every calendar as a row, which looks exactly like a campaign with no
// calendars.
func TestCalendarRender_SpineNotInstalledExplainsTheEmptyPage(t *testing.T) {
	f := twoCalendarCampaign()
	f.SpineInstalled = false
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")
		if !strings.Contains(got, "Block spine: **NOT INSTALLED**") {
			t.Errorf("a nil spine must be stated:\n%s", got)
		}
		if !strings.Contains(got, "FALLS BACK TO A ROW") {
			t.Errorf("the consequence must be stated on the seat itself:\n%s", got)
		}
	})
}

// TestCalendarRender_NoUserIdSaysWhichPathItTraced: the owner path is not what a
// player sees, and a trace that silently used it would answer the wrong
// question with total confidence.
func TestCalendarRender_NoUserIdSaysWhichPathItTraced(t *testing.T) {
	p := &fakeCampaignProvider{cal: twoCalendarCampaign()}
	withCampaignProvider(t, p, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")
		if p.lastUserID != "" {
			t.Errorf("no user id was supplied; the provider got %q", p.lastUserID)
		}
		if !strings.Contains(got, "No user id supplied") || !strings.Contains(got, "OWNER path") {
			t.Errorf("the traced path must be named:\n%s", got)
		}
	})
}

// TestCalendarRender_ArgSplitPassesTheUserThrough asserts the parsing at the
// seam rather than through the rendered text.
func TestCalendarRender_ArgSplitPassesTheUserThrough(t *testing.T) {
	p := &fakeCampaignProvider{cal: twoCalendarCampaign()}
	withCampaignProvider(t, p, func() {
		_, _ = RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1:user-42")
		if p.lastUserID != "user-42" {
			t.Errorf("want user-42 passed to the provider, got %q", p.lastUserID)
		}
	})
}

// TestCalendarRender_SaysItIsAMirror. The mirror warning is one of the two
// things keeping the re-derived rules honest (the other is the source pin), and
// it is the one the reader sees.
func TestCalendarRender_SaysItIsAMirror(t *testing.T) {
	withCampaignProvider(t, &fakeCampaignProvider{cal: twoCalendarCampaign()}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.render", "c1")
		if !strings.Contains(got, "RE-DERIVES the producer's rules") {
			t.Errorf("the trace must declare itself a mirror:\n%s", got)
		}
		if !strings.Contains(got, "trust the page") {
			t.Errorf("the trace must say which side wins on a disagreement:\n%s", got)
		}
	})
}

// ── calendar.config ─────────────────────────────────────────────────────────

// TestCalendarConfig_StoredAndRenderedMoonsAreSeparate is the whole point of
// rank 2: a GM told "1 moon" needs to know whether there is a row they can
// edit. Since the real-Moon fallback landed, "rendered" and "stored" disagree
// on exactly the calendar they are asking about.
func TestCalendarConfig_StoredAndRenderedMoonsAreSeparate(t *testing.T) {
	f := twoCalendarCampaign()
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1")
		if !strings.Contains(got, "**moons: 0 stored row(s), 1 rendered**") {
			t.Errorf("the two counts must both appear when they differ:\n%s", got)
		}
		if !strings.Contains(got, "There is nothing to edit in Settings → Moons") {
			t.Errorf("the synthesized body must say it is not editable:\n%s", got)
		}
		if !strings.Contains(got, "**moons: 2 stored row(s)**") {
			t.Errorf("agreeing counts print once:\n%s", got)
		}
	})
}

// TestCalendarConfig_InWorldZeroMoonsNamesTheCause: an in-world calendar with no
// moons is not a bug, it is seedDefaults declining to invent a sky. Saying so
// stops a reader "fixing" the seeder.
func TestCalendarConfig_InWorldZeroMoonsNamesTheCause(t *testing.T) {
	f := CampaignCalendarFacts{
		Found: true, CampaignID: "c1", CampaignName: "Test", AddonEnabled: boolPtr(true), SpineInstalled: true,
		All: []DiagCalendar{{ID: "in", Name: "Harptos", Mode: "fantasy", IsDefault: true, Months: 12}},
	}
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1")
		if !strings.Contains(got, "never calls `SetMoons`") {
			t.Errorf("the cause must be named:\n%s", got)
		}
		if !strings.Contains(got, "Settings → Moons (Owner only)") {
			t.Errorf("the remedy must be named, with its role floor:\n%s", got)
		}
	})
}

// TestCalendarConfig_UnreadableStoredCountIsNeverZero. Zero rows IS the finding
// this diagnostic reports, so a failed read that rendered as zero would be the
// most damaging possible lie in this file.
func TestCalendarConfig_UnreadableStoredCountIsNeverZero(t *testing.T) {
	f := twoCalendarCampaign()
	f.All[1].StoredCountNote = "the stored-row read failed: connection refused"
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1")
		if !strings.Contains(got, "stored count UNKNOWN") || !strings.Contains(got, "connection refused") {
			t.Errorf("a failed read must render as UNKNOWN with its reason:\n%s", got)
		}
		if strings.Contains(got, "moons: 0 stored row(s)") {
			t.Errorf("a failed read must NOT render as zero rows:\n%s", got)
		}
	})
}

// TestCalendarConfig_ZeroMonthsIsFlagged: without months nothing can resolve a
// date, and the fallback Moon has nowhere to anchor.
func TestCalendarConfig_ZeroMonthsIsFlagged(t *testing.T) {
	f := CampaignCalendarFacts{
		Found: true, CampaignID: "c1", CampaignName: "T", AddonEnabled: boolPtr(true), SpineInstalled: true,
		All: []DiagCalendar{{ID: "x", Name: "Broken", Mode: "reallife"}},
	}
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1")
		if !strings.Contains(got, "**zero months**") {
			t.Errorf("a month-less calendar must be flagged:\n%s", got)
		}
	})
}

// TestCalendarConfig_FiltersToOneCalendar covers the optional `:calId` tail,
// including the case where it names nothing.
func TestCalendarConfig_FiltersToOneCalendar(t *testing.T) {
	withCampaignProvider(t, &fakeCampaignProvider{cal: twoCalendarCampaign()}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1:rw")
		if strings.Contains(got, "### Harptos") {
			t.Errorf("the filter must exclude the other calendar:\n%s", got)
		}
		if !strings.Contains(got, "### Earth") {
			t.Errorf("the named calendar must be shown:\n%s", got)
		}
		miss, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1:nope")
		if !strings.Contains(miss, "No calendar `nope`") {
			t.Errorf("an unknown calendar id must say so:\n%s", miss)
		}
	})
}

// TestCalendarConfig_NoCalendarsIsAStatedState, not an empty section.
func TestCalendarConfig_NoCalendarsIsAStatedState(t *testing.T) {
	f := CampaignCalendarFacts{Found: true, CampaignID: "c1", CampaignName: "T", AddonEnabled: boolPtr(true)}
	withCampaignProvider(t, &fakeCampaignProvider{cal: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.config", "c1")
		if !strings.Contains(got, "NO calendars") {
			t.Errorf("zero calendars must be stated:\n%s", got)
		}
	})
}

// ── campaign.surfaces ───────────────────────────────────────────────────────

// calHandler spells a handler name the way Echo records a method value.
func calHandler(name string) string {
	return "github.com/keyxmakerx/chronicle/internal/plugins/calendar.(*Handler)." + name + "-fm"
}

// liveCalendarRoutes is a stand-in route table: every declared row, plus the
// two frozen-shell routes that campaign.surfaces DISCOVERS by handler instead
// of declaring by path.
func liveCalendarRoutes() []RouteFact {
	var out []RouteFact
	for _, row := range calendarSurfaceMap() {
		out = append(out, RouteFact{Method: "GET", Path: row.Path, Handler: calHandler(row.Handler)})
	}
	return append(out,
		RouteFact{Method: "GET", Path: "/campaigns/:id/calendar/" + "v2", Handler: calHandler("ShowV2")},
		RouteFact{Method: "GET", Path: "/campaigns/:id/calendar/" + "v2/:calId/settings/:resource", Handler: calHandler("ShowV2SubresourceSettings")},
	)
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
			"**LEGACY-REDIRECT** `/campaigns/:id/calendar`",
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

// TestCampaignSurfaces_FrozenShellIsDiscoveredNotDeclared.
//
// [VS-2] SIGNED sunset the V2 shell as a clickable destination and preserved it
// as a URL, and TestSunset_NoLiveDoorRemains enforces that by walking internal/
// for the shell's path prefix. So campaign.surfaces classifies it by the
// HANDLER the router reports and prints the router's own path — which keeps the
// answer complete without this repo writing the path down, and makes the row
// disappear on its own if the route is ever actually removed.
func TestCampaignSurfaces_FrozenShellIsDiscoveredNotDeclared(t *testing.T) {
	// No declared row may carry the shell's path: that is what the sunset guard
	// forbids, and stating it here means a later edit that re-adds one fails in
	// THIS package too, with the reason attached.
	shellPrefix := "/calendar/" + "v2"
	for _, row := range calendarSurfaceMap() {
		if strings.Contains(row.Path, shellPrefix) {
			t.Errorf("%s is declared by path; the frozen shell must be discovered by handler ([VS-2])", row.Path)
		}
	}
	if len(handlerSurfaces()) == 0 {
		t.Fatal("no handler-keyed surfaces — the discovery half would be silently empty")
	}

	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(liveCalendarRoutes())}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "DISCOVERED from the live table") {
			t.Errorf("the discovered section must appear:\n%s", got)
		}
		if !strings.Contains(got, "**LEGACY-PRESERVED** `/campaigns/:id"+shellPrefix+"`") {
			t.Errorf("the shell must be printed with its live path and its status:\n%s", got)
		}
		if !strings.Contains(got, "NO LIVE LINK REACHES THIS") {
			t.Errorf("the shell's row must say why it is still registered:\n%s", got)
		}
	})

	// And when the route is gone, so is the row — with no edit here.
	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(nil)}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if strings.Contains(got, "DISCOVERED from the live table") {
			t.Errorf("with no routes there is nothing to discover:\n%s", got)
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

// TestCampaignSurfaces_HandlerDisagreementPrefersTheLiveTable.
func TestCampaignSurfaces_HandlerDisagreementPrefersTheLiveTable(t *testing.T) {
	routes := liveCalendarRoutes()
	routes[0].Handler = "github.com/keyxmakerx/chronicle/internal/plugins/calendar.(*Handler).SomethingElse-fm"
	withCampaignProvider(t, &fakeCampaignProvider{surf: surfaceFactsWith(routes)}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "Trust the live handler") {
			t.Errorf("a handler disagreement must be flagged and resolved in favour of the binary:\n%s", got)
		}
		if !strings.Contains(got, "SomethingElse") {
			t.Errorf("the live handler must be named:\n%s", got)
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

// TestCampaignSurfaces_DisabledAddonMakesEveryRouteUnreachable.
func TestCampaignSurfaces_DisabledAddonMakesEveryRouteUnreachable(t *testing.T) {
	f := surfaceFactsWith(liveCalendarRoutes())
	f.CalendarAddonEnabled = boolPtr(false)
	withCampaignProvider(t, &fakeCampaignProvider{surf: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "campaign.surfaces", "c1")
		if !strings.Contains(got, "A registered route is not a reachable one") {
			t.Errorf("the addon gate must be stated above the table:\n%s", got)
		}
	})
}

// TestCampaignSurfaces_HandAuthoredSidebarLinkIsShown. A `type:"link"` item can
// point at a retired surface, which is one of the two ways the V2 shell is still
// reachable.
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
// reason rank 4 exists — and the answer has to disambiguate the two things
// called "skybox", because the operator uses one word for both.
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
		// CALV5 rewording: the distinction between the two things nicknamed
		// "skybox" survives (that is this test's whole point), but the text
		// now also says the widget's engine was deleted and the placement
		// renders the rebuilding notice — a diagnostic asserting the old
		// "genuinely DOES render the Moon" claim would be lying to the
		// operator about a dead pipeline.
		if !strings.Contains(got, "LEGACY skybox widget") || !strings.Contains(got, "distinct from the v4 sky band") {
			t.Errorf("the two things called skybox must be distinguished:\n%s", got)
		}
		if !strings.Contains(got, "rebuilding notice") {
			t.Errorf("the placement must say what it renders TODAY (the rebuilding notice), "+
				"not what the deleted engine used to render:\n%s", got)
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
		if !strings.Contains(got, "removes every calendar route at the middleware") {
			t.Errorf("the consequence of a disabled calendar addon must be stated:\n%s", got)
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

// ── the mirrored rules, as pure functions ───────────────────────────────────

// TestMirrorBenchClassify covers the classification table directly. The source
// pin proves the producer's rule has not moved; this proves the copy is right.
func TestMirrorBenchClassify(t *testing.T) {
	fantasy := func(id string, def bool) DiagCalendar {
		return DiagCalendar{ID: id, Mode: "fantasy", IsDefault: def}
	}
	real := func(id string, def bool) DiagCalendar {
		return DiagCalendar{ID: id, Mode: "reallife", IsDefault: def}
	}
	tests := []struct {
		name          string
		cals          []DiagCalendar
		active        string
		wantPrimary   string
		wantRealWorld string
		wantClause    string
	}{
		{"default in-world wins", []DiagCalendar{real("r", false), fantasy("f", true)}, "", "f", "r", "campaign default AND in-world"},
		{"active in-world when no default", []DiagCalendar{fantasy("a", false), fantasy("b", false)}, "b", "b", "", "ACTIVE calendar"},
		{"first in-world otherwise", []DiagCalendar{real("r", false), fantasy("a", false), fantasy("b", false)}, "", "a", "r", "first in-world calendar"},
		{"real-world-only campaign promotes the default", []DiagCalendar{real("r1", false), real("r2", true)}, "", "r2", "r1", "NOTHING BUT REAL-WORLD"},
		{"single real-world calendar has no second seat", []DiagCalendar{real("r", false)}, "", "r", "", "first calendar in the list"},
		{"an active REAL-WORLD calendar does not win the primary seat", []DiagCalendar{fantasy("f", false), real("r", false)}, "r", "f", "r", "first in-world calendar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seats := mirrorBenchClassify(tc.cals, tc.active)
			if len(seats) == 0 {
				t.Fatal("no seats")
			}
			if seats[0].Seat != seatPrimary || seats[0].Cal.ID != tc.wantPrimary {
				t.Errorf("primary: want %s, got %s (%s)", tc.wantPrimary, seats[0].Cal.ID, seats[0].Seat)
			}
			if !strings.Contains(seats[0].Clause, tc.wantClause) {
				t.Errorf("clause: want it to mention %q, got %q", tc.wantClause, seats[0].Clause)
			}
			gotRW := ""
			for _, s := range seats {
				if s.Seat == seatRealWorld {
					gotRW = s.Cal.ID
				}
			}
			if gotRW != tc.wantRealWorld {
				t.Errorf("real-world seat: want %q, got %q", tc.wantRealWorld, gotRW)
			}
			if len(seats) != len(tc.cals) {
				t.Errorf("every calendar must get a seat: %d calendars, %d seats", len(tc.cals), len(seats))
			}
		})
	}
	if seats := mirrorBenchClassify(nil, ""); seats != nil {
		t.Errorf("no calendars must produce no seats, got %v", seats)
	}
}

// TestMirrorSeatRenderAndAlmanacGate pins the [SKY-1] seat and the gate it
// feeds, including the case that surprises people: the real-world Block does not
// build an Almanac EVEN WITH moons.
func TestMirrorSeatRenderAndAlmanacGate(t *testing.T) {
	skyOn, shelfHidden := mirrorSeatRender(seatPrimary)
	if !skyOn || shelfHidden {
		t.Errorf("PRIMARY: want sky on, shelf shown; got sky=%t shelfHidden=%t", skyOn, shelfHidden)
	}
	if !mirrorAlmanacBuilt(skyOn, shelfHidden, 1) {
		t.Error("PRIMARY with a moon must build the Almanac")
	}
	if mirrorAlmanacBuilt(skyOn, shelfHidden, 0) {
		t.Error("no moons means no register, on any seat")
	}

	skyOn, shelfHidden = mirrorSeatRender(seatRealWorld)
	if skyOn || !shelfHidden {
		t.Errorf("REAL-WORLD: want no sky, shelf hidden; got sky=%t shelfHidden=%t", skyOn, shelfHidden)
	}
	if mirrorAlmanacBuilt(skyOn, shelfHidden, 3) {
		t.Error("REAL-WORLD fails both halves of the gate, so three moons still build no register — this is [SKY-1] plus [SKY-7], not a bug")
	}
}

// TestMirrorResolveBenchSections covers the three-way nil / empty / list split.
func TestMirrorResolveBenchSections(t *testing.T) {
	all := mirrorResolveBenchSections(nil, true)
	for _, k := range benchSectionKeysMirror {
		if !all[k] {
			t.Errorf("never-chosen must close %q ([BR2-4])", k)
		}
	}
	none := mirrorResolveBenchSections([]string{}, false)
	for _, k := range benchSectionKeysMirror {
		if none[k] {
			t.Errorf("an empty stored list means closed NOTHING; %q should be open", k)
		}
	}
	some := mirrorResolveBenchSections([]string{"rsvp", "bogus"}, false)
	if !some["rsvp"] || some["rows"] {
		t.Errorf("stored [rsvp] should close rsvp only, got %v", some)
	}
	if some["bogus"] {
		t.Error("an unknown stored key must resolve to nothing and take nothing down with it")
	}
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
	if idx["campaigns.list"] > idx["calendar.render"] {
		t.Error("campaigns.list supplies the argument these four need and must come first")
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
	}
	// The optional tail must survive substitution, and its absence must not
	// leave a dangling colon behind.
	if got := WithCampaign("calendar.render", "<campaignId>:u1", "c1"); got != "c1:u1" {
		t.Errorf("substitution must keep the user id: got %q", got)
	}
	if got := WithCampaign("calendar.render", "<campaignId>", "c1"); got != "c1" {
		t.Errorf("a bare campaign id is a complete argument here: got %q", got)
	}
	if got := WithCampaign("campaign.config", "<campaignId>", "c1"); got != "c1" {
		t.Errorf("campaign.config takes the campaign and nothing else: got %q", got)
	}
}

// TestDeployCheckPointsAtTheRenderTrace is the catalog-hygiene fix. Its own file
// header records an incident in which "a label was read as evidence"; a marker
// hit read as "the feature works" is the same mistake one layer up, and it is
// the most likely wrong answer an assistant would give to "is RSVP on my
// calendar?".
func TestDeployCheckPointsAtTheRenderTrace(t *testing.T) {
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
	if !strings.Contains(desc, "calendar.render") {
		t.Errorf("the Desc must name the diagnostic that DOES answer it:\n%s", desc)
	}

	out := renderHostDeployCheckFrom(deployCheckSources{}, "some-marker")
	if !strings.Contains(out, "IT PROVES NOTHING ABOUT WHETHER IT RENDERS") {
		t.Errorf("the marker section itself must carry the caveat, where somebody is looking at a tick:\n%s", out)
	}
	if !strings.Contains(out, "calendar.render") || !strings.Contains(out, "campaign.config") {
		t.Errorf("the marker section must name the diagnostics that answer the render question:\n%s", out)
	}
}
