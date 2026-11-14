package quests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// digestEnv holds sheets under a viewer-aware fake: ids in ents.hidden are
// pages a player may not open.
func digestEnv(t *testing.T, cal CalendarDirectory) questEnv {
	t.Helper()
	e := newQuestEnv()
	e.svc = NewQuestService(e.repo, e.ents, e.maps, cal)
	return e
}

// sheet stores a quest sheet for a page and when it was last saved.
func (e questEnv) sheet(id, name string, hidden bool, saved time.Time, body string) {
	e.ents.add(camp, id, name, hidden)
	info := e.ents.byID[id]
	info.TypeSlug = "quest"
	e.ents.byID[id] = info
	e.repo.docs[id], e.repo.camp[id] = []byte(body), camp
	if e.repo.saved == nil {
		e.repo.saved = map[string]time.Time{}
	}
	e.repo.saved[id] = saved
}

type onlyCalendar struct{ CalendarDirectory }

func (onlyCalendar) Calendar(context.Context, string) (QuestCalendar, error) {
	return fakeQuestCalendar{}, nil
}

func TestDigest(t *testing.T) {
	ctx := context.Background()
	day := func(d int) time.Time { return time.Date(2026, 11, d, 12, 0, 0, 0, time.UTC) }
	build := func() questEnv {
		e := digestEnv(t, onlyCalendar{})
		// Today in the fake calendar is Frost 10, year 100.
		e.sheet("newest", "Who is buying the grain", false, day(13), `{"status":"not_started","notice":{"title":"Who is buying the grain?"}}`)
		e.sheet("older", "The abbey bell", false, day(2), `{"status":"active","notice":{"title":"Find the bell"}}`)
		e.sheet("due-soon", "Escort the envoy", false, day(1), `{"status":"active","notice":{"title":"Escort the envoy"},"dueDate":{"year":100,"month":1,"day":16}}`)
		e.sheet("due-late", "Pay the ferryman", false, day(1), `{"status":"active","notice":{"title":"Pay the ferryman"},"dueDate":{"year":100,"month":1,"day":8}}`)
		e.sheet("done", "Old business", false, day(12), `{"status":"done","notice":{"title":"Old business"}}`)
		e.sheet("failed", "Lost cause", false, day(12), `{"status":"failed","notice":{"title":"Lost cause"}}`)
		e.sheet("secret-page", "The Pretender Plot", true, day(14), `{"status":"active","notice":{"title":"SECRET PLOT"}}`)
		e.sheet("hidden-notice", "Quiet job", false, day(14), `{"status":"active","notice":{"title":"HIDDEN NOTICE"},"layout":{"notice":{"x":1,"y":1,"w":30,"hidden":true},"map":{"x":1,"y":1,"w":30},"tag":{"x":1,"y":1,"w":30}}}`)
		return e
	}
	titles := func(d *QuestDigest) string {
		var out []string
		for _, q := range d.Open {
			out = append(out, q.Title)
		}
		return strings.Join(out, " | ")
	}
	tests := []struct {
		name      string
		viewer    Viewer
		wantOpen  string
		wantTotal int
	}{
		{"a player: due first (late before soon), then newest; nothing hidden", player,
			"Pay the ferryman | Escort the envoy | Who is buying the grain? | Find the bell", 6},
		{"a guest reads as a player", guest,
			"Pay the ferryman | Escort the envoy | Who is buying the grain? | Find the bell", 6},
		{"the DM sees the hidden page and the hidden notice too", dm,
			"Pay the ferryman | Escort the envoy | HIDDEN NOTICE | SECRET PLOT | Who is buying the grain? | Find the bell", 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := build().svc.Digest(ctx, camp, tt.viewer)
			if err != nil {
				t.Fatal(err)
			}
			if got := titles(d); got != tt.wantOpen {
				t.Errorf("open = %q, want %q", got, tt.wantOpen)
			}
			if d.Total != tt.wantTotal {
				t.Errorf("total = %d, want %d", d.Total, tt.wantTotal)
			}
			if d.TypeSlug != "quest" {
				t.Errorf("type slug = %q", d.TypeSlug)
			}
		})
	}
	t.Run("days left come from the calendar", func(t *testing.T) {
		d, _ := build().svc.Digest(ctx, camp, player)
		if d.Open[0].DaysLeft == nil || *d.Open[0].DaysLeft != -2 || d.Open[1].DaysLeft == nil || *d.Open[1].DaysLeft != 6 || d.Open[2].DaysLeft != nil {
			t.Errorf("days left = %+v", d.Open)
		}
	})
	t.Run("another campaign's sheets are never read", func(t *testing.T) {
		e := build()
		e.ents.add(other, "foreign", "Foreign quest", false)
		e.repo.docs["foreign"], e.repo.camp["foreign"] = []byte(`{"status":"active","notice":{"title":"FOREIGN"}}`), other
		d, _ := e.svc.Digest(ctx, camp, dm)
		if strings.Contains(titles(d), "FOREIGN") || d.Total != 8 {
			t.Errorf("digest reads across campaigns: %q total %d", titles(d), d.Total)
		}
	})
	t.Run("no sheets", func(t *testing.T) {
		d, err := newQuestEnv().svc.Digest(ctx, camp, player)
		if err != nil || len(d.Open) != 0 || d.Total != 0 || d.TypeSlug != "" {
			t.Errorf("got %+v, %v", d, err)
		}
	})
}

