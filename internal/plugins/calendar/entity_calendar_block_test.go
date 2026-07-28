// entity_calendar_block_test.go — C-CAL-ENTITY-PAGE-EMBED. The entity-page
// calendar embed renders the band + the entity's linked events, filters
// dm_only by viewer role, and shows empty-but-present states (never blank).
package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestEntityEmbeds_DistinctSeedIds is the E7 guard (C-CAL-CLOSEOUT): a page
// composing BOTH an entity worldstate embed and an entity calendar embed for
// the same entity must NOT emit two elements with the same id (invalid HTML
// that left getElementById binding only the first band). The seed ids are now
// namespaced per band type, and both carry the [data-cal-worldstate] attribute
// the engine actually reads.
func TestEntityEmbeds_DistinctSeedIds(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	var ws, cal strings.Builder
	if err := EntityWorldStateBlock(sampleWSSvc(), cc, "ent-1", "user-1", "", "own").Render(context.Background(), &ws); err != nil {
		t.Fatalf("worldstate render: %v", err)
	}
	if err := EntityCalendarBlock(sampleEmbedSvc(), cc, "ent-1", "user-1", "", "").Render(context.Background(), &cal); err != nil {
		t.Fatalf("calendar render: %v", err)
	}
	wsID := `id="cal-v2-worldstate-ws-ent-1"`
	calID := `id="cal-v2-worldstate-cal-ent-1"`
	if !strings.Contains(ws.String(), wsID) {
		t.Errorf("worldstate embed missing namespaced seed id %q", wsID)
	}
	if !strings.Contains(cal.String(), calID) {
		t.Errorf("calendar embed missing namespaced seed id %q", calID)
	}
	if wsID == calID {
		t.Fatalf("the two embeds must use distinct ids on a shared page")
	}
	// The plain (un-namespaced) duplicate id must be gone from both.
	for _, h := range []string{ws.String(), cal.String()} {
		if strings.Contains(h, `id="cal-v2-worldstate"`) {
			t.Errorf("entity embed still emits the collision-prone plain id")
		}
		if !strings.Contains(h, "data-cal-worldstate=") {
			t.Errorf("entity embed missing the [data-cal-worldstate] seed attribute")
		}
	}
}

// entityCalBlockStub satisfies CalendarService via embedding; only the methods
// EntityCalendarBlock calls are overridden.
type entityCalBlockStub struct {
	CalendarService
	cal  *Calendar
	seed *WorldStateSeed
	ties []EntityEventTie

	// filteredCalls records every EventsForEntityFiltered call's viewer context,
	// so the switch away from the hand-rolled filter loop is pinned by what the
	// host ASKED FOR, not only by what came back.
	filteredCalls [][2]string // {entityID, userID}
	filteredRoles []int
}

func (s *entityCalBlockStub) GetCalendar(context.Context, string) (*Calendar, error) {
	return s.cal, nil
}
func (s *entityCalBlockStub) BuildWorldStateSeed(context.Context, string, int, int, int, int, string) (*WorldStateSeed, error) {
	return s.seed, nil
}

// EventsForEntity is deliberately left FAILING. Nothing in the entity embed may
// reach for the unfiltered read any more: it applies no calendar-level
// visibility, so a tie into a calendar the viewer cannot see would surface that
// event's name (C-CALV4-TIEFIX-PB Bug 1 item 3).
func (s *entityCalBlockStub) EventsForEntity(context.Context, string) ([]EntityEventTie, error) {
	return nil, errors.New("entity embed must use the viewer-filtered read, not EventsForEntity")
}

// EventsForEntityFiltered mirrors the real method's contract: event-level
// visibility applied for this viewer (the calendar-level half needs a repo and
// is pinned in entity_ties_test.go).
func (s *entityCalBlockStub) EventsForEntityFiltered(_ context.Context, entityID string, role int, userID string) ([]EntityEventTie, error) {
	s.filteredCalls = append(s.filteredCalls, [2]string{entityID, userID})
	s.filteredRoles = append(s.filteredRoles, role)
	if permissions.CanSeeDmOnly(role) || userID == "" {
		return s.ties, nil
	}
	var out []EntityEventTie
	for _, t := range s.ties {
		if canUserView(t.Event.Visibility, t.Event.VisibilityRules, role, userID) {
			out = append(out, t)
		}
	}
	return out, nil
}

