// event_partial_update_test.go — sweep R4, the calendar-event half of the
// absent-means-preserve contract.
//
// C-CAL-NULL-PRESERVE got eighteen pointer fields right in 2026-05 and said
// out loud why it could not get the rest: a plain pointer collapses "the key
// was absent" and "the key was null", so the value-typed fields were left
// unguarded on the reasoning that they have no absent state. True of the Go
// type; false of the wire. Three Foundry calendar-sync paths push five-key
// bodies ({name, year, month, day, description}), and each of them was
// silently writing is_recurring=false, all_day=false and a cleared entity
// link onto whatever event it touched.
//
// patch.Field supplies the missing distinction, so this file pins the whole
// contract on the one endpoint the module actually calls.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// foundryFiveKeyBody is exactly what calendar-sync.mjs sends on a note edit:
// the Calendaria/SimpleCalendar note's name, date and description, and
// nothing else.
func foundryFiveKeyBody() UpdateEventInput {
	return UpdateEventInput{
		Name:        patch.Of("Harvest Feast (edited in Foundry)"),
		Year:        patch.Of(1492),
		Month:       patch.Of(7),
		Day:         patch.Of(15),
		Description: patch.Of("the note body from Foundry"),
	}
}

func runEventUpdate(t *testing.T, in UpdateEventInput) *Event {
	t.Helper()
	var written *Event
	repo := &mockCalendarRepo{
		getEventFn:    func(_ context.Context, _ string) (*Event, error) { return seededEvent(), nil },
		updateEventFn: func(_ context.Context, evt *Event) error { written = evt; return nil },
	}
	if err := newTestCalendarService(repo).UpdateEvent(context.Background(), "evt-1", in); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if written == nil {
		t.Fatal("repo.UpdateEvent not called")
	}
	return written
}

// THE headline regression, on the exact body the module sends.
func TestFoundryFiveKeyPush_KeepsRecurrenceAllDayAndTheEntityLink(t *testing.T) {
	got := runEventUpdate(t, foundryFiveKeyBody())

	if !got.IsRecurring {
		t.Error("is_recurring was switched off by a body that never mentioned it — the booked hazard")
	}
	if !got.AllDay {
		t.Error("all_day was switched off by a body that never mentioned it")
	}
	if got.EntityID == nil || *got.EntityID != "ent-1" {
		t.Errorf("EntityID = %v, want ent-1 preserved — the leg C-ENTITY-LINK-DESIGN parked", got.EntityID)
	}
	if got.RecurrenceType == nil || *got.RecurrenceType != "yearly" {
		t.Errorf("RecurrenceType = %v, want yearly preserved", got.RecurrenceType)
	}
	if got.Visibility != "everyone" {
		t.Errorf("Visibility = %q, want preserved", got.Visibility)
	}
	// …and the five keys it DOES send take effect.
	if got.Name != "Harvest Feast (edited in Foundry)" {
		t.Errorf("Name = %q, want the pushed value", got.Name)
	}
	if got.Description == nil || *got.Description != "the note body from Foundry" {
		t.Errorf("Description = %v, want the pushed value", got.Description)
	}
}

// The three directions, on the two fields that were value-typed.
func TestEvent_IsRecurringAndAllDay_ThreeDirections(t *testing.T) {
	cases := []struct {
		name           string
		in             UpdateEventInput
		wantRecurring  bool
		wantAllDay     bool
		wantClockClear bool
	}{
		{"absent preserves both", UpdateEventInput{Name: patch.Of("x")}, true, true, false},
		{"explicit false replaces both", UpdateEventInput{IsRecurring: patch.Of(false), AllDay: patch.Of(false)}, false, false, false},
		{"explicit all_day true still blanks the clock", UpdateEventInput{AllDay: patch.Of(true)}, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runEventUpdate(t, tc.in)
			if got.IsRecurring != tc.wantRecurring {
				t.Errorf("IsRecurring = %v, want %v", got.IsRecurring, tc.wantRecurring)
			}
			if got.AllDay != tc.wantAllDay {
				t.Errorf("AllDay = %v, want %v", got.AllDay, tc.wantAllDay)
			}
			if tc.wantClockClear && got.StartHour != nil {
				t.Errorf("StartHour = %d, want cleared by an explicit all_day=true", *got.StartHour)
			}
			if !tc.wantClockClear && (got.StartHour == nil || *got.StartHour != 14) {
				t.Errorf("StartHour = %v, want 14 preserved", got.StartHour)
			}
		})
	}
}

// seededEvent() is AllDay:true with a clock set — a shape that only exists
// because the pre-sweep code could produce it. An absent all_day must not
// re-run the "explicit all-day blanks the clock" rule, or every partial save
// would silently destroy times a later edit put back.
func TestEvent_AbsentAllDayDoesNotReRunTheClockBlanking(t *testing.T) {
	got := runEventUpdate(t, UpdateEventInput{Name: patch.Of("title only")})
	if got.StartHour == nil || *got.StartHour != 14 {
		t.Errorf("StartHour = %v, want 14 — an absent all_day is not an instruction to blank the clock", got.StartHour)
	}
}
