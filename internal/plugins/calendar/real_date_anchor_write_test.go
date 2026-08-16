// real_date_anchor_write_test.go — the anchor's WRITE path: what the form
// sends, what the service refuses, and what reaches the row.
//
// real_date_anchor_test.go proves the arithmetic. This file proves the two
// things that stand between an owner and that arithmetic:
//
//	· parseAnchorRequest, because the form sends STRINGS and the difference
//	  between "the owner cleared this field" and "the owner typed 0" is a real
//	  year on a fantasy calendar;
//	· SetRealDateAnchor, because a phantom in-world date NEVER FAILS
//	  downstream — AbsoluteDay returns a number for month 14 day 40 quite
//	  happily — so the write is the only place it can be caught.
package calendar

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ── the form's four strings ─────────────────────────────────────────────────

func TestParseAnchorRequest(t *testing.T) {
	cases := []struct {
		name    string
		req     anchorRequest
		wantNil bool   // a clear
		wantErr string // substring the message must carry
		want    *RealDateAnchor
	}{
		{
			name: "all four",
			req:  anchorRequest{Year: "1492", Month: "4", Day: "14", RealDate: "2026-10-03"},
			want: &RealDateAnchor{Year: 1492, Month: 4, Day: 14, RealDate: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)},
		},
		{
			// THE CLEAR IS A REAL OPERATION, not an error. An owner removing a
			// wrong anchor has no other way to say so, and refusing an empty
			// form would trap them with a pin they know is wrong.
			name:    "all empty is the clear",
			req:     anchorRequest{},
			wantNil: true,
		},
		{
			name:    "whitespace is still empty",
			req:     anchorRequest{Year: "  ", Month: "\t", Day: " ", RealDate: "  "},
			wantNil: true,
		},
		{
			// The message must NAME the missing field. "Invalid" in a form with
			// four inputs tells an owner nothing about which one to look at.
			name:    "three of four names what is missing",
			req:     anchorRequest{Year: "1492", Month: "4", Day: "14"},
			wantErr: "real-world date",
		},
		{
			name:    "only a real date names the other three",
			req:     anchorRequest{RealDate: "2026-10-03"},
			wantErr: "in-world year",
		},
		{
			// YEAR ZERO IS NOT AN EMPTY FIELD. This is the whole reason the
			// request binds strings: with ints, Echo binds a cleared
			// <input type="number"> to 0, so this case and "all empty" would be
			// indistinguishable and one of them would be silently wrong.
			name: "year zero is a value, not an absence",
			req:  anchorRequest{Year: "0", Month: "1", Day: "1", RealDate: "2026-01-01"},
			want: &RealDateAnchor{Year: 0, Month: 1, Day: 1, RealDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:    "a non-numeric year says so",
			req:     anchorRequest{Year: "MCMXCII", Month: "4", Day: "14", RealDate: "2026-10-03"},
			wantErr: "whole number",
		},
		{
			name:    "a non-ISO real date says so",
			req:     anchorRequest{Year: "1492", Month: "4", Day: "14", RealDate: "3 Oct 2026"},
			wantErr: "YYYY-MM-DD",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAnchorRequest(tc.req)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error naming %q, got anchor %+v", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("the refusal reads %q and does not name %q — an owner staring at "+
						"four inputs cannot act on it", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Errorf("expected the clear, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected an anchor, got the clear — an owner's save would silently " +
					"REMOVE their anchor instead of setting it")
			}
			if got.Year != tc.want.Year || got.Month != tc.want.Month || got.Day != tc.want.Day ||
				!got.RealDate.Equal(tc.want.RealDate) {
				t.Errorf("parsed %+v, want %+v", *got, *tc.want)
			}
		})
	}
}

// ── the service ─────────────────────────────────────────────────────────────

// anchorSvcFixture wires a service over the package mock, with a calendar whose
// months the repo serves separately — which is the shape the real repo has, and
// the reason SetRealDateAnchor has to load them before validating.
func anchorSvcFixture(t *testing.T) (CalendarService, *mockCalendarRepo) {
	t.Helper()
	months := []Month{
		{Name: "Hammer", Days: 31, SortOrder: 0},
		{Name: "Alturiak", Days: 28, SortOrder: 1},
		{Name: "Ches", Days: 30, SortOrder: 2},
	}
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			// Deliberately WITHOUT Months, mirroring the real row read: the
			// structure is eager-loaded separately. A service that validated
			// against this alone would refuse every anchor.
			return &Calendar{ID: id, CampaignID: "camp-1", Name: "Harptos", Mode: "fantasy"}, nil
		},
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return months, nil },
	}
	return NewCalendarService(repo), repo
}

