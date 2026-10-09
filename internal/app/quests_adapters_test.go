package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
)

// testQuestCalendar is a two-month calendar with a leap day in the first
// month every 4 years (offset 0); today is Frost 10, year 100.
func testQuestCalendar() *calendar.Calendar {
	return &calendar.Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Harvest Reckoning",
		CurrentYear: 100, CurrentMonth: 1, CurrentDay: 10,
		LeapYearEvery: 4, LeapYearOffset: 0,
		Months: []calendar.Month{{Name: "Frost", Days: 30, LeapYearDays: 1}, {Name: "Bloom", Days: 20}},
	}
}

// stubQuestCalendarSvc embeds the interface so only the calls the adapter
// makes need bodies; any other call panics, which the tests would show.
type stubQuestCalendarSvc struct {
	calendar.CalendarService
	cal       *calendar.Calendar
	calErr    error
	events    map[string]*calendar.Event
	updateErr error
	created   []calendar.CreateEventInput
	updated   []calendar.UpdateEventInput
	deleted   []string
	visSet    []string
}

func (s *stubQuestCalendarSvc) GetPrimaryCalendarForViewer(_ context.Context, _ string, v permissions.Viewer) (*calendar.Calendar, error) {
	if !v.IsSystem() {
		return nil, errors.New("expected the system viewer")
	}
	return s.cal, s.calErr
}

func (s *stubQuestCalendarSvc) CreateEvent(_ context.Context, _, _ string, in calendar.CreateEventInput) (*calendar.Event, error) {
	s.created = append(s.created, in)
	e := &calendar.Event{ID: "ev-new", Visibility: in.Visibility}
	if s.events == nil {
		s.events = map[string]*calendar.Event{}
	}
	s.events[e.ID] = e
	return e, nil
}

func (s *stubQuestCalendarSvc) UpdateEvent(_ context.Context, id, _, _ string, in calendar.UpdateEventInput, _ permissions.Viewer) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if _, ok := s.events[id]; !ok {
		return apperror.NewNotFound("event not found")
	}
	s.updated = append(s.updated, in)
	return nil
}

func (s *stubQuestCalendarSvc) GetEventForViewer(_ context.Context, id, _, _ string, _ permissions.Viewer) (*calendar.Event, error) {
	if e, ok := s.events[id]; ok {
		return e, nil
	}
	return nil, apperror.NewNotFound("event not found")
}

func (s *stubQuestCalendarSvc) SetEventVisibility(_ context.Context, id, _, _ string, in calendar.UpdateEventVisibilityInput, _ permissions.Viewer) error {
	s.visSet = append(s.visSet, id+":"+in.Visibility)
	s.events[id].Visibility = in.Visibility
	return nil
}

func (s *stubQuestCalendarSvc) DeleteEvent(_ context.Context, id, _, _ string, _ permissions.Viewer) error {
	if _, ok := s.events[id]; !ok {
		return apperror.NewNotFound("event not found")
	}
	s.deleted = append(s.deleted, id)
	delete(s.events, id)
	return nil
}

type stubAddons struct {
	on  bool
	err error
}

func (a stubAddons) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return a.on, a.err
}

func TestQuestCalendarAdapterCalendar(t *testing.T) {
	tests := []struct {
		name    string
		addons  stubAddons
		svc     *stubQuestCalendarSvc
		wantCal bool
		wantErr bool
	}{
		{"calendar and addon on", stubAddons{on: true}, &stubQuestCalendarSvc{cal: testQuestCalendar()}, true, false},
		{"addon off is no calendar", stubAddons{on: false}, &stubQuestCalendarSvc{cal: testQuestCalendar()}, false, false},
		{"campaign without a calendar", stubAddons{on: true}, &stubQuestCalendarSvc{calErr: apperror.NewNotFound("calendar not found")}, false, false},
		{"a broken lookup is an error", stubAddons{on: true}, &stubQuestCalendarSvc{calErr: errors.New("db down")}, false, true},
		{"a broken addon check is an error", stubAddons{err: errors.New("db down")}, &stubQuestCalendarSvc{cal: testQuestCalendar()}, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &questCalendarAdapter{svc: tc.svc, addons: tc.addons}
			cal, err := a.Calendar(context.Background(), "camp-1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if (cal != nil) != tc.wantCal {
				t.Fatalf("calendar = %v, want present=%v", cal, tc.wantCal)
			}
		})
	}
}

