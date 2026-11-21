package sessions

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestAllNightsNoRealDateWords(t *testing.T) {
	h := NewHandler(NewSessionService(&mockSessionRepo{}, nil, nil))
	h.SetWorldDateWorder(fakeWorder{words: "14 Hammer, 1492"})
	y, m, d := 1492, 3, 14
	story := Session{ID: "s1", CampaignID: "camp-1", Name: "Session 1", Status: StatusPlanned, CalendarYear: &y, CalendarMonth: &m, CalendarDay: &d}
	dated := Session{ID: "s2", Name: "Friday", Status: StatusPlanned, ScheduledDate: strp("2026-11-06")}
	bare := Session{ID: "s3", Name: "Someday", Status: StatusPlanned}
	groups := allNightsGroups("camp-1", groupSessions([]Session{story, dated, bare}, "2026-10-31"), "2026-10-31", "",
		func(s *Session) string { return h.noRealDateWhen(context.Background(), s) })
	got := map[string]allNightsItem{}
	for _, g := range groups {
		for _, n := range g.Nights {
			got[n.SessionID] = n
		}
	}
	if w := got["s1"]; !w.NoRealDate || w.When != "No real date yet · 14 Hammer, 1492" || w.Day != "" {
		t.Fatalf("story night = %+v", w)
	}
	if w := got["s3"]; !w.NoRealDate || w.When != "No real date yet" {
		t.Fatalf("night with no date at all = %+v", w)
	}
	if w := got["s2"]; w.NoRealDate || !strings.Contains(w.When, "Nov 6, 2026") {
		t.Fatalf("dated night = %+v", w)
	}
}

// The night's own page says there is no real date, names the event it came
// from, and offers the owner a way to set the date; a player gets the words
// and the event but no date box.
func TestNightPageSaysNoRealDate(t *testing.T) {
	y, m, d := 1492, 3, 14
	ev := "e1"
	s := &Session{ID: "s1", CampaignID: "camp-1", Name: "Session 1", Status: StatusPlanned,
		CalendarYear: &y, CalendarMonth: &m, CalendarDay: &d, CalendarEventID: &ev}
	h := NewHandler(NewSessionService(&mockSessionRepo{}, nil, nil))
	h.SetWorldDateWorder(fakeWorder{words: "14 Hammer, 1492"})
	h.SetSessionEventSource(fakeEventSource{events: []SessionEvent{{ID: "e1", Name: "Session 1", CalendarID: "cal-1", Year: 1492, Month: 3, Day: 14}}})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Table"}, MemberRole: campaigns.RoleOwner}
	note := h.nightDateNote(context.Background(), cc, s)
	if !note.NoRealDate || note.Words != "No real date yet · 14 Hammer, 1492" || note.EventName != "Session 1" ||
		note.EventHref != "/campaigns/camp-1/calendars/cal-1/view?event=e1&date=1492-3-14" {
		t.Fatalf("note = %+v", note)
	}
	for _, tt := range []struct {
		name      string
		scribe    bool
		wantInput bool
	}{{"owner", true, true}, {"player", false, false}} {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			if err := SessionDetailPage(cc, s, "csrf", tt.scribe, tt.scribe, "u1", RecapView{}, "", note).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			for _, want := range []string{"No real date yet · 14 Hammer, 1492", "On the calendar: Session 1", "event=e1"} {
				if !strings.Contains(out, want) {
					t.Errorf("page lacks %q", want)
				}
			}
			if has := strings.Contains(out, `id="night-real-date"`); has != tt.wantInput {
				t.Errorf("date box present = %v, want %v", has, tt.wantInput)
			}
			// The card is a widget mount, so it works after a boosted swap
			// without an inline handler or a script tag.
			if has := strings.Contains(out, `data-widget="night-date"`); has != tt.wantInput {
				t.Errorf("night-date mount present = %v, want %v", has, tt.wantInput)
			}
			if tt.wantInput && !strings.Contains(out, `data-url="/campaigns/camp-1/sessions/s1"`) {
				t.Error("the mount does not carry the night's URL")
			}
			if strings.Contains(out, "night_setRealDate") || strings.Contains(out, "scheduled_date:i.value") {
				t.Error("the date card still has an inline handler")
			}
		})
	}
}

// A list of date-less nights reads the calendars once, however many rows, and
// not at all when no row has an in-world day.
func TestNoRealDateWordsReadCalendarsOncePerList(t *testing.T) {
	y, m, d := 1492, 3, 14
	story := func(id string) *Session {
		return &Session{ID: id, CampaignID: "camp-1", CalendarYear: &y, CalendarMonth: &m, CalendarDay: &d}
	}
	tests := []struct {
		name      string
		nights    []*Session
		wantReads int
		want      string
	}{
		{"many story nights, one read", []*Session{story("a"), story("b"), story("c")}, 1, "No real date yet · 14 Hammer, 1492"},
		{"nights with no in-world day, no read", []*Session{{ID: "x"}, {ID: "y"}}, 0, NoRealDateWords},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reads := 0
			h := NewHandler(NewSessionService(&mockSessionRepo{}, nil, nil))
			h.SetWorldDateWorder(fakeWorder{words: "14 Hammer, 1492", reads: &reads})
			words := h.noRealDateWhenFn(context.Background(), "camp-1")
			for _, n := range tt.nights {
				if got := words(n); got != tt.want {
					t.Errorf("night %s = %q, want %q", n.ID, got, tt.want)
				}
			}
			if reads != tt.wantReads {
				t.Errorf("calendar reads = %d, want %d", reads, tt.wantReads)
			}
		})
	}
}
