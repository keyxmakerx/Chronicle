package sessions

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func ptr[T any](v T) *T { return &v }

func TestEventHasNight(t *testing.T) {
	ev := SessionEvent{ID: "e1", Name: "Session 1", CalendarID: "cal", Year: 1492, Month: 3, Day: 14}
	real := SessionEvent{ID: "e2", Name: "Session 2", CalendarID: "real", Year: 2026, Month: 10, Day: 24, RealDate: "2026-10-24"}
	tests := []struct {
		name  string
		ev    SessionEvent
		night Session
		want  bool
	}{
		{"same name", ev, Session{Name: "Session 1"}, true},
		{"same name, other case and spacing", ev, Session{Name: "  session   1 "}, true},
		{"other name", ev, Session{Name: "Session 2"}, false},
		{"same story day on the same calendar", ev, Session{Name: "The siege", CalendarID: ptr("cal"), CalendarYear: ptr(1492), CalendarMonth: ptr(3), CalendarDay: ptr(14)}, true},
		{"same story day on another calendar", ev, Session{Name: "The siege", CalendarID: ptr("other"), CalendarYear: ptr(1492), CalendarMonth: ptr(3), CalendarDay: ptr(14)}, false},
		{"another story day", ev, Session{Name: "The siege", CalendarID: ptr("cal"), CalendarYear: ptr(1492), CalendarMonth: ptr(3), CalendarDay: ptr(15)}, false},
		{"real date matches a real-life event", real, Session{Name: "Friday", ScheduledDate: ptr("2026-10-24")}, true},
		{"real date differs", real, Session{Name: "Friday", ScheduledDate: ptr("2026-10-25")}, false},
		{"a story event is not matched by a real date", ev, Session{Name: "Friday", ScheduledDate: ptr("2026-10-24")}, false},
		{"a cancelled night still stands for it", ev, Session{Name: "Session 1", Status: StatusCancelled}, true},
		{"an event with no name matches nothing by name", SessionEvent{Name: " "}, Session{Name: ""}, false},
		{"a night linked to another event is not matched by name", ev, Session{Name: "Session 1", CalendarEventID: ptr("other")}, false},
		{"a night linked to another event is not matched by day", ev, Session{Name: "The siege", CalendarEventID: ptr("other"), CalendarID: ptr("cal"), CalendarYear: ptr(1492), CalendarMonth: ptr(3), CalendarDay: ptr(14)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventHasNight(tt.ev, []Session{tt.night}, nil); got != tt.want {
				t.Fatalf("eventHasNight = %v, want %v", got, tt.want)
			}
		})
	}
}

// A stored link answers "has a night" even with no night in the list (one in
// the Trash is not listed), whatever the night is now called.
func TestEventHasNightByStoredLink(t *testing.T) {
	ev := SessionEvent{ID: "e1", Name: "Session 1"}
	if !eventHasNight(ev, nil, map[string]bool{"e1": true}) {
		t.Fatal("a linked event was offered again")
	}
	if eventHasNight(ev, nil, map[string]bool{"e2": true}) {
		t.Fatal("another event's link hid this one")
	}
	renamed := Session{Name: "The siege of Hammerfall", CalendarEventID: ptr("e1")}
	if !eventHasNight(ev, []Session{renamed}, map[string]bool{"e1": true}) {
		t.Fatal("a renamed night no longer stands for its event")
	}
}

func TestEventsWithoutNightKeepsOrder(t *testing.T) {
	events := []SessionEvent{{ID: "a", Name: "One"}, {ID: "b", Name: "Two"}, {ID: "c", Name: "Three"}, {ID: "d", Name: "Four"}}
	got := eventsWithoutNight(events, []Session{{Name: "two"}}, map[string]bool{"d": true})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("got %+v: the events that have a night are left out", got)
	}
}

func TestEventNightInput(t *testing.T) {
	tests := []struct {
		name     string
		ev       SessionEvent
		wantReal string
		wantY    int
	}{
		{"a story event gives the in-world date and no real date", SessionEvent{Name: "Session 1", Year: 1492, Month: 3, Day: 14}, "", 1492},
		{"a real-life event gives the real date and no in-world date", SessionEvent{Name: "Session 2", Year: 2026, Month: 10, Day: 24, RealDate: "2026-10-24"}, "2026-10-24", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := eventNightInput(tt.ev, "u1")
			if in.Name != tt.ev.Name || in.CreatedBy != "u1" {
				t.Fatalf("input = %+v", in)
			}
			if in.CalendarEventID == nil || *in.CalendarEventID != tt.ev.ID {
				t.Fatalf("the night does not carry its event: %v", in.CalendarEventID)
			}
			if tt.wantReal == "" && in.ScheduledDate != nil {
				t.Fatalf("a real date was invented: %q", *in.ScheduledDate)
			}
			if tt.wantReal != "" && (in.ScheduledDate == nil || *in.ScheduledDate != tt.wantReal) {
				t.Fatalf("scheduled date = %v, want %s", in.ScheduledDate, tt.wantReal)
			}
			if (tt.wantY == 0) != (in.CalendarYear == nil) || (tt.wantY != 0 && *in.CalendarYear != tt.wantY) {
				t.Fatalf("calendar year = %v, want %d", in.CalendarYear, tt.wantY)
			}
		})
	}
}