func TestQuestCalendarDateMaths(t *testing.T) {
	c := &questCalendar{cal: testQuestCalendar()}
	valid := []struct {
		name string
		d    quests.DueDay
		want bool
	}{
		{"today", quests.DueDay{Year: 100, Month: 1, Day: 10}, true},
		{"leap day in a leap year", quests.DueDay{Year: 104, Month: 1, Day: 31}, true},
		{"leap day in a common year", quests.DueDay{Year: 103, Month: 1, Day: 31}, false},
		{"last day of Bloom", quests.DueDay{Year: 103, Month: 2, Day: 20}, true},
		{"past the end of Bloom", quests.DueDay{Year: 103, Month: 2, Day: 21}, false},
		{"month zero", quests.DueDay{Year: 100, Month: 0, Day: 1}, false},
		{"month past the last", quests.DueDay{Year: 100, Month: 3, Day: 1}, false},
		{"day zero", quests.DueDay{Year: 100, Month: 1, Day: 0}, false},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if got := c.Valid(tc.d); got != tc.want {
				t.Fatalf("Valid(%+v) = %v, want %v", tc.d, got, tc.want)
			}
		})
	}

	days := []struct {
		name string
		d    quests.DueDay
		want int
	}{
		{"today", quests.DueDay{Year: 100, Month: 1, Day: 10}, 0},
		{"tomorrow", quests.DueDay{Year: 100, Month: 1, Day: 11}, 1},
		{"yesterday", quests.DueDay{Year: 100, Month: 1, Day: 9}, -1},
		// Year 100 is a leap year: Frost has 31 days, so Bloom 1 is 22 days on.
		{"first of next month", quests.DueDay{Year: 100, Month: 2, Day: 1}, 22},
		// 51 days in leap year 100, then 50 in 101.
		{"a year on", quests.DueDay{Year: 101, Month: 1, Day: 10}, 51},
		{"long ago", quests.DueDay{Year: 99, Month: 1, Day: 10}, -50},
	}
	for _, tc := range days {
		t.Run("daysLeft/"+tc.name, func(t *testing.T) {
			if got := c.DaysFromToday(tc.d); got != tc.want {
				t.Fatalf("DaysFromToday(%+v) = %d, want %d", tc.d, got, tc.want)
			}
		})
	}

	if got := c.Label(quests.DueDay{Year: 104, Month: 2, Day: 3}); got != "Bloom 3, 104" {
		t.Fatalf("label = %q", got)
	}
	if got := (&questCalendar{cal: testQuestCalendar()}).Today(); got != (quests.DueDay{Year: 100, Month: 1, Day: 10}) {
		t.Fatalf("today = %+v", got)
	}
	// Label works on a copy, so asking for another day never moves today.
	if c.cal.CurrentYear != 100 || c.cal.CurrentDay != 10 {
		t.Fatal("Label moved the calendar's current date")
	}
	every, offset := c.Leap()
	if every != 4 || offset != 0 || len(c.Months()) != 2 || c.Months()[0].LeapDays != 1 {
		t.Fatalf("leap/months wrong: %d %d %+v", every, offset, c.Months())
	}
}

