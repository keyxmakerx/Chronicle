// bench_anonymous_test.go — C-CALV4-V2SUNSET R2-4 (the V2 sunset), stage 3 of
// the dispatch, committed FIRST per [VS-10] SIGNED as option (b).
//
// THIS FILE IS THE SECURITY REVIEW OF THE ANONYMOUS-PUBLIC ROUTE MOVE.
// [VS-5] SIGNED rules that the review "IS the assertion list, and it ships as
// TESTS rather than as prose" — a security paragraph nobody can run is a
// paragraph. Nineteen items are ruled; the reachability half (items 1-7) lives
// in routes_test.go beside the anonymous matrix it extends, and the render half
// (items 8-19) lives here, because it needs a real campaign with real rows.
//
// WHY A REAL DATABASE. Every "must NOT contain" item is a claim about what the
// PRODUCER put in the payload, not about which template branch ran. A fake
// service can only return what the test already decided to return, so it would
// assert the fixture rather than the filter. The dm_only event, the
// restricted-audience event and the hidden calendar in this fixture are real
// rows behind the real visibility SQL, and the anonymous viewer's absence of
// them is the service layer's answer, not the stub's.
//
// THE OTHER HALF OF EVERY ABSENCE ASSERTION IS A PRESENCE ASSERTION. A guard
// that passes because the whole page is broken proves nothing ([VS-5] item 17
// says so in terms about PersistURL), so each item that asserts an absence for
// the anonymous viewer asserts the same thing PRESENT for a Player or an Owner
// on the same fixture, in the same test.
package calendar

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- the fixture ------------------------------------------------------------

// anonFixtureRoster is the party the RSVP panel would print. Display names are
// the thing item 8 is about: they are the campaign's members, and a logged-out
// stranger is not entitled to them.
type anonFixtureRoster struct{}

func (anonFixtureRoster) BenchRoster(_ context.Context, _ string) ([]BenchRosterMember, error) {
	return []BenchRosterMember{
		{UserID: "u-owner", Name: "Kaelthorn Vex", Role: "Owner", IsOwner: true, TZ: "Europe/London"},
		{UserID: "u-player", Name: "Brynwyth Ashe", Role: "Player", TZ: "America/Denver"},
	}, nil
}

func (anonFixtureRoster) BenchAvailability(_ context.Context, _, _, _ string,
	includeDetail bool) (*BenchAvailability, error) {
	// includeDetail IS the permission (bench.go's own comment). The lane data is
	// only ever returned when the caller asked for detail, so a leak here would
	// be the caller's, which is exactly what item 8 checks.
	av := &BenchAvailability{}
	if includeDetail {
		av.FreeDays = map[string][]bool{"u-player": {false, false, true}}
	}
	return av, nil
}

// anonFixture seeds a PUBLIC campaign with one visible calendar carrying three
// events — public, dm_only and restricted-to-one-user — and one calendar the
// viewer may not see at all. It returns the campaign id.
func anonFixture(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx := context.Background()
	campaignID, cal := calTestSeedNavCalendar(t, db)
	repo := NewCalendarRepository(db)

	rules := `{"allowed_users":["u-player"]}`
	for _, e := range []*Event{
		{CalendarID: cal.ID, Name: "The Sealed Vault", Year: 1523, Month: 1, Day: 16,
			Visibility: storageVisibilityDMOnly},
		{CalendarID: cal.ID, Name: "Brynwyths Private Word", Year: 1523, Month: 1, Day: 17,
			Visibility: storageVisibilityEveryone, VisibilityRules: &rules},
	} {
		e.ID = calTestID(t)
		if err := repo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("create event %q: %v", e.Name, err)
		}
	}

	// The calendar nobody below Owner may see (per-calendar visibility, W5a).
	//
	// The visibility column is NOT part of the Create INSERT (repository.go:248
	// lists its columns and visibility is not among them), so it takes the
	// schema default and the row must be re-marked through the writer the
	// product itself uses. A fixture that only SET the struct field would have
	// seeded a public calendar and this whole assertion would have passed by
	// testing nothing.
	hidden := &Calendar{
		ID: calTestID(t), CampaignID: campaignID, Name: "The Hidden Reckoning",
		Mode: ModeFantasy, CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	if err := repo.Create(ctx, hidden); err != nil {
		t.Fatalf("create hidden calendar: %v", err)
	}
	if err := repo.UpdateCalendarVisibility(ctx, hidden.ID, storageVisibilityDMOnly, nil); err != nil {
		t.Fatalf("hide the hidden calendar: %v", err)
	}
	return campaignID
}

