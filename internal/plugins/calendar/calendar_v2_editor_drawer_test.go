// calendar_v2_editor_drawer_test.go — the redesigned full event editor DRAWER
// (C-CAL-LARGE-EDITOR, design slice 5 of 6). Pins the SIGNED-mockup sections
// that render from shipped fields, the disabled+flagged gaps (location, RSVP,
// sky-pin — no storage), the fantasy-hint data plumbing, and the backend
// binding-completion for `tier` + `all_day` (existing columns; the drawer's
// tier segment + all-day toggle had no wire before this slice).
//
// Cites: cordinator/decisions/2026-05-21-core-tenets §T-B3 (production UI),
// §T-B1 (visibility gate), §T-B2 (calendar-owned).
package calendar

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// renderEditorDrawer renders eventV2Drawer with a rich, mockup-shaped view so
// the section pins below all have data to bind against.
func renderEditorDrawer(t *testing.T, mutate func(*CalendarV2ViewData)) string {
	t.Helper()
	end := 1495
	data := CalendarV2ViewData{
		ActiveCalendar: &Calendar{
			ID: "cal-1", Name: "Harptos", EpochName: strPtr("DR"),
			Months: []Month{{Name: "Hammer", Days: 30}, {Name: "Harvestwane", Days: 30}},
			EventCategories: []EventCategory{
				{Slug: "session", Name: "Session", Color: "#2a78d6"},
				{Slug: "quest", Name: "Quest", Color: "#e34948"},
			},
			Eras: []Era{{Name: "Year of the Broken Lantern", StartYear: 1490, EndYear: &end, Color: "#7c5cd6"}},
		},
		CampaignID: "camp-1",
		IsScribe:   true,
		TierDefinitions: []TierDefinitionAlias{
			{Slug: "major", Name: "Major", Color: "#ef4444", Prominence: 100},
			{Slug: "standard", Name: "Standard", Color: "#6366f1", Prominence: 50},
		},
	}
	if mutate != nil {
		mutate(&data)
	}
	var sb strings.Builder
	if err := eventV2Drawer(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render drawer: %v", err)
	}
	return sb.String()
}

// TestEditorDrawer_MockupSectionsRender pins every section of the signed layout
// that maps to a shipped, wired field.
func TestEditorDrawer_MockupSectionsRender(t *testing.T) {
	html := renderEditorDrawer(t, nil)
	for _, want := range []string{
		`role="dialog"`, `aria-modal="true"`, // focus-trapped modal chrome
		"drawer-slide-in",                   // sky-panel slide-in motion
		`data-drawer-backdrop`,              // scrim
		`data-field="name"`,                 // title
		`data-type-chips`, `data-type-chip`, // type chips
		`data-cat-slug="session"`, `data-cat-slug="quest"`,
		`data-field="category"`,                                       // hidden chip-backed category
		`data-field="month"`, `data-field="day"`, `data-field="year"`, // WHEN date
		`data-time-fields`, `data-time-start`, `data-time-end`, // times
		`data-fantasy-hint`,               // equivalence hint line
		`data-allday-toggle`,              // all-day
		`data-field="recurrence_type"`,    // recurrence
		`data-recurrence-summary`,         // plain-language summary
		`>Repeats weekly<`,                // weekly option
		`data-tier-seg`, `data-tier-chip`, // tier segment (WHO)
		`data-tier-slug="major"`, `data-field="tier"`,
		`data-field="description"`, // description
		`data-event-ties-section`,  // linked entities (ties)
		`data-drawer-error`,        // inline 422 region
		`data-drawer-save`,         // save
		`Save changes`,             // mockup label
		`btn-primary`,              // action-accent token (bg-action)
	} {
		if !strings.Contains(html, want) {
			t.Errorf("drawer missing signed-mockup element %q", want)
		}
	}
}

// TestEditorDrawer_FantasyHintDataPlumbed pins the epoch + eras the JS reads to
// compute the fantasy-equivalence hint.
func TestEditorDrawer_FantasyHintDataPlumbed(t *testing.T) {
	html := renderEditorDrawer(t, nil)
	if !strings.Contains(html, `data-cal-epoch="DR"`) {
		t.Error("drawer must carry data-cal-epoch for the fantasy hint")
	}
	// templ HTML-escapes the JSON; the DOM decodes it back before JSON.parse.
	if !strings.Contains(html, "data-cal-eras=") || !strings.Contains(html, "Year of the Broken Lantern") {
		t.Error("drawer must carry data-cal-eras with the era vocabulary")
	}
}