type fakeMembers struct{ ids []string }

func (f fakeMembers) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	out := make([]campaigns.CampaignMember, len(f.ids))
	for i, id := range f.ids {
		out[i] = campaigns.CampaignMember{UserID: id}
	}
	return out, nil
}

func TestMakeNightFromEvent(t *testing.T) {
	ev := SessionEvent{ID: "e1", Name: "Session 1", CalendarID: "cal", Year: 1492, Month: 3, Day: 14, EntityID: "page-1"}
	tests := []struct {
		name     string
		existing []Session
		linked   []string
		wantErr  int
		wantMade bool
	}{
		{"makes the night", nil, nil, 0, true},
		{"a second press makes none", []Session{{Name: "Session 1"}}, nil, http.StatusConflict, false},
		{"a night in the Trash holds the event", nil, []string{"e1"}, http.StatusConflict, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var made *Session
			var invited []string
			var linked [][3]string
			repo := &mockSessionRepo{
				listByCampaignFn:     func(context.Context, string) ([]Session, error) { return tt.existing, nil },
				listLinkedEventIDsFn: func(context.Context, string) ([]string, error) { return tt.linked, nil },
				createFn:             func(_ context.Context, _ string, s *Session) error { made = s; return nil },
				addAttendeeFn: func(_ context.Context, _, userID, status string) error {
					invited = append(invited, userID+":"+status)
					return nil
				},
				linkEntityFn: func(_ context.Context, sid, eid, role string) error {
					linked = append(linked, [3]string{sid, eid, role})
					return nil
				},
			}
			svc := newTestSessionService(repo)
			s, err := makeNightFromEvent(context.Background(), svc, fakeMembers{ids: []string{"u1", "u2"}}, "camp", "u1", ev)
			if tt.wantErr != 0 {
				assertAppError(t, err, tt.wantErr)
				if made != nil {
					t.Fatal("a night was made anyway")
				}
				return
			}
			if err != nil || !tt.wantMade || s == nil || made == nil {
				t.Fatalf("s=%v err=%v", s, err)
			}
			if made.CalendarEventID == nil || *made.CalendarEventID != "e1" {
				t.Fatalf("the night is not linked to its event: %v", made.CalendarEventID)
			}
			if made.Name != "Session 1" || made.Status != StatusPlanned || made.ScheduledDate != nil {
				t.Fatalf("made %+v: planned, named for the event, with no real date", made)
			}
			if made.CalendarYear == nil || *made.CalendarYear != 1492 || *made.CalendarMonth != 3 || *made.CalendarDay != 14 {
				t.Fatalf("in-world date not carried: %+v", made)
			}
			if len(invited) != 2 || invited[0] != "u1:invited" {
				t.Fatalf("invited %v: every member is asked", invited)
			}
			if len(linked) != 1 || linked[0] != [3]string{s.ID, "page-1", EntityRoleKey} {
				t.Fatalf("linked %v", linked)
			}
		})
	}
}

// The calendar's message says "no real date yet" only when the reply has none.
func TestMadeNightBodyCarriesTheRealDate(t *testing.T) {
	day := "2026-10-29"
	tests := []struct {
		name string
		s    Session
		want string
	}{
		{"a story calendar's night has no real date", Session{ID: "s1", Name: "A"}, ""},
		{"a real-life calendar's night has one", Session{ID: "s2", Name: "B", ScheduledDate: &day}, day},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := madeNightBody(&tt.s)
			if body["scheduledDate"] != tt.want || body["id"] != tt.s.ID || body["name"] != tt.s.Name {
				t.Fatalf("body %v, want scheduledDate %q", body, tt.want)
			}
		})
	}
}

// Three simultaneous presses can all pass the read; the unique key refuses the
// later inserts, and that refusal has to reach the caller as a conflict, not a
// server fault, and must not go on to invite anyone or link a page.
func TestMakeNightFromEventRaceLoserGetsConflict(t *testing.T) {
	ev := SessionEvent{ID: "e1", Name: "Session 1", CalendarID: "cal", Year: 1492, Month: 3, Day: 14, EntityID: "page-1"}
	invited, linked := 0, 0
	repo := &mockSessionRepo{
		createFn: func(context.Context, string, *Session) error {
			return apperror.NewConflict("this event already has a game night")
		},
		addAttendeeFn: func(context.Context, string, string, string) error { invited++; return nil },
		linkEntityFn:  func(context.Context, string, string, string) error { linked++; return nil },
	}
	_, err := makeNightFromEvent(context.Background(), newTestSessionService(repo), fakeMembers{ids: []string{"u1"}}, "camp", "u1", ev)
	assertAppError(t, err, http.StatusConflict)
	if invited != 0 || linked != 0 {
		t.Fatalf("the losing request still invited %d and linked %d", invited, linked)
	}
}