func TestQuestCalendarAdapterEvents(t *testing.T) {
	ctx := context.Background()
	qc := &questCalendar{cal: testQuestCalendar()}
	day := quests.DueDay{Year: 100, Month: 2, Day: 3}

	t.Run("create sets the brief's fields", func(t *testing.T) {
		svc := &stubQuestCalendarSvc{}
		a := &questCalendarAdapter{svc: svc}
		id, err := a.SaveDueEvent(ctx, "camp-1", qc, "", quests.DueEvent{EntityID: "q1", Title: "Due: Goat", Day: day, DMOnly: true, CreatedBy: "u1"})
		if err != nil || id != "ev-new" || len(svc.created) != 1 {
			t.Fatalf("id=%q err=%v created=%d", id, err, len(svc.created))
		}
		in := svc.created[0]
		if in.Name != "Due: Goat" || *in.EntityID != "q1" || in.Year != 100 || in.Month != 2 || in.Day != 3 || !in.AllDay ||
			in.Visibility != "dm_only" || in.CreatedBy != "u1" || !in.CanAuthorDmOnly || !in.Author.IsSystem() || in.Announced == nil || *in.Announced != calendar.AnnouncedAhead {
			t.Fatalf("create input wrong: %+v", in)
		}
	})

	t.Run("update changes only the fields it owns", func(t *testing.T) {
		svc := &stubQuestCalendarSvc{events: map[string]*calendar.Event{"ev-1": {ID: "ev-1"}}}
		a := &questCalendarAdapter{svc: svc}
		id, err := a.SaveDueEvent(ctx, "camp-1", qc, "ev-1", quests.DueEvent{EntityID: "q1", Title: "Due: Goat", Day: day})
		if err != nil || id != "ev-1" || len(svc.created) != 0 || len(svc.updated) != 1 {
			t.Fatalf("id=%q err=%v", id, err)
		}
		u := svc.updated[0]
		if v, _ := u.Visibility.Get(); v != "everyone" {
			t.Fatalf("visibility = %q", v)
		}
		for name, absent := range map[string]bool{
			"description": !u.Description.Present(), "recurrence": !u.IsRecurring.Present(), "color": !u.Color.Present(),
			"icon": !u.Icon.Present(), "kind": !u.KindID.Present(), "visibility rules": !u.VisibilityRules.Present(), "announced": !u.Announced.Present(),
		} {
			if !absent {
				t.Fatalf("update must leave %s alone", name)
			}
		}
	})

	t.Run("a gone event is created again", func(t *testing.T) {
		svc := &stubQuestCalendarSvc{}
		a := &questCalendarAdapter{svc: svc}
		id, err := a.SaveDueEvent(ctx, "camp-1", qc, "ev-gone", quests.DueEvent{EntityID: "q1", Title: "Due: Goat", Day: day})
		if err != nil || id != "ev-new" || len(svc.created) != 1 {
			t.Fatalf("id=%q err=%v", id, err)
		}
	})

	t.Run("another update error is returned, not papered over", func(t *testing.T) {
		svc := &stubQuestCalendarSvc{updateErr: errors.New("db down")}
		a := &questCalendarAdapter{svc: svc}
		if _, err := a.SaveDueEvent(ctx, "camp-1", qc, "ev-1", quests.DueEvent{Title: "x", Day: day}); err == nil || len(svc.created) != 0 {
			t.Fatalf("err=%v created=%d", err, len(svc.created))
		}
	})

	t.Run("delete: a missing event is not an error", func(t *testing.T) {
		svc := &stubQuestCalendarSvc{events: map[string]*calendar.Event{"ev-1": {ID: "ev-1"}}}
		a := &questCalendarAdapter{svc: svc}
		if err := a.DeleteDueEvent(ctx, "camp-1", qc, "ev-1"); err != nil || len(svc.deleted) != 1 {
			t.Fatalf("err=%v", err)
		}
		if err := a.DeleteDueEvent(ctx, "camp-1", qc, "ev-1"); err != nil {
			t.Fatalf("second delete: %v", err)
		}
	})

	visTests := []struct {
		name    string
		stored  string
		dmOnly  bool
		wantSet []string
	}{
		{"hide a visible event", "everyone", true, []string{"ev-1:dm_only"}},
		{"show a hidden event", "dm_only", false, []string{"ev-1:everyone"}},
		{"already hidden", "dm_only", true, nil},
		{"already visible", "everyone", false, nil},
	}
	for _, tc := range visTests {
		t.Run("visibility/"+tc.name, func(t *testing.T) {
			svc := &stubQuestCalendarSvc{events: map[string]*calendar.Event{"ev-1": {ID: "ev-1", Visibility: tc.stored}}}
			a := &questCalendarAdapter{svc: svc}
			if err := a.SetDueEventVisibility(ctx, "camp-1", qc, "ev-1", tc.dmOnly); err != nil {
				t.Fatal(err)
			}
			if len(svc.visSet) != len(tc.wantSet) || (len(tc.wantSet) == 1 && svc.visSet[0] != tc.wantSet[0]) {
				t.Fatalf("visibility writes = %v, want %v", svc.visSet, tc.wantSet)
			}
		})
	}
	t.Run("visibility of a missing event is a no-op", func(t *testing.T) {
		a := &questCalendarAdapter{svc: &stubQuestCalendarSvc{}}
		if err := a.SetDueEventVisibility(ctx, "camp-1", qc, "nope", true); err != nil {
			t.Fatal(err)
		}
	})
}

