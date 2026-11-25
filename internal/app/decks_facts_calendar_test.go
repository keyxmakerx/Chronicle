package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
)

// decksCalendar is a main calendar on month 4, day 1 at noon with a moon every
// viewer sees and one hidden from players, filtered by role the way the
// calendar service filters it.
type decksCalendar struct {
	calendar.CalendarService
	none    bool
	events  []calendar.Event
	kinds   []calendar.EventKind
	viewers []permissions.Viewer
}

func (f *decksCalendar) GetDefaultCalendarForViewer(_ context.Context, _ string, v permissions.Viewer) (*calendar.Calendar, error) {
	f.viewers = append(f.viewers, v)
	if f.none {
		return nil, apperror.NewNotFound("calendar not found")
	}
	months := make([]calendar.Month, 12)
	for i := range months {
		months[i] = calendar.Month{Name: "M", Days: 30}
	}
	c := &calendar.Calendar{ID: "cal-1", HoursPerDay: 24, MinutesPerHour: 60, Months: months,
		CurrentYear: 1, CurrentMonth: 4, CurrentDay: 1, CurrentHour: 12}
	c.Moons = []calendar.Moon{{Name: " Nyx ", CycleDays: 30}, {Name: "Veil", CycleDays: 10, HiddenFromPlayers: true}}
	if !v.SkipsPerUserRules() {
		c.Moons = c.Moons[:1]
	}
	return c, nil
}

func (f *decksCalendar) ListEventsForMonth(_ context.Context, _, _ string, _, _ int, _ permissions.Viewer) ([]calendar.Event, error) {
	return f.events, nil
}

func (f *decksCalendar) ListEventKinds(context.Context, string) ([]calendar.EventKind, error) {
	return f.kinds, nil
}