// anonRender drives the REAL handler for one identity and returns the response
// body. role is the campaign MemberRole; userID is "" for an anonymous request,
// which is precisely the empty key [VS-15] rules on.
func anonRender(t *testing.T, h *Handler, campaignID string, role campaigns.Role,
	anonymous bool, userID string) string {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/apps/calendar", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(campaignID)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: campaignID, Name: "Imix", IsPublic: true},
		MemberRole:  role,
		IsMember:    !anonymous && role >= campaigns.RolePlayer,
		IsAnonymous: anonymous,
	})
	if userID != "" {
		c.Set("auth_user_id", userID)
	}
	if err := h.AppDashboard(c); err != nil {
		t.Fatalf("AppDashboard for role=%d anonymous=%v: %v", role, anonymous, err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("AppDashboard for role=%d anonymous=%v: status=%d want 200", role, anonymous, rec.Code)
	}
	return rec.Body.String()
}

// anonHandler wires the Bench's full read stack against the scratch schema.
func anonHandler(t *testing.T, db *sql.DB) *Handler {
	t.Helper()
	prev := BlockSpine()
	t.Cleanup(func() { blockSpine.Store(prev) })
	InstallBlockSpine(NewBlockService(NewBlockRepository(db)))
	h := NewHandler(NewCalendarService(NewCalendarRepository(db)))
	h.SetScheduleReader(anonFixtureRoster{})
	return h
}

// anonBenchRegion narrows a rendered page to the Bench element itself
// (`data-cal-bench`, bench.templ:78) so a write-control sweep judges THIS
// surface rather than the app layout's chrome.
func anonBenchRegion(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, "data-cal-bench")
	if start < 0 {
		t.Fatal("no data-cal-bench root in the rendered page — has the Bench's root marker been renamed?")
	}
	rest := page[start:]
	if end := strings.Index(rest, "data-bench-caption"); end >= 0 {
		return rest[:end]
	}
	return rest
}

// --- the assertion set ------------------------------------------------------