func renderEntityCal(t *testing.T, svc CalendarService, role campaigns.Role, dmGranted bool) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: dmGranted}
	var sb strings.Builder
	if err := EntityCalendarBlock(svc, cc, "ent-1", "user-1", "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// GetBlockLayers — C-CALV4-LAYERS-P9. The stub EMBEDS CalendarService (a nil
// interface), so a method it does not implement compiles fine and panics at
// call time. The host now reads the viewer's stored layer set, so the stub has
// to answer: nil, meaning "never chosen", which is what keeps every assertion
// in this file about the host's SEED rather than about a preference.
func (s *entityCalBlockStub) GetBlockLayers(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func sampleEmbedSvc() *entityCalBlockStub {
	return &entityCalBlockStub{
		cal:  &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 15, HoursPerDay: 24, MinutesPerHour: 60},
		seed: &WorldStateSeed{TimeOfDay: 0.5, Season: "Spring", Date: WorldStateDate{1492, 4, 15}, Weather: WorldStateWeather{Type: "rain", Intensity: 1}},
		ties: []EntityEventTie{
			{Event: Event{ID: "e1", Name: "Public Siege", Year: 1492, Month: 4, Day: 15, Visibility: "everyone"}, ParticipationRole: "involved"},
			{Event: Event{ID: "e2", Name: "Secret Pact", Year: 1492, Month: 4, Day: 16, Visibility: "dm_only"}, ParticipationRole: "mentioned"},
		},
	}
}

func TestEntityCalendarBlock_RendersBandAndEvents(t *testing.T) {
	html := renderEntityCal(t, sampleEmbedSvc(), campaigns.RoleOwner, false)
	for _, want := range []string{
		"data-entity-calendar",      // block container
		"id=\"cal-v2-worldstate-cal-ent-1\"", // E7: per-band namespaced seed id (no duplicate-id collisions)
		"data-cal-worldstate=",               // the seed blob the engine reads by attribute
		"data-cal-sky",              // the reused 2a band scaffold
		"/static/js/cal-almanac.js", // the shared engine
		"Linked events",             // the events section header
		"Public Siege",              // the linked event
		"involved",                  // its participation role
	} {
		if !strings.Contains(html, want) {
			t.Errorf("embed missing %q", want)
		}
	}
}

func TestEntityCalendarBlock_DmOnlyFiltering(t *testing.T) {
	// Owner sees the secret event; Player does not.
	owner := renderEntityCal(t, sampleEmbedSvc(), campaigns.RoleOwner, false)
	if !strings.Contains(owner, "Secret Pact") {
		t.Errorf("owner should see the dm_only linked event")
	}
	player := renderEntityCal(t, sampleEmbedSvc(), campaigns.RolePlayer, false)
	if strings.Contains(player, "Secret Pact") {
		t.Errorf("player must NOT see the dm_only linked event")
	}
	if !strings.Contains(player, "Public Siege") {
		t.Errorf("player should still see the public linked event")
	}
	// A DM-granted player (co-DM) sees it.
	coDM := renderEntityCal(t, sampleEmbedSvc(), campaigns.RolePlayer, true)
	if !strings.Contains(coDM, "Secret Pact") {
		t.Errorf("co-DM (dm-granted) should see the dm_only linked event")
	}
}

// TestEntityCalendarBlock_Unavailable: no campaign context renders the friendly
// unavailable state, never a raw error/blank (item 2).
func TestEntityCalendarBlock_Unavailable(t *testing.T) {
	render := func(cc *campaigns.CampaignContext, entityID string) string {
		var sb strings.Builder
		if err := EntityCalendarBlock(sampleEmbedSvc(), cc, entityID, "u1", "", "").Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}
	// No campaign context → friendly unavailable state, no band.
	html := render(nil, "ent-1")
	if !strings.Contains(html, "data-entity-calendar-unavailable") {
		t.Errorf("nil cc: expected friendly unavailable state, got: %q", html)
	}
	if strings.Contains(html, "data-cal-sky") {
		t.Errorf("nil cc: must not render the band")
	}
}

// TestEntityCalendarBlock_PreviewPlaceholder (C-WIDGET-BINDING-QA1 Bug 2): a
// concrete-entity block rendered WITHOUT an entity (customization/layout editor
// or preview) shows the CALM "previews on the entity page" placeholder — never
// the alarming can't-load copy — and never the band.
func TestEntityCalendarBlock_PreviewPlaceholder(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	var sb strings.Builder
	if err := EntityCalendarBlock(sampleEmbedSvc(), cc, "", "u1", "", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "data-entity-calendar-preview") {
		t.Errorf("empty entity: expected calm preview placeholder, got: %q", html)
	}
	// Must NOT be the alarming unavailable copy, and must not render the band.
	if strings.Contains(html, "data-entity-calendar-unavailable") || strings.Contains(html, "data-cal-sky") {
		t.Errorf("empty entity: must show the preview placeholder only, got: %q", html)
	}
}

// TestEntityCalendarBlock_OpenFullCalendarLink (C-WIDGET-BINDING-QA2 Part B):
// the block header has an "Open full calendar" link to the V2 shell for the
// resolved calendar, shown for ALL roles (players can view the calendar).
func TestEntityCalendarBlock_OpenFullCalendarLink(t *testing.T) {
	for _, role := range []campaigns.Role{campaigns.RolePlayer, campaigns.RoleScribe} {
		html := renderEntityCal(t, sampleEmbedSvc(), role, false)
		// V2 shell for the resolved calendar (sampleEmbedSvc's default cal-1).
		if !strings.Contains(html, "/campaigns/camp-1/calendar/v2/cal-1") {
			t.Errorf("role %d: missing V2 open-calendar link; got: %q", role, html)
		}
		if !strings.Contains(html, "data-open-calendar") || !strings.Contains(html, "Open full calendar") {
			t.Errorf("role %d: missing the Open-full-calendar affordance", role)
		}
		// Must be V2, never the V1 /calendars/<id> view.
		if strings.Contains(html, `href="/campaigns/camp-1/calendars/cal-1"`) {
			t.Errorf("role %d: open link must target V2, not the V1 view", role)
		}
	}
}

func TestEntityCalendarBlock_EmptyStates(t *testing.T) {
	// No calendar → friendly "Create calendar" CTA (C-CAL-EMBED-CONVERGE-POLISH
	// item 3), not a raw message; no band.
	noCal := renderEntityCal(t, &entityCalBlockStub{}, campaigns.RoleOwner, false)
	for _, want := range []string{"data-entity-calendar-empty", "No calendar yet", "Create calendar", "/campaigns/camp-1/calendars"} {
		if !strings.Contains(noCal, want) {
			t.Errorf("no-calendar empty state missing %q; got: %q", want, noCal)
		}
	}
	if strings.Contains(noCal, "data-cal-sky") {
		t.Errorf("no-calendar should not render the band")
	}
	// Calendar but no linked events → header + "No linked events".
	svc := sampleEmbedSvc()
	svc.ties = nil
	noTies := renderEntityCal(t, svc, campaigns.RoleOwner, false)
	if !strings.Contains(noTies, "Linked events") || !strings.Contains(noTies, "No linked events") {
		t.Errorf("no-ties should show the header + empty note, got: %q", noTies)
	}
}

// ── C-CALV4-HOST-P3: the entity page hosts the calendar-v4 Block ────────────

// entityHostEvents is the signed entity scenario as EVENT ROWS: two visible
// events tied to the host entity, one visible and untied, one dm_only and tied.
//
// The tie is carried by calendar_events.entity_id — the original one-entity
// link, which blockEventTied honours directly — rather than by the batched
// entity_event_links read, so this fixture needs nothing from the repository
// fake beyond the rows themselves.
func entityHostEvents() []Event {
	host := "ent-1"
	evs := []Event{
		blockEvent("tied-visible-1", 3, "everyone"),
		blockEvent("tied-visible-2", 8, "everyone"),
		blockEvent("untied-visible", 12, "everyone"),
		blockEvent("tied-hidden", 5, "dm_only"), // a player must never learn this exists
	}
	evs[0].EntityID = &host
	evs[1].EntityID = &host
	evs[3].EntityID = &host
	return evs
}

// entityHostSpine installs a real BlockService over the fake repository for the
// duration of one test, and restores whatever was there before.
//
// The spine is a process-wide singleton (InstallBlockSpine) because
// internal/app/routes.go belongs to exactly one calendar-v4 slice for the whole
// wave, so tests save and restore rather than construct one per call.
func entityHostSpine(t *testing.T, cal *Calendar) *entityCalBlockStub {
	t.Helper()
	repo := newBlockFakeRepo(cal)
	repo.events[cal.ID] = entityHostEvents()
	prev := BlockSpine()
	InstallBlockSpine(NewBlockService(repo))
	t.Cleanup(func() { blockSpine.Store(prev) })

	svc := sampleEmbedSvc()
	svc.cal = cal
	return svc
}

// TestEntityCalendarBlock_RendersTheV4Block is §1: the embed's month surface is
// now internal/widgets/calendar_block.Block, projected through the spine — not
// the compact adaptiveCalendarWidget it grew before the Block existed.
func TestEntityCalendarBlock_RendersTheV4Block(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())

	html := renderEntityCal(t, svc, campaigns.RoleOwner, false)
	for _, want := range []string{
		"data-cal-block",                 // the Block's size-class query container
		`data-cal-slug="cal-harptos"`,    // the real calendar identity, not a zero
		"Harptos of Imix",                // the nameplate
		"/static/css/calendar-block.css", // the Block brings its own sheet
		"Deepwinter",                     // the month it resolved
	} {
		if !strings.Contains(html, want) {
			t.Errorf("entity embed missing %q — the Block did not render", want)
		}
	}
	// The pre-v4 surface is gone from THIS embed (app_dashboard.templ keeps the
	// component; removing dead code there is a post-wave slice).
	if strings.Contains(html, "data-cal-adaptive") {
		t.Error("the entity embed still renders the pre-v4 adaptive calendar widget")
	}
	// The rest of the ladder still renders around it.
	for _, want := range []string{"data-entity-calendar", `id="cal-v2-worldstate-cal-ent-1"`, "Linked events"} {
		if !strings.Contains(html, want) {
			t.Errorf("the Block replaced more than the month surface: %q is gone", want)
		}
	}
}