type recordingEntityPublisher struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingEntityPublisher) PublishEntityEvent(eventType, _, _ string, _ *entities.Entity) {
	r.mu.Lock()
	r.events = append(r.events, eventType)
	r.mu.Unlock()
}
func (r *recordingEntityPublisher) PublishEntityTypeEvent(string, string, *entities.EntityType) {}

type fakeDueSyncer struct {
	calls chan string
	err   error
	boom  bool
}

func (f *fakeDueSyncer) SyncDueEvent(_ context.Context, _, id string) error {
	f.calls <- "sync:" + id
	if f.boom {
		panic("boom")
	}
	return f.err
}

func (f *fakeDueSyncer) RemoveDueEvent(_ context.Context, _, id string) error {
	f.calls <- "remove:" + id
	return f.err
}

func TestQuestEntityEventsFanOut(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		campaign  string
		want      string // "" = quests not called
		syncerErr error
		boom      bool
	}{
		{"update syncs", "updated", "camp-1", "sync:e1", nil, false},
		{"delete removes", "deleted", "camp-1", "remove:e1", nil, false},
		{"create is ignored", "created", "camp-1", "", nil, false},
		{"no campaign is ignored", "updated", "", "", nil, false},
		{"a failing sync is only logged", "updated", "camp-1", "sync:e1", errors.New("calendar down"), false},
		{"a panicking sync is contained", "updated", "camp-1", "sync:e1", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next := &recordingEntityPublisher{}
			p := newQuestEntityEvents(next)
			syncer := &fakeDueSyncer{calls: make(chan string, 4), err: tc.syncerErr, boom: tc.boom}
			p.attach(syncer)
			p.PublishEntityEvent(tc.eventType, tc.campaign, "e1", &entities.Entity{ID: "e1"})

			// The websocket publisher always sees the event first and unchanged.
			if len(next.events) != 1 || next.events[0] != tc.eventType {
				t.Fatalf("next publisher got %v", next.events)
			}
			select {
			case got := <-syncer.calls:
				if got != tc.want {
					t.Fatalf("quests got %q, want %q", got, tc.want)
				}
			case <-time.After(300 * time.Millisecond):
				if tc.want != "" {
					t.Fatalf("quests not called, want %q", tc.want)
				}
			}
		})
	}

	t.Run("works before the quests service is attached", func(t *testing.T) {
		next := &recordingEntityPublisher{}
		p := newQuestEntityEvents(next)
		p.PublishEntityEvent("updated", "camp-1", "e1", nil)
		p.PublishEntityTypeEvent("updated", "camp-1", nil)
		time.Sleep(20 * time.Millisecond) // let the background sync run; it must not panic
		if len(next.events) != 1 {
			t.Fatalf("next got %v", next.events)
		}
	})
}