// TestAnonymousBench_RenderCarriesNoPrivateSurface is [VS-5] items 8-14 and
// 17-18: what an anonymous render must NOT contain, each paired with the same
// thing PRESENT for a viewer who is entitled to it.
func TestAnonymousBench_RenderCarriesNoPrivateSurface(t *testing.T) {
	if testing.Short() {
		t.Skip("the anonymous Bench assertion set requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	campaignID := anonFixture(t, db)
	h := anonHandler(t, db)

	anon := anonRender(t, h, campaignID, campaigns.RoleNone, true, "")
	player := anonRender(t, h, campaignID, campaigns.RolePlayer, false, "u-player")
	owner := anonRender(t, h, campaignID, campaigns.RoleOwner, false, "u-owner")

	// The page must actually have rendered, or every absence below is vacuous.
	if !strings.Contains(anon, "data-bench-block") {
		t.Fatal("the anonymous render carries no Bench at all — every absence assertion below would pass for the wrong reason")
	}

	// ITEM 8 — no member names, no member identifiers, no per-member lane.
	// The absence is in the DATA: benchRsvpResolve never reads the roster below
	// RolePlayer, so this cannot be restored by a template edit.
	for _, name := range []string{"Kaelthorn Vex", "Brynwyth Ashe", "u-owner", "u-player"} {
		if strings.Contains(anon, name) {
			t.Errorf("item 8: the anonymous render leaks the party member %q", name)
		}
	}
	for _, present := range []string{"Kaelthorn Vex", "Brynwyth Ashe"} {
		if !strings.Contains(player, present) {
			t.Errorf("item 8 (the other half): a PLAYER lost %q — the panel's signed audience "+
				"law is every member at every role, and a guard that also breaks the party's own "+
				"view proves nothing", present)
		}
	}

	// ITEM 9 — no `needs backend` CHIP.
	//
	// MEASURED CORRECTION, stated rather than assumed: the literal STRING
	// "needs backend" also appears in the Bench's standing caption
	// (bench.templ:957), which renders identically for a Player and is not
	// gated by anything. [VS-5] item 9 as drafted ("no `needs backend` string")
	// therefore fails on prose that has nothing to do with anonymity. The
	// ruling it cites (decisions/2026-07-27-needs-backend-audience.md) is about
	// the CHIP — "chip on GM and owner tiers, omit on player" — so the chip is
	// what is asserted, and the caption is reported as a finding.
	if strings.Contains(anon, `class="badge need"`) {
		t.Error("item 9: a `needs backend` chip reached an anonymous viewer — it is GM-tier " +
			"(decisions/2026-07-27-needs-backend-audience.md)")
	}

	// ITEM 10 — no day-card editor scaffold.
	for _, scaffold := range []string{"data-dc-can-edit", "data-dc-create", "dc-editor"} {
		if strings.Contains(anon, scaffold) {
			t.Errorf("item 10: the anonymous render carries the day-card editor scaffold %q", scaffold)
		}
	}

	// ITEM 11 — no permissions modal, no calendar_permissions.js mount.
	for _, owned := range []string{"calendar_permissions.js", "calendarPermissionsModal", "cal-perm-modal"} {
		if strings.Contains(anon, owned) {
			t.Errorf("item 11: the anonymous render carries the owner-gated %q", owned)
		}
	}

	// ITEM 12 / 15 — no write control of any kind: no form, no CSRF token, no
	// hx-post/put/delete. The surface is a single page render and stays one.
	//
	// SCOPED TO THE BENCH ITSELF, and the scope is measured rather than
	// convenient: the app layout ships a global `<form id="reauth-form">` inside
	// the reauth modal on EVERY page, anonymous or not, and this slice neither
	// added it nor may remove it. Asserting `<form` page-wide would fail on the
	// chrome and teach the next reader to delete the assertion.
	benchOnly := anonBenchRegion(t, anon)
	for _, write := range []string{"hx-post", "hx-put", "hx-delete", "csrf", "<form", `method="post"`} {
		if strings.Contains(strings.ToLower(benchOnly), write) {
			t.Errorf("item 12/15: the anonymous Bench carries the write control %q", write)
		}
	}

	// ITEM 13 — no dm_only and no restricted event, in the rendered Blocks OR in
	// the day-card payload. The payload is the dangerous one: it is a JSON blob
	// in an attribute, and a template branch cannot un-leak it.
	for _, secret := range []string{"The Sealed Vault", "Brynwyths Private Word"} {
		if strings.Contains(anon, secret) {
			t.Errorf("item 13: the anonymous render leaks the restricted event %q", secret)
		}
	}
	if !strings.Contains(owner, "The Sealed Vault") {
		t.Error("item 13 (the other half): the OWNER lost the dm_only event — the fixture is not " +
			"exercising the visibility filter at all")
	}
	if !strings.Contains(player, "Brynwyths Private Word") {
		t.Error("item 13 (the other half): the allowed player lost their own restricted event")
	}

	// ITEM 14 — no calendar the viewer may not see. Hidden and missing are
	// byte-identical: the anonymous render must not so much as name it.
	if strings.Contains(anon, "The Hidden Reckoning") {
		t.Error("item 14: the anonymous render names a calendar the viewer may not see")
	}
	if !strings.Contains(owner, "The Hidden Reckoning") {
		t.Error("item 14 (the other half): the OWNER lost the hidden calendar — per-calendar " +
			"visibility is not being exercised")
	}

	// ITEM 17 — no disclosure-register persist. bench.templ:255 renders
	// hx-post={ d.PersistURL } on every <details> when PersistURL != "". For an
	// anonymous viewer it must be empty, or every section toggle fires an
	// unauthenticated POST at a `cg` route. Asserted BOTH ways.
	if strings.Contains(anon, "calendar/prefs") {
		t.Error("item 17: the anonymous render carries a disclosure-register persist URL — every " +
			"<details> toggle would fire an unauthenticated POST at a `cg` route")
	}
	if !strings.Contains(player, "calendar/prefs") {
		t.Error("item 17 (the other half): a PLAYER lost the disclosure register — a guard that " +
			"also passes when the register is broken for everyone proves nothing")
	}

	// ITEM 18 — no RSVP form. bench.templ:400 and :1053 render hx-post={ f.Action }
	// with a method="post" fallback; both are covered by item 12's sweep above,
	// and this names the panel explicitly so a future RSVP control cannot be
	// added on a different attribute without reading this line.
	if strings.Contains(anon, "rsvp") && strings.Contains(anon, "hx-post") {
		t.Error("item 18: the anonymous render carries an RSVP form")
	}
}

// TestAnonymousBench_EmptyUserIDIsNoUser is [VS-5] item 19 and [VS-15] SIGNED:
// the empty-user-id case EXERCISED, not assumed.
//
// app_dashboard.go feeds buildBench `auth.GetUserID(c)`, which is "" for an
// anonymous request, so every per-user lookup in buildBench receives an empty
// key. THIS IS THE ONE FAILURE MODE THE REACHABILITY MATRIX CANNOT SEE, because
// a 200 that quietly renders someone else's row is still a 200.
func TestAnonymousBench_EmptyUserIDIsNoUser(t *testing.T) {
	if testing.Short() {
		t.Skip("the empty-user-id case requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	campaignID := anonFixture(t, db)
	h := anonHandler(t, db)

	// (a) it does not panic — anonRender t.Fatal's on any handler error, and the
	//     test binary would die on a panic, so reaching the next line IS (a).
	first := anonRender(t, h, campaignID, campaigns.RoleNone, true, "")

	// (b) the empty key resolves to nobody. A per-user read that ran with "" and
	//     matched a row would surface as a member's row, lane or RSVP state.
	for _, perUser := range []string{"u-owner", "u-player", "Kaelthorn Vex", "Brynwyth Ashe"} {
		if strings.Contains(first, perUser) {
			t.Errorf("[VS-15]: the empty user key resolved to %q — an empty UserID must never be "+
				"used as a lookup key", perUser)
		}
	}

	// (c) two anonymous requests to the same public campaign render
	//     byte-identically. If they do not, something per-user survived.
	second := anonRender(t, h, campaignID, campaigns.RoleNone, true, "")
	if first != second {
		t.Errorf("[VS-15] item 4: two anonymous renders differ (%d vs %d bytes) — something "+
			"per-user survived the guard", len(first), len(second))
	}

	// (d) no sentinel identity was synthesised to satisfy a lookup. Scoped to the
	//     Bench for the same measured reason as item 12: the app layout's CDN
	//     <link> carries crossorigin="anonymous" on every page in the product.
	benchOnly := anonBenchRegion(t, first)
	for _, sentinel := range []string{"anonymous", "anon-user", "guest-user"} {
		if strings.Contains(benchOnly, sentinel) {
			t.Errorf("[VS-15] item 3: the render carries the synthesised identity %q — a shared "+
				"anonymous identity is a shared write target", sentinel)
		}
	}
}

// TestAnonymousBench_NoCachingHeaders is [VS-16] SIGNED: caching is RULED NONE.
//
// Making a page public-capable is the moment caching becomes tempting and
// dangerous in the same breath — one cached anonymous render served to a member
// is this slice's entire security story reversed. The route emits after this
// slice whatever it emitted before, so the assertion is that the anonymous
// response carries NO cache-validation header at all.
func TestAnonymousBench_NoCachingHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	campaignID := anonFixture(t, db)
	h := anonHandler(t, db)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/apps/calendar", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(campaignID)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: campaignID, Name: "Imix", IsPublic: true},
		MemberRole:  campaigns.RoleNone,
		IsAnonymous: true,
	})
	if err := h.AppDashboard(c); err != nil {
		t.Fatalf("AppDashboard: %v", err)
	}
	for _, hdr := range []string{"Cache-Control", "ETag", "Last-Modified", "Expires"} {
		if v := rec.Header().Get(hdr); v != "" {
			t.Errorf("[VS-16]: the anonymous render emitted %s: %q — the payload is viewer-filtered "+
				"and a cache key without the viewer is a permission bug with good latency", hdr, v)
		}
	}
}

// TestAnonymousBench_RosterRidesNoExport is [VS-5] item 16: no export or
// AI-workspace DTO gains a field. Modelled on rsvp_egress_test.go and
// bench_section_egress_test.go, which is the precedent the block names.
//
// THE THING THAT MUST NEVER EGRESS IS THE PANEL'S ROSTER. This slice makes the
// Bench publicly reachable and answers the member-name question in the producer
// (benchRsvpResolve's Player floor). An export DTO that carried the same roster
// would route around that answer entirely, and exports are hand-written
// per-aggregate so a new field is invisible by construction. STRUCTURAL: no DB,
// no fixture, nothing to keep in sync.
func TestAnonymousBench_RosterRidesNoExport(t *testing.T) {
	tokens := []string{"roster", "benchrsvp", "freedays", "availability"}
	var walk func(typ reflect.Type, path string, seen map[reflect.Type]bool)
	walk = func(typ reflect.Type, path string, seen map[reflect.Type]bool) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			hay := strings.ToLower(f.Name + " " + f.Tag.Get("json") + " " + f.Tag.Get("db"))
			for _, tok := range tokens {
				if strings.Contains(hay, tok) {
					t.Errorf("egress leak: %s.%s references the Bench party roster (%q) — it is "+
						"campaign membership, the surface is now publicly reachable, and the "+
						"audience answer lives in benchRsvpResolve, not in an export",
						path, f.Name, tok)
				}
			}
			ft := f.Type
			for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
				walk(ft, path+"."+f.Name, seen)
			}
		}
	}
	walk(reflect.TypeOf(ChronicleExport{}), "ChronicleExport", map[reflect.Type]bool{})
}