func TestDecksFactsTodayReadsTheCalendarAsTheViewerSeesIt(t *testing.T) {
	ctx := context.Background()
	kind := 5
	cal := &decksCalendar{
		events: []calendar.Event{{Name: " Bloodnight ", Year: 1, Month: 4, Day: 1, KindID: &kind}, {Name: "Harvest", Year: 1, Month: 4, Day: 9}},
		kinds:  []calendar.EventKind{{ID: kind, Slug: "omen", Name: "Red Omen"}},
	}
	a := &decksFactsAdapter{calendars: cal}

	dir, err := a.Today(ctx, "camp", decks.Viewer{Director: true, MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	if dir.DayPart != "day" || len(dir.Moons) != 2 || dir.Moons["nyx"].Name != "Nyx" || len(dir.Moons["nyx"].Words) == 0 {
		t.Errorf("the Director's day: %+v", dir)
	}
	for _, want := range []string{"bloodnight", "omen", "red omen"} {
		if !dir.Events[want] {
			t.Errorf("today's events lack %q: %v", want, dir.Events)
		}
	}
	if dir.Events["harvest"] {
		t.Error("an event on another day is today")
	}

	pl, err := a.Today(ctx, "camp", decks.Viewer{UserID: "u-tam", MemberRole: permissions.RolePlayer, VisibilityRole: permissions.RolePlayer})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pl.Moons["veil"]; ok || len(pl.Moons) != 1 {
		t.Errorf("a player's day shows a hidden moon: %+v", pl.Moons)
	}
	if !pl.Events["bloodnight"] || pl.Events["omen"] || pl.Events["red omen"] {
		t.Errorf("a player's day must name the event but not its kind: %v", pl.Events)
	}
	last := cal.viewers[len(cal.viewers)-1]
	if last.Role() != permissions.RolePlayer || last.UserID() != "u-tam" {
		t.Errorf("the player's calendar was read as role %d, user %q", last.Role(), last.UserID())
	}
}

func TestDecksFactsTodayWithoutACalendarIsUnavailable(t *testing.T) {
	ctx := context.Background()
	for name, a := range map[string]*decksFactsAdapter{
		"the calendar plugin is down": {},
		"no main calendar":            {calendars: &decksCalendar{none: true}},
	} {
		if _, err := a.Today(ctx, "camp", decks.Viewer{Director: true, VisibilityRole: permissions.RoleOwner}); !errors.Is(err, decks.ErrUnavailable) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestDecksCalendarOptionsOfferEveryMoonAndKind(t *testing.T) {
	ctx := context.Background()
	a := &decksFactsAdapter{calendars: &decksCalendar{kinds: []calendar.EventKind{{ID: 1, Slug: "omen", Name: "Red Omen"}}}}
	var o decks.WhenOptions
	a.calendarOptions(ctx, "camp", &o)
	if !o.Calendar || len(o.Moons) != 2 || o.Moons[0] != "Nyx" || o.Moons[1] != "Veil" {
		t.Errorf("moons: %v (calendar %v)", o.Moons, o.Calendar)
	}
	if len(o.EventKinds) != 1 || o.EventKinds[0] != (decks.EventKindOption{Slug: "omen", Name: "Red Omen"}) {
		t.Errorf("kinds: %v", o.EventKinds)
	}
	var none decks.WhenOptions
	(&decksFactsAdapter{calendars: &decksCalendar{none: true}}).calendarOptions(ctx, "camp", &none)
	if none.Calendar {
		t.Error("a campaign with no calendar offers one")
	}
}

// Worked-out fields read the same day through the shared reader: moons by
// lower-cased name, today's events and kinds, the time of day, and a player's
// day without the hidden moon.
func TestFieldRulesTodaySharesTheCalendarReader(t *testing.T) {
	ctx := context.Background()
	kind := 5
	w := &fieldRulesWorld{calendars: &decksCalendar{
		events: []calendar.Event{{Name: "Bloodnight", Year: 1, Month: 4, Day: 1, KindID: &kind}},
		kinds:  []calendar.EventKind{{ID: kind, Slug: "omen", Name: "Red Omen"}},
	}}
	dm := w.Today(ctx, "camp", fieldrules.Viewer{Role: permissions.RoleOwner})
	if dm.DayPart != "day" || len(dm.Moons) != 2 || dm.Moons["nyx"].Words == nil || !dm.EventTags["bloodnight"] || !dm.EventTags["omen"] || !dm.EventTags["red omen"] {
		t.Errorf("the Director's situation: %+v", dm)
	}
	if pl := w.Today(ctx, "camp", fieldrules.Viewer{Role: permissions.RolePlayer, UserID: "u-tam"}); len(pl.Moons) != 1 || pl.EventTags["omen"] || !pl.EventTags["bloodnight"] {
		t.Errorf("a player's situation shows a hidden moon or an event's kind: %+v", pl)
	}
	if none := (&fieldRulesWorld{calendars: &decksCalendar{none: true}}).Today(ctx, "camp", fieldrules.Viewer{Role: permissions.RoleOwner}); none.Moons != nil || none.EventTags != nil {
		t.Errorf("no calendar must read as missing facts: %+v", none)
	}
}

type recordingCalendarPublisher struct{ types []string }

func (r *recordingCalendarPublisher) PublishCalendarEvent(eventType, _, _ string, _ any) {
	r.types = append(r.types, eventType)
}

func TestDecksCalendarEventsWakeTheWatcher(t *testing.T) {
	tests := []struct {
		eventType string
		wake      bool
	}{
		{calendar.PubDateAdvanced, true}, {calendar.PubEventCreated, true}, {calendar.PubEventUpdated, true},
		{calendar.PubEventDeleted, true}, {calendar.PubStructureUpdated, true}, {calendar.PubMoonPhaseChanged, true},
		{calendar.PubWeatherChanged, false}, {calendar.PubSeasonChanged, false},
	}
	for _, tt := range tests {
		next := &recordingCalendarPublisher{}
		var poked []string
		p := &decksCalendarEvents{next: next, poke: func(c string) { poked = append(poked, c) }}
		p.PublishCalendarEvent(tt.eventType, "camp", "cal-1", nil)
		if len(next.types) != 1 || next.types[0] != tt.eventType {
			t.Errorf("%s: the real publisher got %v", tt.eventType, next.types)
		}
		if (len(poked) == 1 && poked[0] == "camp") != tt.wake {
			t.Errorf("%s: poked %v, want a wake %v", tt.eventType, poked, tt.wake)
		}
	}
}
