// app_dashboard_w5b_test.go — C-CAL-DASHBOARD-W5b: the per-calendar permissions
// write path. Validates persistence, level/rules validation, bulk-replace, and
// (at the gate level) that non-owners cannot reach the route.
package calendar

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestUpdateCalendarVisibility_PersistsLevelAndRules(t *testing.T) {
	var gotCal, gotVis string
	var gotRules *string
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Visibility: "everyone"}, nil
		},
		updateCalVisFn: func(_ context.Context, calID, vis string, rules *string) error {
			gotCal, gotVis, gotRules = calID, vis, rules
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	rules := `{"allowed_users":["u1","u2"]}`
	err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{
		Visibility: "everyone", VisibilityRules: &rules,
	})
	if err != nil {
		t.Fatalf("UpdateCalendarVisibility: %v", err)
	}
	if gotCal != "cal-1" || gotVis != "everyone" {
		t.Errorf("repo got (%q,%q); want (cal-1,everyone)", gotCal, gotVis)
	}
	if gotRules == nil || *gotRules != rules {
		t.Errorf("rules not bulk-replaced verbatim; got %v want %q", gotRules, rules)
	}
}

func TestUpdateCalendarVisibility_BulkReplaceClearsRules(t *testing.T) {
	var gotRules *string
	called := false
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Visibility: "everyone"}, nil
		},
		updateCalVisFn: func(_ context.Context, _, _ string, rules *string) error {
			called, gotRules = true, rules
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Switching to GM-only with no rules must write nil rules (bulk-replace),
	// not merge with any prior rule set.
	if err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{
		Visibility: "dm_only", VisibilityRules: nil,
	}); err != nil {
		t.Fatalf("UpdateCalendarVisibility: %v", err)
	}
	if !called {
		t.Fatal("repo write not called")
	}
	if gotRules != nil {
		t.Errorf("rules should be cleared to nil on bulk-replace; got %v", *gotRules)
	}
}

func TestUpdateCalendarVisibility_Validation(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1"}, nil
		},
		updateCalVisFn: func(_ context.Context, _, _ string, _ *string) error {
			t.Fatal("repo write must not be reached on a validation failure")
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Bad base level.
	if err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{Visibility: "bogus"}); err == nil {
		t.Error("invalid visibility level should error")
	}
	// Bad rules JSON.
	bad := `{not json`
	if err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{Visibility: "everyone", VisibilityRules: &bad}); err == nil {
		t.Error("invalid visibility_rules JSON should error")
	}
}

func TestUpdateCalendarVisibility_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return nil, nil },
	}
	svc := newTestCalendarService(repo)
	err := svc.UpdateCalendarVisibility(context.Background(), "missing", UpdateCalendarVisibilityInput{Visibility: "everyone"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("missing calendar should be NotFound; got %v", err)
	}
}

func TestCalVisModeForCard(t *testing.T) {
	rules := `{"allowed_users":["u1"]}`
	cases := []struct {
		cal  Calendar
		want string
	}{
		{Calendar{Visibility: "everyone"}, "public"},
		{Calendar{Visibility: "dm_only"}, "gmonly"},
		{Calendar{Visibility: "everyone", VisibilityRules: &rules}, "specific"},
		{Calendar{Visibility: "dm_only", VisibilityRules: &rules}, "gmonly"}, // dm_only wins
	}
	for _, c := range cases {
		if got := calVisModeForCard(c.cal); got != c.want {
			t.Errorf("calVisModeForCard(%+v) = %q; want %q", c.cal, got, c.want)
		}
	}
}

// benchW5bData is a one-calendar Bench whose single calendar is dm_only, so the
// permissions trigger's seeded state is exercised end to end.
//
// PIN REFRESH (C-CALV4-BENCH-P4): the route now renders the Bench, so these
// drive BenchPage. The W5b guarantee is unchanged and so is every marker —
// the Bench moved the per-calendar trigger from the card onto the row and the
// Block's management strip, and left the modal and its driver untouched.
func benchW5bData(isOwner bool) BenchData {
	cal := Calendar{ID: "c1", CampaignID: "camp-1", Name: "Secret", Visibility: "dm_only"}
	return BenchData{
		CampaignID: "camp-1", CampaignName: "Imix",
		IsGM: isOwner, IsOwner: isOwner, ShowNewSlot: isOwner,
		CalendarCount: 1,
		Rsvp:          benchRsvpPanel(),
		Rows:          benchRows([]*Calendar{&cal}, "", "camp-1", isOwner),
	}
}

