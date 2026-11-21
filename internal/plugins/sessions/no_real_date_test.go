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
			if tt.wantInput && !strings.Contains(out, "scheduled_date:i.value") {
				t.Error("the Set the date button does not send scheduled_date")
			}
			if tt.wantInput && !strings.Contains(out, `onclick="`) {
				t.Error("the date handler must be an inline onclick, not a script helper")
			}
		})
	}
}
