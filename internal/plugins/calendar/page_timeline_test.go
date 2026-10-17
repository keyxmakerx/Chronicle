// page_timeline_test.go: the time line block's line must hold exactly the
// events a month read would show the same viewer, belonging to this page,
// placed between the first of them and today.
package calendar

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

const timelinePage = "ent-keep"

// newTimelineService wires the real service over eraFixture's calendar
// (today is 1022-8-9, a secret era from 1040) and events that belong to
// timelinePage unless their EntityID names another page. The fake applies
// the role filter the SQL applies.
func newTimelineService(cals []Calendar, def *Calendar, events []Event) CalendarService {
	calRepo := &fakeCalendarRepo{
		getDefaultFn: func(context.Context, string) (*Calendar, error) {
			if def == nil {
				return nil, nil
			}
			c := *def
			return &c, nil
		},
		listByCampaignFn: func(context.Context, string) ([]Calendar, error) { return append([]Calendar(nil), cals...), nil },
		getMonthsFn:      func(context.Context, string) ([]Month, error) { return eraMonths(), nil },
		getErasFn: func(context.Context, string) ([]Era, error) {
			_, eras, _ := eraFixture()
			return eras, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listForPageFn: func(_ context.Context, calendarID, entityID string, role int) ([]Event, error) {
			var out []Event
			for _, e := range events {
				if e.CalendarID != calendarID || (e.EntityID != nil && *e.EntityID != entityID) {
					continue
				}
				if e.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
					continue
				}
				out = append(out, e)
			}
			return out, nil
		},
	}
	return NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{})
}

func timelineEvents() []Event {
	_, _, events := eraFixture()
	deny := `{"denied_users":["u-1"]}`
	return append(events,
		Event{ID: "ev-dm", CalendarID: eraCalendarID, Name: "Assassination plot", Year: 1021, Month: 3, Day: 2, Visibility: "dm_only"},
		Event{ID: "ev-denied", CalendarID: eraCalendarID, Name: "Private oath", Year: 1020, Month: 1, Day: 1, Visibility: "everyone", VisibilityRules: &deny},
		Event{ID: "ev-ondaylater", CalendarID: eraCalendarID, Name: "Surprise", Year: 1022, Month: 9, Day: 1, Visibility: "everyone"},
		Event{ID: "ev-other", CalendarID: eraCalendarID, EntityID: strp("ent-other"), Name: "Elsewhere", Year: 1010, Month: 1, Day: 1, Visibility: "everyone"},
	)
}