// TestEditorDrawer_GapsDisabledAndFlagged pins the no-storage gaps: they render
// (the signed layout keeps the slot) but DISABLED with a flag, never as a
// control that silently drops input.
func TestEditorDrawer_GapsDisabledAndFlagged(t *testing.T) {
	html := renderEditorDrawer(t, func(d *CalendarV2ViewData) { d.CanControlWorldState = true })
	// Sky-pin — dispatch's explicit "render disabled + flag" case.
	if !strings.Contains(html, "coming with sky-pin storage") {
		t.Error("sky-pin select must render disabled with the coming-soon flag")
	}
	// Location — no column; disabled + flagged.
	if !strings.Contains(html, "coming with location storage") {
		t.Error("location field must render disabled with the coming-soon flag")
	}
	// RSVP — no per-event storage; disabled + flagged.
	if !strings.Contains(html, "Collect RSVPs") || !strings.Contains(html, "coming soon") {
		t.Error("Collect RSVPs must render disabled with the coming-soon flag")
	}
	// Every gap control must actually carry the disabled attribute.
	if strings.Count(html, "disabled") < 3 {
		t.Errorf("expected the 3 gap controls disabled; got %d 'disabled' tokens", strings.Count(html, "disabled"))
	}
}

// TestEditorDrawer_VisibilityGate pins the GM-only case: co-DMs get the full
// visibility editor (Everyone/GM-only + advanced rules); everyone else gets the
// locked "Visible to everyone" note (server downgrade backstops it).
func TestEditorDrawer_VisibilityGate(t *testing.T) {
	gm := renderEditorDrawer(t, func(d *CalendarV2ViewData) { d.CanAuthorDmOnly = true })
	if !strings.Contains(gm, "data-visibility-editor") {
		t.Error("co-DM drawer must render the visibility editor (Everyone/GM-only)")
	}
	scribe := renderEditorDrawer(t, nil) // CanAuthorDmOnly false
	if !strings.Contains(scribe, "data-event-visibility-locked") {
		t.Error("non-co-DM drawer must render the locked 'Visible to everyone' note")
	}
	if strings.Contains(scribe, "data-visibility-editor") {
		t.Error("non-co-DM must NOT get the restricted-visibility editor")
	}
}

// TestEditorDrawer_FooterCarriesDeleteCancelSave pins the SIGNED footer order:
// Delete… (left) · Cancel · Save changes (right, action accent). This replaces
// the pre-redesign "delete lives in the Actions section" contract — the mockup
// puts Delete back in the footer.
func TestEditorDrawer_FooterCarriesDeleteCancelSave(t *testing.T) {
	html := renderEditorDrawer(t, nil)
	del := strings.LastIndex(html, "data-drawer-delete")
	save := strings.LastIndex(html, "data-drawer-save")
	if del < 0 || save < 0 {
		t.Fatal("footer must carry both delete and save")
	}
	if del > save {
		t.Error("footer order must be Delete… before Save changes (mockup)")
	}
}

// --- Tier segment vocabulary + fallback ---

func TestEventTierOptions_PrefersCampaignDefs(t *testing.T) {
	data := CalendarV2ViewData{TierDefinitions: []TierDefinitionAlias{{Slug: "epic", Name: "Epic"}}}
	got := eventTierOptions(data)
	if len(got) != 1 || got[0].Slug != "epic" {
		t.Errorf("campaign tier defs must win; got %+v", got)
	}
}

func TestEventTierOptions_FallsBackToPlatformDefaults(t *testing.T) {
	got := eventTierOptions(CalendarV2ViewData{})
	if len(got) != 3 {
		t.Fatalf("empty defs must fall back to the 3 platform tiers; got %d", len(got))
	}
	slugs := got[0].Slug + "," + got[1].Slug + "," + got[2].Slug
	if slugs != "major,standard,detail" {
		t.Errorf("platform fallback slugs = %q; want major,standard,detail", slugs)
	}
}

// --- Fantasy-hint helpers ---

func TestEventDrawerErasJSON_ShapeAndEmpty(t *testing.T) {
	end := 1495
	data := CalendarV2ViewData{ActiveCalendar: &Calendar{
		Eras: []Era{{Name: "Broken Lantern", StartYear: 1490, EndYear: &end}},
	}}
	got := eventDrawerErasJSON(data)
	if !strings.Contains(got, `"name":"Broken Lantern"`) || !strings.Contains(got, `"start":1490`) || !strings.Contains(got, `"end":1495`) {
		t.Errorf("eras JSON shape wrong: %s", got)
	}
	if eventDrawerErasJSON(CalendarV2ViewData{}) != "[]" {
		t.Error("no calendar → '[]' so the JS parse is always safe")
	}
	// Ongoing era → null end.
	ongoing := CalendarV2ViewData{ActiveCalendar: &Calendar{Eras: []Era{{Name: "Now", StartYear: 1, EndYear: nil}}}}
	if !strings.Contains(eventDrawerErasJSON(ongoing), `"end":null`) {
		t.Errorf("ongoing era must serialize end:null; got %s", eventDrawerErasJSON(ongoing))
	}
}