// TestDashboard_OwnerGetsPermissionsEditor: the owner page ships the modal +
// the reused chip-row editor (with the GM-only mode), and the per-calendar
// trigger carries its current visibility state for the editor to seed.
//
// THE DRIVER SCRIPT IS NO LONGER ASSERTED HERE because the page no longer mounts
// it. It sat inside <main id="main-content">, the region the sidebar's
// hx-boost/hx-select swaps, and htmx removes script tags from a swapped fragment
// (allowScriptTags=false) — so the Permissions button worked on a direct load
// and was dead when the Bench was reached through the sidebar. The driver moved
// to the plugin body-script registry; its presence AND its ordering behind
// cal_visibility.js are pinned by
// TestPermissionsDriverIsNeverMountedWithoutItsDependency below. What this test
// owns — the owner-only DOM, which is the actual permission gate — is unchanged.
func TestDashboard_OwnerGetsPermissionsEditor(t *testing.T) {
	html := renderBench(t, benchW5bData(true))
	for _, want := range []string{
		"cal-permissions-modal",      // the modal
		"data-visibility-editor",     // the reused Q-V2-7 widget
		`value="gmonly"`,             // the W5b GM-only mode
		`data-cal-vis-mode="gmonly"`, // the trigger seeds the editor with current state
		`data-cal-visibility="dm_only"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("owner bench missing %q", want)
		}
	}
}

// TestDashboard_PlayerNoPermissionsEditor: players never receive the editor DOM,
// the driver, or the per-calendar Permissions trigger.
func TestDashboard_PlayerNoPermissionsEditor(t *testing.T) {
	html := renderBench(t, benchW5bData(false))
	for _, forbidden := range []string{"cal-permissions-modal", "data-cal-permissions", "calendar_permissions.js", "data-visibility-editor"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("player must NOT receive %q", forbidden)
		}
	}
}

// TestCalendarVisibilityRouteGate documents the route gate (non-owner forbidden):
// the PUT uses CanControlWorldState — owner OR co-DM only, players excluded.
func TestCalendarVisibilityRouteGate(t *testing.T) {
	player := &campaigns.CampaignContext{MemberRole: campaigns.RolePlayer}
	owner := &campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}
	coDM := &campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsDmGranted: true}
	if player.CanControlWorldState() {
		t.Error("a player must NOT pass the calendar-visibility gate")
	}
	if !owner.CanControlWorldState() || !coDM.CanControlWorldState() {
		t.Error("owner and co-DM must pass the calendar-visibility gate")
	}
}

// --- C-CALV4-DAYCARD/R2-2a, PERM-JS-HARD-DEP-2 ------------------------------
//
// THE ANCHOR MOVED, AND IT IS WORTH SAYING SO. The finding cited
// app_dashboard_w5b_test.go:152 as the line pinning the orphan mount. That line
// lives in TestDashboard_OwnerGetsPermissionsEditor, which renders the BENCH
// (renderBench), not the card-grid page — and the Bench mount is correct: it
// ships cal_visibility.js first and calendar_permissions.js after it. So that
// assertion STAYS; weakening it would retire a live, correct guarantee to make
// a finding pass. The mount that had no cal_visibility.js beside it was
// app_dashboard.templ's, and its absence is pinned here, against the page that
// actually renders it.

// TestDashboardPage_DoesNotMountThePermissionsDriverAlone: the card-grid page
// no longer mounts calendar_permissions.js. The driver has a hard dependency on
// window.ChronicleCalVisibility and no fallback, so a mount without
// cal_visibility.js beside it wires nothing — a dead modal waiting for the page
// to be re-routed or copied.
func TestDashboardPage_DoesNotMountThePermissionsDriverAlone(t *testing.T) {
	html := renderDashboardPage(t, sampleDashboardData())
	if strings.Contains(html, "calendar_permissions.js") {
		t.Error("the card-grid page mounts calendar_permissions.js without cal_visibility.js — " +
			"the driver's init() returns immediately and the modal never wires")
	}
}

// TestPermissionsDriverIsNeverMountedWithoutItsDependency is the durable form of
// the same rule, over SOURCE rather than over one render: any file that mounts
// calendar_permissions.js must also mount cal_visibility.js, and must mount it
// FIRST — the driver reads window.ChronicleCalVisibility as it executes
// (calendar_permissions.js:22, and init() bails on a nil V), so order is part of
// the contract, not a detail.
//
// THE SEARCH SET NOW INCLUDES THE PLUGIN BODY-SCRIPT REGISTRY, and it has to.
// The Bench's mount used to be a <script> in bench.templ; that tag lived inside
// <main id="main-content">, which the sidebar's hx-boost/hx-select swaps, and
// htmx DELETES script tags from a swapped fragment when allowScriptTags is false
// (boot.js). The Bench therefore wired on a direct load and silently did not
// when reached through the sidebar. The mount moved to
// internal/app/routes.go's pluginBodyScripts, which base.templ emits after
// {children...} — outside the swapped region. Order there means the same thing
// it meant in a templ (base.templ emits the slice in order, deferred, so
// execution follows document order), so the same rule applies to the same
// strings and this guard simply reads one more file.
//
// It is a coverage WIDENING, not an amendment to a signed guard: it forbids
// something no surface should ever have done, and it pins the fix against the
// copy-paste that would otherwise reintroduce it.
func TestPermissionsDriverIsNeverMountedWithoutItsDependency(t *testing.T) {
	files, err := filepath.Glob("*.templ")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no .templ files found — the guard would pass vacuously")
	}
	// The registry is where the Bench's drivers live now; a guard that only read
	// templs would have gone quiet the moment they moved there.
	const registry = "../../app/routes.go"
	if _, err := os.Stat(registry); err != nil {
		t.Fatalf("the plugin body-script registry is not at %s any more (%v) — "+
			"this guard would stop reading the file that actually mounts the driver", registry, err)
	}
	files = append(files, registry)
	checked := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		driver := strings.Index(string(src), "calendar_permissions.js\"")
		if driver < 0 {
			continue
		}
		checked++
		dep := strings.Index(string(src), "cal_visibility.js\"")
		if dep < 0 {
			t.Errorf("%s mounts calendar_permissions.js with no cal_visibility.js anywhere in the file: "+
				"the driver's init() returns immediately and the modal silently never wires", f)
			continue
		}
		if dep > driver {
			t.Errorf("%s mounts cal_visibility.js AFTER calendar_permissions.js: the driver reads "+
				"window.ChronicleCalVisibility as it parses, so the dependency must come first", f)
		}
	}
	if checked == 0 {
		t.Error("nothing mounts calendar_permissions.js — neither a template nor the plugin " +
			"body-script registry. The driver is orphaned entirely, or the mount string changed " +
			"and this guard stopped reading anything")
	}
}

// TestBenchDriversShipFromThePluginBodyScriptRegistry pins WHERE the Bench's
// three drivers are mounted, which is the whole of the boosted-navigation fix.
//
// A <script src> in a page templ is inside <main id="main-content">; the sidebar
// navigates with hx-boost + hx-select="#main-content"; boot.js sets
// htmx.config.allowScriptTags=false, and htmx's makeFragment then REMOVES every
// script tag in the swapped fragment. The consequence was not a console error —
// it was a Bench that looked exactly right (the stylesheets in the same region
// survive) with a dead day card and a dead Permissions button, but only when
// reached through the sidebar.
//
// The registry (internal/app/routes.go → layouts.SetPluginBodyScripts →
// base.templ) emits after {children...}, outside the swapped region, so both
// navigation paths deliver the same scripts. This test reads the source because
// pluginBodyScripts is a local built during startup wiring: there is no exported
// value to assert against, and a render-level assertion cannot see it either
// (the page templs render with an empty context in these tests, by design).
func TestBenchDriversShipFromThePluginBodyScriptRegistry(t *testing.T) {
	src, err := os.ReadFile("../../app/routes.go")
	if err != nil {
		t.Fatalf("read the plugin body-script registry: %v", err)
	}
	s := string(src)
	// Ordered, because base.templ emits the slice in order with `defer` and
	// deferred scripts execute in document order.
	want := []string{
		`/js/cal_visibility.js"`,
		`/js/calendar_permissions.js"`,
		`/js/calendar_daycard.js"`,
	}
	prev := -1
	for _, w := range want {
		at := strings.Index(s, w)
		if at < 0 {
			t.Errorf("the plugin body-script registry does not mount %s — a Bench reached through "+
				"the sidebar would render it with that driver stripped", strings.Trim(w, `"`))
			continue
		}
		if at < prev {
			t.Errorf("%s is registered out of order: the registry is emitted in slice order and "+
				"deferred, so cal_visibility.js must precede calendar_permissions.js", strings.Trim(w, `"`))
		}
		prev = at
	}
}