func TestPageTimeline_FiltersLikeAMonthRead(t *testing.T) {
	cal, _, _ := eraFixture()
	svc := newTimelineService([]Calendar{cal}, &cal, timelineEvents())

	tests := []struct {
		name   string
		role   int
		hideDM bool
		want   []string
	}{
		{"player sees no dm-only, denied, secret-era or unannounced event", permissions.RolePlayer, false, []string{"ev-visible"}},
		{"scribe is still not a Director", permissions.RoleScribe, false, []string{"ev-visible"}},
		{"owner sees every event of this page, in date order", permissions.RoleOwner, false,
			[]string{"ev-denied", "ev-dm", "ev-visible", "ev-ondaylater", "ev-secret"}},
		{"the block's hide setting drops dm-only events for the owner too", permissions.RoleOwner, true,
			[]string{"ev-denied", "ev-visible", "ev-ondaylater", "ev-secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PageTimeline(context.Background(), eraCampaignID, timelinePage, tt.hideDM, permissions.RequestViewer(tt.role, "u-1"))
			if err != nil {
				t.Fatalf("PageTimeline: %v", err)
			}
			var ids []string
			for _, e := range got.Events {
				ids = append(ids, e.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tt.want, ",") {
				t.Errorf("events = %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestPageTimeline_WhichCalendar(t *testing.T) {
	cal, _, _ := eraFixture()
	hidden := cal
	hidden.Visibility = "dm_only"
	tests := []struct {
		name     string
		cals     []Calendar
		def      *Calendar
		role     int
		notFound bool
	}{
		{"the default calendar", []Calendar{cal}, &cal, permissions.RolePlayer, false},
		{"the first calendar when none is the default", []Calendar{cal}, nil, permissions.RolePlayer, false},
		{"no calendar at all", nil, nil, permissions.RoleOwner, true},
		{"a calendar the player may not open", []Calendar{hidden}, &hidden, permissions.RolePlayer, true},
		{"the same calendar for its Director", []Calendar{hidden}, &hidden, permissions.RoleOwner, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTimelineService(tt.cals, tt.def, timelineEvents())
			got, err := svc.PageTimeline(context.Background(), eraCampaignID, timelinePage, false, permissions.RequestViewer(tt.role, "u-1"))
			var ae *apperror.AppError
			isNotFound := errors.As(err, &ae) && ae.Code == 404
			if tt.notFound != isNotFound {
				t.Fatalf("err = %v, want not found %v", err, tt.notFound)
			}
			if !tt.notFound && got.CalendarID != eraCalendarID {
				t.Errorf("calendar = %q, want %q", got.CalendarID, eraCalendarID)
			}
		})
	}
}

// day is a day number on eraFixture's calendar (12 months of 30 days).
func day(y, m, d int) int {
	cal, _, _ := eraFixture()
	cal.Months = eraMonths()
	return cal.AbsoluteDay(y, m, d)
}

func TestNewPageTimeline_Placement(t *testing.T) {
	cal, _, _ := eraFixture() // today is 1022-8-9
	cal.Months = eraMonths()
	ev := func(id string, y, m, d int) Event {
		return Event{ID: id, Name: id, Year: y, Month: m, Day: d, Visibility: "everyone"}
	}
	tests := []struct {
		name           string
		events         []Event
		start, end     int
		fill           float64
		firstAt, todAt float64
	}{
		{"past events run from the first to today", []Event{ev("a", 1020, 1, 1), ev("b", 1021, 1, 1)},
			day(1020, 1, 1), day(1022, 8, 9), 100, 0, 100},
		{"an event ahead stretches the line past today", []Event{ev("a", 1022, 8, 9), ev("b", 1022, 9, 9)},
			day(1022, 8, 9), day(1022, 9, 9), 0, 0, 0},
		{"only future events start the line at today", []Event{ev("a", 1023, 1, 1)},
			day(1022, 8, 9), day(1023, 1, 1), 0, 100, 0},
		{"one event on today sits in the middle", []Event{ev("a", 1022, 8, 9)},
			day(1022, 8, 9), day(1022, 8, 9), 50, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newPageTimeline(&cal, tt.events)
			if got.Start != tt.start || got.End != tt.end {
				t.Errorf("span = %d..%d, want %d..%d", got.Start, got.End, tt.start, tt.end)
			}
			if f := got.FillPercent(); f != tt.fill {
				t.Errorf("fill = %v, want %v", f, tt.fill)
			}
			if p := got.Percent(got.Events[0].Abs); p != tt.firstAt {
				t.Errorf("first dot at %v, want %v", p, tt.firstAt)
			}
			if p := got.Percent(got.Today); p != tt.todAt {
				t.Errorf("today at %v, want %v", p, tt.todAt)
			}
		})
	}
}

func TestNewPageTimeline_LabelsAndCap(t *testing.T) {
	cal, _, _ := eraFixture()
	cal.Months = eraMonths()
	var events []Event
	// 250 events, one a day from 1021-1-1: the line keeps the first 200.
	for i := 0; i < 250; i++ {
		y, rest := 1021+i/360, i%360
		events = append(events, Event{ID: "e", Year: y, Month: rest/30 + 1, Day: rest%30 + 1, Visibility: "everyone"})
	}
	got := newPageTimeline(&cal, events)
	if len(got.Events) != maxPageTimelineEvents || got.Total != 250 || !got.Truncated() {
		t.Fatalf("kept %d of %d (truncated %v), want %d of 250", len(got.Events), got.Total, got.Truncated(), maxPageTimelineEvents)
	}
	// Every event is in 1021, so only the first carries a year.
	labelled := 0
	for _, e := range got.Events {
		if e.Labelled {
			labelled++
		}
	}
	if labelled != 1 || !got.Events[0].Labelled {
		t.Errorf("labelled %d dots (first %v), want only the first", labelled, got.Events[0].Labelled)
	}

	// Years far enough apart each get a label; one crowding the last label
	// does not.
	got = newPageTimeline(&cal, []Event{
		{Year: 1000, Month: 1, Day: 1}, {Year: 1001, Month: 1, Day: 1}, {Year: 1011, Month: 1, Day: 1}, {Year: 1022, Month: 1, Day: 1},
	})
	want := []bool{true, false, true, true}
	for i, e := range got.Events {
		if e.Labelled != want[i] {
			t.Errorf("event %d (year %d) labelled = %v, want %v", i, e.Year, e.Labelled, want[i])
		}
	}
}

func TestPageTimelineBlock_Render(t *testing.T) {
	cal, _, _ := eraFixture()
	cal.Months = eraMonths()
	tl := newPageTimeline(&cal, []Event{
		{ID: "ev-1", Name: "The <harbour> charter", Year: 1020, Month: 1, Day: 1, Visibility: "everyone"},
		{ID: "ev-2", Name: "The coup", Year: 1021, Month: 5, Day: 3, Visibility: "dm_only", IsRecurring: true},
	})
	render := func(t *PageTimeline, canAuthor bool) string {
		var b bytes.Buffer
		if err := PageTimelineBlock("camp-1", t, canAuthor).Render(context.Background(), &b); err != nil {
			panic(err)
		}
		return b.String()
	}

	html := render(tl, true)
	for _, want := range []string{
		`data-widget="page_timeline"`,
		`href="/campaigns/camp-1/calendars/` + eraCalendarID + `/view?event=ev-1&amp;date=1020-1-1"`,
		`The &lt;harbour&gt; charter`,
		`data-dm-only`,
		`data-ptl-repeats`,
		`left:0.00%`,
		`--i:1`,
		`Today is MH 9, 1022.`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("block lacks %q\n%s", want, html)
		}
	}
	if strings.Contains(html, "<harbour>") {
		t.Error("an event name was not escaped")
	}

	empty := newPageTimeline(&cal, nil)
	if got := render(empty, false); strings.Contains(got, "<section") {
		t.Errorf("a player's empty line should draw nothing, got %s", got)
	}
	if got := render(empty, true); !strings.Contains(got, "No dated events belong to this page yet.") {
		t.Errorf("the Director's empty line lacks its note: %s", got)
	}
	if got := render(nil, false); strings.Contains(got, "<section") {
		t.Errorf("no calendar should draw nothing for a player, got %s", got)
	}
}

// TestEventRepository_ListEventsForPage_Integration pins the page read
// against a real database: an event linked by its page column, one tied
// through entity_event_links, one linked both ways (still one row), and
// none from another page or another calendar; dm_only only for a Director.
func TestEventRepository_ListEventsForPage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	eventRepo := NewEventRepository(db)
	calRepo := NewCalendarRepository(db)

	fix := newTestCampaign(t, db, "pagetl")
	page := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Highwater")
	other := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Elsewhere")
	calA := newTestCalendar(testUUID(t), fix.CampaignID, "Calendar A")
	calB := newTestCalendar(testUUID(t), fix.CampaignID, "Calendar B")
	for _, c := range []*Calendar{calA, calB} {
		if err := calRepo.Create(ctx, c); err != nil {
			t.Fatalf("Create calendar: %v", err)
		}
	}
	mk := func(cal *Calendar, name string, year int, entityID *string, vis string) string {
		e := &Event{ID: testUUID(t), CalendarID: cal.ID, EntityID: entityID, Name: name, Year: year, Month: 1, Day: 1, Visibility: vis, AllDay: true}
		if err := eventRepo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		return e.ID
	}
	byColumn := mk(calA, "by column", 3, &page, "everyone")
	tied := mk(calA, "tied", 1, nil, "everyone")
	both := mk(calA, "both", 2, &page, "everyone")
	secret := mk(calA, "secret", 4, &page, "dm_only")
	mk(calA, "other page", 5, &other, "everyone")
	otherCal := mk(calB, "other calendar", 6, &page, "everyone")
	for _, id := range []string{tied, both, otherCal} {
		if err := eventRepo.LinkEntityEvent(ctx, page, id, string(RoleInvolved)); err != nil {
			t.Fatalf("LinkEntityEvent: %v", err)
		}
	}

	tests := []struct {
		name string
		role int
		want []string
	}{
		{"a player gets this page's events in date order", permissions.RolePlayer, []string{tied, both, byColumn}},
		{"a Director also gets the dm-only one", permissions.RoleOwner, []string{tied, both, byColumn, secret}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := eventRepo.ListEventsForPage(ctx, calA.ID, page, tt.role)
			if err != nil {
				t.Fatalf("ListEventsForPage: %v", err)
			}
			var ids []string
			for _, e := range got {
				ids = append(ids, e.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tt.want, ",") {
				t.Errorf("events = %v, want %v", ids, tt.want)
			}
		})
	}
}