func TestEventDrawerEpoch(t *testing.T) {
	if eventDrawerEpoch(CalendarV2ViewData{}) != "" {
		t.Error("no calendar → empty epoch")
	}
	data := CalendarV2ViewData{ActiveCalendar: &Calendar{EpochName: strPtr("DR")}}
	if eventDrawerEpoch(data) != "DR" {
		t.Errorf("epoch = %q; want DR", eventDrawerEpoch(data))
	}
}

// --- Backend binding completion: all_day clears the clock (both paths) ---

func TestUpdateEvent_AllDayClearsStoredTimes(t *testing.T) {
	var written *Event
	repo := &mockCalendarRepo{
		getEventFn:    func(_ context.Context, _ string) (*Event, error) { return seededEvent(), nil },
		updateEventFn: func(_ context.Context, evt *Event) error { written = evt; return nil },
	}
	svc := newTestCalendarService(repo)

	// seededEvent() has StartHour 14 etc. Toggling all-day ON must clear them so
	// the grid (nil StartHour == all-day) renders it correctly — the null-preserve
	// guard would otherwise keep the stale clock.
	err := svc.UpdateEvent(context.Background(), "evt-1", UpdateEventInput{
		Name: "Now all day", Year: 1492, Month: 7, Day: 15, Visibility: "everyone",
		AllDay: true,
	})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if written == nil {
		t.Fatal("repo.UpdateEvent not called")
	}
	if !written.AllDay {
		t.Error("AllDay must persist true")
	}
	for _, c := range []struct {
		name string
		got  *int
	}{
		{"StartHour", written.StartHour}, {"StartMinute", written.StartMinute},
		{"EndHour", written.EndHour}, {"EndMinute", written.EndMinute},
	} {
		if c.got != nil {
			t.Errorf("%s must be cleared for an all-day event; got %d", c.name, *c.got)
		}
	}
}

func TestUpdateEvent_NotAllDayKeepsNullPreserve(t *testing.T) {
	var written *Event
	repo := &mockCalendarRepo{
		getEventFn:    func(_ context.Context, _ string) (*Event, error) { return seededEvent(), nil },
		updateEventFn: func(_ context.Context, evt *Event) error { written = evt; return nil },
	}
	svc := newTestCalendarService(repo)

	// AllDay false (zero value) + no time fields sent → the existing null-preserve
	// guard must keep the seeded clock. This pins that my all-day clear is gated
	// strictly on AllDay==true and does not regress C-CAL-NULL-PRESERVE.
	err := svc.UpdateEvent(context.Background(), "evt-1", UpdateEventInput{
		Name: "Title only", Year: 1492, Month: 7, Day: 15, Visibility: "everyone",
	})
	if err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if written.StartHour == nil || *written.StartHour != 14 {
		t.Errorf("StartHour must be preserved (14) when not all-day; got %v", written.StartHour)
	}
	if written.AllDay {
		t.Error("AllDay must be false")
	}
}

func TestCreateEvent_AllDayClearsTimes(t *testing.T) {
	var written *Event
	repo := &mockCalendarRepo{
		createEventFn: func(_ context.Context, evt *Event) error { written = evt; return nil },
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestCalendarService(repo)

	sh, sm := 19, 0
	_, err := svc.CreateEvent(context.Background(), "cal-1", CreateEventInput{
		Name: "All-day fest", Year: 1492, Month: 7, Day: 15, Visibility: "everyone",
		StartHour: &sh, StartMinute: &sm, AllDay: true,
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if written == nil {
		t.Fatal("repo.CreateEvent not called")
	}
	if written.StartHour != nil || written.StartMinute != nil {
		t.Error("all-day create must drop the clock time even if times were supplied")
	}
	if !written.AllDay {
		t.Error("AllDay must persist true on create")
	}
}

// TestEventHandler_BindsTierAndAllDay pins the internal-UI handler binding
// completion at the source level (the request structs + pass-through), the way
// the codebase pins other wire seams. Guards a future sweep from dropping the
// two fields again and re-breaking the drawer's tier segment + all-day toggle.
func TestEventHandler_BindsTierAndAllDay(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/handler.go")
	// Whitespace-insensitive so gofmt's tag/value alignment can shift freely.
	ws := regexp.MustCompile(`\s+`)
	flat := ws.ReplaceAllString(src, " ")
	for _, want := range []string{
		"Tier *string `json:\"tier\"`",   // request-struct binding
		"AllDay bool `json:\"all_day\"`", // request-struct binding
		"Tier: req.Tier,",                // pass-through to the service input
		"AllDay: req.AllDay,",            // pass-through to the service input
	} {
		if strings.Count(flat, want) < 2 {
			t.Errorf("CreateEventAPI + UpdateEventAPI must both bind %q (found %d, want 2)", want, strings.Count(flat, want))
		}
	}
}