// TestEntityCalendarBlock_TieToggleIsHostedAndDefaultsTied is §3 from the host's
// side: the Block knows which entity hosts it, so the toggle renders with both
// counts, defaults to the signed tie-filtered mode, and carries the CSS-only
// control's radios. Both counts come from the spine's ONE viewer-filtered pass.
func TestEntityCalendarBlock_TieToggleIsHostedAndDefaultsTied(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())

	gm := renderEntityCal(t, svc, campaigns.RoleOwner, false)
	for _, want := range []string{
		`data-tie-mode="tied"`, // the signed default on an entity page
		`class="seg tie"`,
		`data-tie-pick="tied"`,
		`data-tie-pick="whole"`,
		"Tied 3",           // 3 of the GM's 4 visible events are tied to ent-1
		"Whole calendar 4",
	} {
		if !strings.Contains(gm, want) {
			t.Errorf("entity-hosted Block missing %q", want)
		}
	}

	// A player's pair is computed from the PLAYER's own visible set, so their
	// difference can never be used to infer the dm_only event.
	player := renderEntityCal(t, svc, campaigns.RolePlayer, false)
	if !strings.Contains(player, "Tied 2") || !strings.Contains(player, "Whole calendar 3") {
		t.Error("player counts must come from the player's own filtered pass (want Tied 2 / Whole calendar 3)")
	}
	if strings.Contains(player, "tied-hidden") {
		t.Error("the dm_only event reached a player's DOM")
	}
}