func TestBuildOpenQuests(t *testing.T) {
	n := func(v int) *int { return &v }
	d := &QuestDigest{TypeSlug: "quest", Total: 7, Open: []DigestQuest{
		{EntityID: "a", Title: "Find the bell", Status: StatusActive},
		{EntityID: "b", Title: "Escort the envoy", Status: StatusActive, DaysLeft: n(6)},
		{EntityID: "c", Title: "Who is buying the grain?", Status: StatusNotStarted},
		{EntityID: "d", Title: "A fourth quest", Status: StatusNotStarted},
	}}
	v := buildOpenQuests("c1", d)
	if len(v.Rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(v.Rows))
	}
	for i, want := range []string{"In progress", "Due in 6 days", "New"} {
		if v.Rows[i].Word != want {
			t.Errorf("row %d word = %q, want %q", i, v.Rows[i].Word, want)
		}
	}
	if v.AllWord != "All 7 quests" || v.AllHref != "/campaigns/c1/quest" {
		t.Errorf("link = %q %q", v.AllWord, v.AllHref)
	}
	tests := []struct {
		days int
		want string
	}{{-3, "Overdue"}, {0, "Due today"}, {1, "Due tomorrow"}, {2, "Due in 2 days"}}
	for _, tt := range tests {
		if got := dueWord(tt.days); got != tt.want {
			t.Errorf("dueWord(%d) = %q, want %q", tt.days, got, tt.want)
		}
	}
	if allWord(1) != "All 1 quest" {
		t.Errorf("allWord(1) = %q", allWord(1))
	}
}

func openQuestsHTML(t *testing.T, v openQuestsView) string {
	t.Helper()
	var b bytes.Buffer
	if err := openQuestsBlock(v).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The rendered block holds the titles and words the service let through and
// nothing else, for a player and for a guest.
func TestOpenQuestsBlockLeaksNothing(t *testing.T) {
	ctx := context.Background()
	e := digestEnv(t, onlyCalendar{})
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	e.sheet("seen", "Find the bell", false, at, `{"status":"active","notice":{"title":"Find the bell"},"steps":[{"id":"a","text":"SECRET STEP","shown":false}],"rewards":[{"kind":"money","text":"SECRET REWARD"}]}`)
	e.sheet("hid", "The Pretender Plot", true, at, `{"status":"active","notice":{"title":"SECRET PLOT"}}`)
	e.sheet("quiet", "Quiet job", false, at, `{"status":"active","notice":{"title":"HIDDEN NOTICE"},"layout":{"notice":{"x":1,"y":1,"w":30,"hidden":true},"map":{"x":1,"y":1,"w":30},"tag":{"x":1,"y":1,"w":30}}}`)
	for name, viewer := range map[string]Viewer{"player": player, "guest": guest} {
		t.Run(name, func(t *testing.T) {
			d, err := e.svc.Digest(ctx, camp, viewer)
			if err != nil {
				t.Fatal(err)
			}
			html := openQuestsHTML(t, buildOpenQuests(camp, d))
			for _, not := range []string{"SECRET", "HIDDEN", "Pretender", "Quiet job", "All 2 quests", "All 3 quests"} {
				if strings.Contains(html, not) {
					t.Errorf("block holds %q:\n%s", not, html)
				}
			}
			for _, want := range []string{"Open quests", "Find the bell", "In progress", "All 1 quest", "/campaigns/camp-1/quest"} {
				if !strings.Contains(html, want) {
					t.Errorf("block missing %q:\n%s", want, html)
				}
			}
		})
	}
	t.Run("empty says so", func(t *testing.T) {
		html := openQuestsHTML(t, buildOpenQuests(camp, &QuestDigest{}))
		if !strings.Contains(html, "No open quests") || strings.Contains(html, "All ") {
			t.Errorf("empty block:\n%s", html)
		}
	})
}

type digestStub struct {
	QuestService
	d     *QuestDigest
	calls int
}

func (s *digestStub) Digest(context.Context, string, Viewer) (*QuestDigest, error) {
	s.calls++
	return s.d, nil
}

func TestOpenQuestsBlockGuests(t *testing.T) {
	d := &QuestDigest{TypeSlug: "quest", Total: 1, Open: []DigestQuest{{EntityID: "a", Title: "Find the bell", Status: StatusActive}}}
	placed := func() *campaigns.Campaign {
		camp := &campaigns.Campaign{ID: "c1", IsPublic: true}
		layout := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{
			Blocks: []campaigns.DashboardBlock{{ID: "b", Type: campaigns.BlockOpenQuests}}}}}}}
		full, err := camp.SetRoleDashboardJSON(campaigns.HomeGuest, layout)
		if err != nil {
			t.Fatal(err)
		}
		camp.DashboardLayout = full
		return camp
	}
	tests := []struct {
		name      string
		cc        *campaigns.CampaignContext
		emptyDig  bool
		wantBody  bool
		wantReads int
	}{
		{"a guest where it is not placed gets nothing and nothing is read", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}}, false, false, 0},
		{"a guest where it is placed gets the quests they may open", &campaigns.CampaignContext{Campaign: placed()}, false, true, 1},
		{"a guest with no open quest gets no card", &campaigns.CampaignContext{Campaign: placed()}, true, false, 1},
		{"a member with none sees the empty line", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}, true, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dig := d
			if tt.emptyDig {
				dig = &QuestDigest{}
			}
			stub := &digestStub{d: dig}
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/open-quests", nil), rec)
			c.Set("campaign_context", tt.cc)
			if err := (&Handler{quests: stub}).OpenQuestsBlock(c); err != nil {
				t.Fatal(err)
			}
			if stub.calls != tt.wantReads {
				t.Errorf("digest reads = %d, want %d", stub.calls, tt.wantReads)
			}
			if got := rec.Body.Len() > 0; got != tt.wantBody {
				t.Errorf("body present = %v, want %v:\n%s", got, tt.wantBody, rec.Body.String())
			}
		})
	}
}
