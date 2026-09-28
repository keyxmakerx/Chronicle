// events_eras_for_calendar_test.go exercises ListEventsForCalendar and
// ListErasForCalendar against a real MariaDB: these back timeline's event
// picker and era bands, so every read must stay inside the calendar's own
// campaign and respect role. Skipped under `-short`.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestListEventsAndErasForCalendar_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	calRepo := NewCalendarRepository(db)
	eventRepo := NewEventRepository(db)
	svc := NewCalendarService(calRepo, eventRepo, NewEventKindRepository(db), NewWeatherRepository(db))
	svc.(*calendarService).SetEntityVisibilityGate(testEntityGate{db: db})

	fixA := newTestCampaign(t, db, "evtera-a")
	fixB := newTestCampaign(t, db, "evtera-b")

	open := newTestCalendar(testUUID(t), fixA.CampaignID, "Open Calendar")
	if err := calRepo.Create(ctx, open); err != nil {
		t.Fatalf("create open calendar: %v", err)
	}
	hidden := newTestCalendar(testUUID(t), fixA.CampaignID, "Hidden Calendar")
	hidden.Visibility = "dm_only"
	if err := calRepo.Create(ctx, hidden); err != nil {
		t.Fatalf("create hidden calendar: %v", err)
	}

	mustEvent := func(cal *Calendar, name, visibility string) {
		t.Helper()
		evt := &Event{
			ID: testUUID(t), CalendarID: cal.ID, Name: name,
			Year: 1, Month: 1, Day: 1, Visibility: visibility, AllDay: true,
		}
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("create event %q: %v", name, err)
		}
	}
	mustEvent(open, "Harvest Fair", "everyone")
	mustEvent(open, "War Council", "dm_only")
	mustEvent(hidden, "Secret Meeting", "everyone")

	// A visible event on day 2 linked to a private page: the event is listed
	// for a Player, but the page's id and name are not.
	cultID := newTestEntity(t, db, fixA.CampaignID, fixA.UserID, "The Hidden Cult")
	mustExec(t, db, `UPDATE entities SET is_private = 1 WHERE id = ?`, cultID)
	if err := eventRepo.CreateEvent(ctx, &Event{
		ID: testUUID(t), CalendarID: open.ID, Name: "Cult Gathering", EntityID: &cultID,
		Year: 1, Month: 1, Day: 2, Visibility: "everyone", AllDay: true,
	}); err != nil {
		t.Fatalf("create linked event: %v", err)
	}

	if _, err := calRepo.CreateEra(ctx, open.ID, EraInput{
		Name: "Age of Heroes", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#123456",
	}); err != nil {
		t.Fatalf("create era: %v", err)
	}

	t.Run("ListEventsForCalendar", func(t *testing.T) {
		tests := []struct {
			name       string
			campaignID string
			calendarID string
			role       int
			want       []string
		}{
			{"owner sees every event", fixA.CampaignID, open.ID, permissions.RoleOwner, []string{"Harvest Fair", "War Council", "Cult Gathering"}},
			{"player sees only everyone events", fixA.CampaignID, open.ID, permissions.RolePlayer, []string{"Harvest Fair", "Cult Gathering"}},
			{"player sees nothing on a dm_only calendar", fixA.CampaignID, hidden.ID, permissions.RolePlayer, nil},
			{"wrong campaign returns nothing, even for owner", fixB.CampaignID, open.ID, permissions.RoleOwner, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := svc.ListEventsForCalendar(ctx, tt.campaignID, tt.calendarID, tt.role)
				if err != nil {
					t.Fatalf("ListEventsForCalendar: %v", err)
				}
				var names []string
				for _, e := range got {
					names = append(names, e.Name)
				}
				if !equalStrings(names, tt.want) {
					t.Errorf("got %v, want %v", names, tt.want)
				}
			})
		}
	})

	t.Run("ListEventsForCalendar blanks a linked page the role can't see", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			role     int
			wantPage bool
		}{
			{"scribe", permissions.RoleScribe, false},
			{"player", permissions.RolePlayer, false},
			{"owner", permissions.RoleOwner, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := svc.ListEventsForCalendar(ctx, fixA.CampaignID, open.ID, tc.role)
				if err != nil {
					t.Fatalf("ListEventsForCalendar: %v", err)
				}
				for _, e := range got {
					if e.Name != "Cult Gathering" {
						continue
					}
					shown := e.EntityID != nil || e.EntityName != ""
					if shown != tc.wantPage {
						t.Errorf("linked page shown = %v (id=%v name=%q), want %v", shown, e.EntityID, e.EntityName, tc.wantPage)
					}
					return
				}
				t.Fatal("the linked event itself is missing")
			})
		}
	})

	t.Run("ListErasForCalendar", func(t *testing.T) {
		tests := []struct {
			name       string
			campaignID string
			calendarID string
			role       int
			wantCount  int
		}{
			{"owner sees the era", fixA.CampaignID, open.ID, permissions.RoleOwner, 1},
			{"player sees no eras", fixA.CampaignID, open.ID, permissions.RolePlayer, 0},
			{"anonymous sees no eras", fixA.CampaignID, open.ID, permissions.RoleNone, 0},
			{"wrong campaign returns nothing, even for owner", fixB.CampaignID, open.ID, permissions.RoleOwner, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := svc.ListErasForCalendar(ctx, tt.campaignID, tt.calendarID, tt.role)
				if err != nil {
					t.Fatalf("ListErasForCalendar: %v", err)
				}
				if len(got) != tt.wantCount {
					t.Errorf("got %d eras, want %d", len(got), tt.wantCount)
				}
			})
		}
	})
}