// TestEntityCalendarBlock_HostLayerSet is the A4 ruling made testable.
//
// Producer DEF is ["moons"] and stays there (cordinator ruling 2026-07-28 §1).
// The entity page is a HOST that passes its own layer set, and the set it passes
// is the one the signed entity renders show — eras, week numbers, the docked
// Ledger and the Shelf — minus the one key that is measurably premature
// (moongraph, below) and minus the two those renders do not show at all
// (legend, horizon).
func TestEntityCalendarBlock_HostLayerSet(t *testing.T) {
	svc := entityHostSpine(t, blockTenDayCal())
	html := renderEntityCal(t, svc, campaigns.RoleOwner, false)

	for _, want := range []struct{ needle, why string }{
		{`data-zone="ledger"`, "the docked Ledger — the full-tier column arithmetic subtracts its 300px unconditionally"},
		{`data-zone="shelf"`, "the Shelf foot"},
		{"data-weeknums", "the W1/W2/W3 gutter"},
	} {
		if !strings.Contains(html, want.needle) {
			t.Errorf("entity host layer set is missing %s (%s)", want.needle, want.why)
		}
	}
	for _, bad := range []string{`data-layer="legend"`, `data-layer="horizon"`} {
		if strings.Contains(html, bad) {
			t.Errorf("%s is not in the signed entity renders — the host must not enable it", bad)
		}
	}
	// moongraph IS in the signed entity renders and is still omitted, on a
	// measured layout ground: with it enabled the std tier stacks the moongraph
	// needzone row, the docked Ledger and the Shelf into one another and the
	// Ledger/Shelf headers collide. Wave 1's zone is a `needs backend` chip, so
	// the collision buys nothing.
	//
	// IT STAYS UNINVERTED AFTER W-F, and that is a RULING rather than an
	// oversight ([LYR-7] SIGNED, C-CALV4-LAYERS-P9). W-F filled the zone and
	// deliberately did NOT add the key: the booking wanted REACHABILITY and the
	// switchboard supplies it directly, while L29 says the illumination graph
	// defaults OFF. So the host SEED still omits moongraph, this assertion still
	// holds for a viewer who has never chosen, and the zone is now one toggle
	// away instead of a wave away.
	if strings.Contains(html, `data-layer="moongraph"`) {
		t.Error("moongraph is booked for W-F: enabling its stub zone collides with the docked " +
			"Ledger and the Shelf at std tier, for a chip that carries no information")
	}
	// The set is the HOST's, not DEF's: DEF stays moons-only under the ruling —
	// and it still does after C-CALV4-LAYERS-P9, for a viewer with no stored
	// row. The zero blockLayerPrefs IS that viewer.
	if got := blockDefaultLayers(blockLayerPrefs{}).Enabled; len(got) != 1 || got[0] != "moons" {
		t.Errorf("producer DEF must stay [\"moons\"] (cordinator ruling 2026-07-28 §1); got %v", got)
	}
}

