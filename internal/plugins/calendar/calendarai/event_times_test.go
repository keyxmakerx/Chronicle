package calendarai

import (
	"context"
	"strings"
	"testing"
)

func evFields(extra map[string]any) map[string]any {
	f := map[string]any{"year": 1492, "month": 1, "day": 3}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func TestEventKind_EndTimeAndAnnounced(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		actor   Actor
		fields  map[string]any
		wantErr string
		check   func(t *testing.T, c *fakeCal)
	}{
		{"create with an end time", ActionCreate, owner, evFields(map[string]any{"hour": 19, "minute": 30, "end_hour": 22, "end_minute": 15}), "", func(t *testing.T, c *fakeCal) {
			in := c.events[0]
			if in.EndHour == nil || *in.EndHour != 22 || *in.EndMinute != 15 || in.EndYear == nil || *in.EndDay != 3 {
				t.Errorf("event %+v", in)
			}
		}},
		{"end before start the same day", ActionCreate, owner, evFields(map[string]any{"hour": 19, "end_hour": 18}), "ends before it starts", nil},
		{"end next day may be earlier", ActionCreate, owner, evFields(map[string]any{"hour": 19, "end_hour": 2, "end_year": 1492, "end_month": 1, "end_day": 4}), "", nil},
		{"end hour outside the day", ActionCreate, owner, evFields(map[string]any{"end_hour": 30}), "outside this calendar's day", nil},
		{"end minute outside the hour", ActionCreate, owner, evFields(map[string]any{"end_hour": 3, "end_minute": 75}), "outside this calendar's hour", nil},
		{"end minute alone", ActionCreate, owner, evFields(map[string]any{"end_minute": 5}), "end_minute needs end_hour", nil},
		{"end hour none on create", ActionCreate, owner, evFields(map[string]any{"end_hour": "none"}), "only applies to action: update", nil},
		{"update clears the end time", ActionUpdate, owner, map[string]any{"year": 1492, "month": 1, "day": 21, "end_hour": "none"}, "", func(t *testing.T, c *fakeCal) {
			if !c.updates[0].EndHour.IsNull() || !c.updates[0].EndMinute.IsNull() {
				t.Errorf("update %+v", c.updates[0])
			}
		}},
		{"update sets an end time", ActionUpdate, owner, map[string]any{"year": 1492, "month": 1, "day": 21, "end_hour": 23}, "", func(t *testing.T, c *fakeCal) {
			u := c.updates[0]
			if v, _ := u.EndHour.Get(); v != 23 {
				t.Errorf("update %+v", u)
			}
			if v, _ := u.EndDay.Get(); v != 21 {
				t.Errorf("a timed end needs the event's own day as its end date: %+v", u)
			}
		}},
		{"announced on the day", ActionCreate, owner, evFields(map[string]any{"announced": "on_day"}), "", func(t *testing.T, c *fakeCal) {
			if a := c.events[0].Announced; a == nil || *a != "on_day" {
				t.Errorf("event %+v", c.events[0])
			}
		}},
		{"announced ahead on update", ActionUpdate, owner, map[string]any{"year": 1492, "month": 1, "day": 21, "announced": "ahead"}, "", func(t *testing.T, c *fakeCal) {
			if v, _ := c.updates[0].Announced.Get(); v != "ahead" {
				t.Errorf("update %+v", c.updates[0])
			}
		}},
		{"announced none clears", ActionUpdate, owner, map[string]any{"year": 1492, "month": 1, "day": 21, "announced": "none"}, "", func(t *testing.T, c *fakeCal) {
			if !c.updates[0].Announced.IsNull() {
				t.Errorf("update %+v", c.updates[0])
			}
		}},
		{"announced none on create", ActionCreate, owner, evFields(map[string]any{"announced": "none"}), "only applies to action: update", nil},
		{"announced unknown word", ActionCreate, owner, evFields(map[string]any{"announced": "later"}), "announced must be", nil},
		{"announced by a player", ActionCreate, player, evFields(map[string]any{"announced": "ahead"}), "only the owner or a co-DM", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := extrasCal()
			c.cal.HoursPerDay, c.cal.MinutesPerHour = 24, 60
			k := extrasKind(c)
			r := rec("event", tt.action, "Feast", tt.fields, "")
			if tt.action == ActionCreate {
				r.Name = "New Feast"
			}
			p := k.Plan(context.Background(), camp, tt.actor, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatal(p.Error)
			}
			if tt.check == nil {
				return
			}
			if err := k.Apply(context.Background(), camp, tt.actor, r); err != nil {
				t.Fatal(err)
			}
			tt.check(t, c)
		})
	}
}

func TestLookups_EventsShowTimesAndAnnounced(t *testing.T) {
	l := eventsWorld()
	c := l.Cal.(*fakeCal)
	h, mi, eh, em, ann := 19, 30, 22, 0, "on_day"
	c.stored[0].StartHour, c.stored[0].StartMinute, c.stored[0].EndHour, c.stored[0].EndMinute, c.stored[0].Announced = &h, &mi, &eh, &em, &ann
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("events", map[string]any{"year": 1492, "month": 1})})[0]
	if !strings.Contains(ans.Text, "from 19:30 until 22:00") || !strings.Contains(ans.Text, "announced on_day") {
		t.Errorf("owner:\n%s", ans.Text)
	}
	ans = l.Answer(context.Background(), camp, player, []Record{lookup("events", map[string]any{"year": 1492, "month": 1})})[0]
	if strings.Contains(ans.Text, "announced") {
		t.Errorf("a player was shown the announce setting:\n%s", ans.Text)
	}
}