func TestSetRealDateAnchor_ValidatesAgainstTheCalendarsOwnStructure(t *testing.T) {
	svc, repo := anchorSvcFixture(t)
	ctx := context.Background()

	// A day the calendar HAS.
	err := svc.SetRealDateAnchor(ctx, "cal-1", &RealDateAnchor{
		Year: 1492, Month: 2, Day: 28, RealDate: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("a real day was refused: %v", err)
	}
	if repo.anchorWrites != 1 || repo.lastAnchor == nil {
		t.Fatalf("the accepted anchor did not reach the repository (writes=%d)", repo.anchorWrites)
	}

	// A day it does NOT have. This is the case that matters: the arithmetic
	// downstream would accept it silently and mis-date every other day.
	before := repo.anchorWrites
	err = svc.SetRealDateAnchor(ctx, "cal-1", &RealDateAnchor{
		Year: 1492, Month: 2, Day: 30, RealDate: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)})
	if err == nil {
		t.Error("Alturiak 30 was accepted on a 28-day Alturiak. AbsoluteDay would return a " +
			"number for it quite happily, so every other day would map two days out — " +
			"silently, and permanently")
	}
	if repo.anchorWrites != before {
		t.Error("a refused anchor still reached the repository")
	}

	// A month it does not have.
	before = repo.anchorWrites
	if err := svc.SetRealDateAnchor(ctx, "cal-1", &RealDateAnchor{
		Year: 1492, Month: 9, Day: 1, RealDate: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}); err == nil {
		t.Error("month 9 was accepted on a 3-month calendar")
	}
	if repo.anchorWrites != before {
		t.Error("a refused anchor still reached the repository")
	}
}

// TestSetRealDateAnchor_TheClearNeedsNoValidation. Removing a pin cannot be
// invalid, and it must not be blocked by a calendar whose structure has since
// become un-anchorable — that is exactly when an owner most needs to clear it.
func TestSetRealDateAnchor_TheClearNeedsNoValidation(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(context.Context, string) (*Calendar, error) {
			t.Error("the clear loaded the calendar — there is nothing to validate against")
			return nil, nil
		},
	}
	if err := NewCalendarService(repo).SetRealDateAnchor(context.Background(), "cal-1", nil); err != nil {
		t.Fatalf("the clear was refused: %v", err)
	}
	if repo.anchorWrites != 1 {
		t.Fatalf("the clear did not reach the repository (writes=%d)", repo.anchorWrites)
	}
	if repo.lastAnchor != nil {
		t.Errorf("the clear wrote %+v instead of nil", *repo.lastAnchor)
	}
}

// TestSetRealDateAnchor_DoesNotPublishStructureUpdated.
//
// `calendar.structure.updated` means THE SHAPE OF THE CALENDAR CHANGED, and the
// Foundry module reacts by re-running its structure comparison and badging a
// mismatch (see the module's CLAUDE.md). The anchor changes no month, no
// weekday and no leap rule — the in-world calendar is byte-identical before and
// after — so publishing it would badge a mismatch that does not exist and pause
// a working calendar sync.
func TestSetRealDateAnchor_DoesNotPublishStructureUpdated(t *testing.T) {
	svc, _ := anchorSvcFixture(t)
	pub := &anchorRecordingPublisher{}
	s, ok := svc.(*calendarService)
	if !ok {
		t.Fatal("service is not the concrete type; the publisher cannot be observed")
	}
	s.SetEventPublisher(pub)

	if err := svc.SetRealDateAnchor(context.Background(), "cal-1", &RealDateAnchor{
		Year: 1492, Month: 1, Day: 1, RealDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("setting the anchor failed: %v", err)
	}
	for _, ev := range pub.types {
		if strings.Contains(ev, "structure") {
			t.Errorf("setting the anchor published %q. The calendar's SHAPE did not change; "+
				"a structure signal here badges a mismatch that does not exist and pauses a "+
				"working Foundry sync", ev)
		}
	}
	// A publisher that recorded NOTHING would pass the loop above vacuously, so
	// prove the recorder works by making the service publish something it does.
	if err := svc.SetEras(context.Background(), "cal-1", []EraInput{{Name: "First Age", StartYear: 1}}); err != nil {
		t.Fatalf("SetEras failed: %v", err)
	}
	if len(pub.types) == 0 {
		t.Fatal("the recorder captured nothing even from SetEras, so the assertion above " +
			"proved nothing about the anchor")
	}
	t.Logf("publisher saw %v across an anchor write and an eras write", pub.types)
}

// anchorRecordingPublisher records the event TYPES the service emits.
type anchorRecordingPublisher struct{ types []string }

func (p *anchorRecordingPublisher) PublishCalendarEvent(eventType, _, _ string, _ any) {
	p.types = append(p.types, eventType)
}