// TestEntityCalendarBlock_HiddenCalendarIsIndistinguishableFromNone is A2 /
// C-CALV4-SEAM-P5 stage 9 held at the host boundary.
//
// BlockService.Block answers a calendar the viewer may not see with the same
// not-found a nonexistent one gets. This host must not undo that by rendering a
// different state for the two — a viewer who could tell them apart could probe
// for hidden calendars one entity page at a time.
func TestEntityCalendarBlock_HiddenCalendarIsIndistinguishableFromNone(t *testing.T) {
	cal := blockTenDayCal()
	cal.Visibility = "dm_only"
	svc := entityHostSpine(t, cal)

	hidden := renderEntityCal(t, svc, campaigns.RolePlayer, false)
	// The no-calendar rung, for a campaign that genuinely has none.
	none := renderEntityCal(t, &entityCalBlockStub{}, campaigns.RolePlayer, false)

	if hidden != none {
		t.Errorf("a hidden calendar must render byte-identically to no calendar at all;\nhidden: %q\nnone:   %q", hidden, none)
	}
	for _, leak := range []string{"Harptos of Imix", "cal-harptos", "Deepwinter", "data-cal-block"} {
		if strings.Contains(hidden, leak) {
			t.Errorf("a hidden calendar leaked %q to a player", leak)
		}
	}
	// The GM still sees it — the gate is visibility, not a blanket removal.
	if gm := renderEntityCal(t, svc, campaigns.RoleOwner, false); !strings.Contains(gm, "data-cal-block") {
		t.Error("the GM must still get the Block for a dm_only calendar")
	}
}

// TestEntityCalendarBlock_DegradesWithoutTheSpine keeps the ladder honest: a
// degraded calendar plugin (no spine installed) omits the Block and renders
// everything it can still build, rather than blanking the entity page.
func TestEntityCalendarBlock_DegradesWithoutTheSpine(t *testing.T) {
	prev := BlockSpine()
	blockSpine.Store(nil)
	t.Cleanup(func() { blockSpine.Store(prev) })

	html := renderEntityCal(t, sampleEmbedSvc(), campaigns.RoleOwner, false)
	if strings.Contains(html, "data-cal-block") {
		t.Error("no spine, but a Block rendered")
	}
	for _, want := range []string{"data-entity-calendar", "data-cal-sky", "Linked events", "Public Siege"} {
		if !strings.Contains(html, want) {
			t.Errorf("the embed must still render %q when the Block cannot be built", want)
		}
	}
}

// TestEntityCalendarBlock_TiesUseTheViewerFilteredRead pins §3's read.
//
// The old loop called EventsForEntity and re-implemented the event-level filter
// inline, which left the CALENDAR-level half unapplied: a tie into a calendar
// the viewer may not see still printed that event's name. The stub's
// EventsForEntity now returns an error, so a regression to it fails loudly
// rather than silently widening what a player sees.
func TestEntityCalendarBlock_TiesUseTheViewerFilteredRead(t *testing.T) {
	svc := sampleEmbedSvc()
	html := renderEntityCal(t, svc, campaigns.RolePlayer, false)

	if len(svc.filteredCalls) != 1 {
		t.Fatalf("expected exactly one viewer-filtered tie read; got %d", len(svc.filteredCalls))
	}
	if got := svc.filteredCalls[0]; got != [2]string{"ent-1", "user-1"} {
		t.Errorf("the tie read must carry the host entity and the viewer; got %v", got)
	}
	if svc.filteredRoles[0] != permissions.RolePlayer {
		t.Errorf("the tie read must carry the viewer's role; got %d", svc.filteredRoles[0])
	}
	if strings.Contains(html, "Secret Pact") {
		t.Error("the dm_only linked event survived the filtered read")
	}
}
