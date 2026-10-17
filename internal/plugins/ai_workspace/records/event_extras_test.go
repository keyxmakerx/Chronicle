package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeTiers []campaigns.TierDefinition

func (f fakeTiers) GetEventTierDefinitions(context.Context, string) ([]campaigns.TierDefinition, error) {
	return f, nil
}

func defaultTiers() fakeTiers {
	return fakeTiers{
		{Slug: "major", Name: "Major"},
		{Slug: "standard", Name: "Standard", IsDefault: true},
		{Slug: "detail", Name: "Detail"},
	}
}

// multiCal adds a second calendar the operator can see to fakeCal.
type multiCal struct {
	*fakeCal
	other calendar.Calendar
}

func (m multiCal) ListCalendars(context.Context, string, permissions.Viewer) ([]calendar.Calendar, error) {
	return []calendar.Calendar{m.cal, m.other}, nil
}

func (m multiCal) GetCalendarForViewer(_ context.Context, id, _ string, _ permissions.Viewer) (*calendar.Calendar, error) {
	switch id {
	case m.cal.ID:
		return &m.cal, nil
	case m.other.ID:
		return &m.other, nil
	}
	return nil, apperror.NewNotFound("calendar not found")
}

func extrasCal() *fakeCal {
	c := newCal()
	c.cal.EventKinds = []calendar.EventKind{{ID: 4, Slug: "festival", Name: "Festival"}, {ID: 5, Slug: "battle", Name: "Battle"}}
	c.stored = []calendar.Event{{ID: "ev-feast", Name: "Feast", Year: 1492, Month: 1, Day: 21}}
	return c
}

func extrasKind(svc CalendarAPI) EventKind {
	return EventKind{Svc: svc, Tiers: defaultTiers(), Pages: pageWorld()}
}

// Pins that an event row's importance, category and page reach the event,
// matched by name, and are refused in plain words when nothing matches.
func TestEventKind_ExtrasOnCreate(t *testing.T) {
	tests := []struct {
		name    string
		extra   map[string]any
		wantErr string
		check   func(t *testing.T, in calendar.CreateEventInput)
	}{
		{name: "importance by name", extra: map[string]any{"importance": "Major"}, check: func(t *testing.T, in calendar.CreateEventInput) {
			if in.Tier == nil || *in.Tier != "major" {
				t.Fatalf("tier %v", in.Tier)
			}
		}},
		{name: "category by name", extra: map[string]any{"category": "battle"}, check: func(t *testing.T, in calendar.CreateEventInput) {
			if in.KindID == nil || *in.KindID != 5 {
				t.Fatalf("kind %v", in.KindID)
			}
		}},
		{name: "page by name", extra: map[string]any{"page": "Age of Ash"}, check: func(t *testing.T, in calendar.CreateEventInput) {
			if in.EntityID == nil || *in.EntityID != "p-ash" {
				t.Fatalf("entity %v", in.EntityID)
			}
		}},
		{name: "none of them", extra: nil, check: func(t *testing.T, in calendar.CreateEventInput) {
			if in.Tier != nil || in.KindID != nil || in.EntityID != nil {
				t.Fatalf("%+v", in)
			}
		}},
		{name: "unknown importance lists the levels", extra: map[string]any{"importance": "epic"}, wantErr: "Major, Standard, Detail"},
		{name: "unknown category lists them", extra: map[string]any{"category": "wedding"}, wantErr: "Festival, Battle"},
		{name: "hidden page reads as missing", extra: map[string]any{"page": "Secret"}, wantErr: "no page called"},
		{name: "none on create", extra: map[string]any{"category": "none"}, wantErr: "only applies to action: update"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := extrasCal()
			k := extrasKind(c)
			r := rec("event", ActionCreate, "Siege", eventFields(tt.extra), "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q; want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("plan %+v", p)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			tt.check(t, c.events[0])
		})
	}
}

// On update a key left out keeps the stored value and `none` clears it.
func TestEventKind_ExtrasOnUpdate(t *testing.T) {
	c := extrasCal()
	k := extrasKind(c)
	r := rec("event", ActionUpdate, "Feast", eventFields(map[string]any{"importance": "none", "page": "Selune Lore"}), "")
	if p := k.Plan(context.Background(), camp, owner, r); p.Error != "" || !strings.Contains(p.Summary, "linked to Selune Lore") {
		t.Fatalf("plan %+v", p)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	in := c.updates[0]
	if !in.Tier.IsNull() {
		t.Errorf("importance: none should clear the tier")
	}
	if v, ok := in.EntityID.Get(); !ok || v != "p-selune" {
		t.Errorf("page = %v", v)
	}
	if in.KindID.Present() {
		t.Errorf("category was left out, so it must stay as stored")
	}
}

func TestEventKind_ExtrasRefusedOnDelete(t *testing.T) {
	k := extrasKind(extrasCal())
	r := rec("event", ActionDelete, "Feast", eventFields(map[string]any{"importance": "major"}), "")
	if p := k.Plan(context.Background(), camp, owner, r); !strings.Contains(p.Error, "don't go with action: delete") {
		t.Fatalf("plan %+v", p)
	}
}

// `calendar:` writes to the named calendar; a name that matches none lists them.
func TestEventKind_NamedCalendar(t *testing.T) {
	other := calendar.Calendar{ID: "cal-2", Name: "Moon Court", Months: []calendar.Month{{Name: "Frost", Days: 20}}}
	tests := []struct {
		name    string
		cal     string
		month   string
		wantErr string
		wantSum string
	}{
		{name: "the other calendar", cal: "moon court", month: "Frost", wantSum: "on Moon Court"},
		{name: "unknown calendar", cal: "Nope", month: "Frost", wantErr: "the calendars are Harptos, Moon Court"},
		{name: "month checked against the named calendar", cal: "Moon Court", month: "Hammer", wantErr: "not a month of Moon Court"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := multiCal{fakeCal: extrasCal(), other: other}
			k := extrasKind(svc)
			r := rec("event", ActionCreate, "Revel", map[string]any{"calendar": tt.cal, "year": 3, "month": tt.month, "day": 2}, "")
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("plan error %q; want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.wantSum) {
				t.Fatalf("plan %+v", p)
			}
		})
	}
}

// Without the calendar list, naming a calendar is refused, never guessed.
func TestEventKind_NamedCalendarUnwired(t *testing.T) {
	k := extrasKind(extrasCal())
	r := rec("event", ActionCreate, "Revel", eventFields(map[string]any{"calendar": "Moon Court"}), "")
	if p := k.Plan(context.Background(), camp, owner, r); !strings.Contains(p.Error, "only the main calendar") {
		t.Fatalf("plan %+v", p)
	}
}
