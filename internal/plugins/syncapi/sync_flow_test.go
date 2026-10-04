package syncapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

var flowT0 = time.Date(2026, 10, 3, 19, 41, 40, 17e6, time.UTC)

func intPtr(n int) *int { return &n }

// The refused date push from the approved mockup, with what came before
// and after it.
func flowFixture() []SyncEvent {
	return []SyncEvent{
		{ID: 1, OccurredAt: flowT0.Add(-28 * time.Second), Direction: DirToFoundry, ReportedBy: reportedByClient, Kind: "page", ResourceID: "e1",
			ResourceName: "The Drowned Bell", Action: "page updated", Call: "ws entity.updated", Status: "ok", OK: true, DurationMs: 42, UserName: "Key Maker"},
		{ID: 2, OccurredAt: flowT0, Direction: DirToChronicle, ReportedBy: reportedByChronicle, Kind: historyKindCalendar,
			ResourceName: "Frostfall 16, 1214", Was: "Frostfall 15, 1214", Action: "date set", Call: "PUT /calendar/date",
			Status: "403", Message: "owner role required", DurationMs: 18, UserName: "Ren", APIKeyID: intPtr(7)},
		{ID: 3, OccurredAt: flowT0.Add(25 * time.Second), Direction: DirToFoundry, ReportedBy: reportedByClient, Kind: historyKindCalendar,
			ResourceID: "cal1", Action: "date moved", Call: "ws calendar.date.advanced", Status: "ok", OK: true, DurationMs: 12, UserName: "Key Maker"},
		{ID: 4, OccurredAt: flowT0.Add(27 * time.Second), Direction: DirToChronicle, ReportedBy: reportedByChronicle, Kind: "page", ResourceID: "e2",
			ResourceName: "Port Ashwick", Action: "page updated", Call: "PUT /entities/:entityID", Status: "200", OK: true, DurationMs: 38, UserName: "Ren"},
	}
}

func flowFixtureInput(admin bool) flowInput {
	return flowInput{
		Events:   flowFixture(),
		Admin:    admin,
		Selected: 2,
		Keys:     map[int]FlowKey{7: {Name: "Ren’s table", Owner: "Ren", Role: "player"}},
		Repeats: map[int64][]SyncEvent{2: {
			{ID: 2, OccurredAt: flowT0, APIKeyID: intPtr(7)},
			{ID: 90, OccurredAt: flowT0.Add(-2 * time.Hour), APIKeyID: intPtr(7)},
			{ID: 80, OccurredAt: flowT0.Add(-4 * time.Hour), APIKeyID: intPtr(7)},
		}},
	}
}

func stepByID(f *CallFlow, id string) *FlowStep {
	for _, s := range f.Steps {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func TestBuildCallFlow_Steps(t *testing.T) {
	f := buildCallFlow(flowFixtureInput(false))
	type want struct {
		id         string
		from, to   int
		label, code string
		fail, reply bool
	}
	cases := []want{
		{"s1-0", laneBrowser, laneServer, "Saves the page", "edit on the web", false, false},
		{"s1-1", laneServer, laneFoundry, "Tells Foundry it changed", "entity.updated", false, false},
		{"s1-2", laneFoundry, laneFoundry, "Journal updated", "applied in Foundry", false, false},
		{"s2-0", laneFoundry, laneServer, "Moves the date", "PUT /calendar/date", false, false},
		{"s2-1", laneServer, laneFoundry, "Refused: owner only", "403 owner role required", true, true},
		{"s3-0", laneBrowser, laneServer, "Sets the date", "edit on the web", false, false},
		{"s3-1", laneServer, laneFoundry, "Tells Foundry", "calendar.date.advanced", false, false},
		{"s3-2", laneFoundry, laneFoundry, "Calendar moved", "applied in Foundry", false, false},
		{"s4-0", laneFoundry, laneServer, "Saves the page", "PUT /entities/:entityID", false, false},
		{"s4-1", laneServer, laneFoundry, "Saved", "200", false, true},
	}
	if len(f.Steps) != len(cases) {
		t.Fatalf("%d steps, want %d", len(f.Steps), len(cases))
	}
	for i, c := range cases {
		s := f.Steps[i]
		if s.ID != c.id || s.From != c.from || s.To != c.to || s.Label != c.label || s.Code != c.code || s.Fail != c.fail || s.Reply != c.reply {
			t.Errorf("step %d = %+v, want %+v", i, *s, c)
		}
	}
	if f.Selected != "s2-1" || f.Problems != 1 {
		t.Fatalf("selected %q, problems %d", f.Selected, f.Problems)
	}
	if got := stepByID(f, "s2-1").Line; got != lineBad {
		t.Fatalf("failed answer line %q", got)
	}
	if stepByID(f, "s2-0").Key == nil || stepByID(f, "s2-0").Key.Name != "Ren’s table" {
		t.Fatal("the call does not name its key")
	}
}

func TestBuildCallFlow_GroupsAndChain(t *testing.T) {
	f := buildCallFlow(flowFixtureInput(false))
	names := []string{}
	for _, g := range f.Groups {
		names = append(names, g.Name)
	}
	if strings.Join(names, "|") != "The Drowned Bell|Calendar|Port Ashwick" {
		t.Fatalf("groups %v", names)
	}
	// The refused date led to the date set in Chronicle 25 s later: same
	// thing, inside the chain gap.
	if got := strings.Join(stepByID(f, "s2-1").Then, " "); got != "s3-0" {
		t.Fatalf("refusal led to %q", got)
	}
	if got := strings.Join(stepByID(f, "s3-0").Cause, " "); got != "s2-1" {
		t.Fatalf("date set caused by %q", got)
	}
	// Different things never chain.
	if len(stepByID(f, "s1-2").Then) != 0 || len(stepByID(f, "s4-0").Cause) != 0 {
		t.Fatal("steps about different things were chained")
	}
	// Group headings come in time order, with the pause before each.
	heads := 0
	for _, r := range f.Rows {
		if r.Head != nil {
			heads++
			if heads == 2 && r.Head.Gap != 28*time.Second {
				t.Fatalf("calendar heading gap %v", r.Head.Gap)
			}
		}
	}
	if heads != 3 {
		t.Fatalf("%d headings, want 3 (page, calendar, the other page)", heads)
	}

	// Past the chain gap, the same thing starts a new story.
	evs := flowFixture()
	evs[2].OccurredAt = flowT0.Add(3 * time.Minute)
	evs[3].OccurredAt = flowT0.Add(4 * time.Minute)
	in := flowFixtureInput(false)
	in.Events = evs
	g := buildCallFlow(in)
	if len(stepByID(g, "s2-1").Then) != 0 {
		t.Fatal("steps minutes apart were chained")
	}
}

func TestBuildCallFlow_Problem(t *testing.T) {
	cases := []struct {
		name  string
		admin bool
		want  func(*testing.T, *FlowProblem)
	}{
		{"campaign", false, func(t *testing.T, p *FlowProblem) {
			if p.Headline != "Chronicle refused Ren’s date" {
				t.Errorf("headline %q", p.Headline)
			}
			if p.Asked != "Frostfall 16, 1214" || p.Was != "Frostfall 15, 1214" {
				t.Errorf("before/after %q -> %q", p.Was, p.Asked)
			}
			if !strings.Contains(p.Why, "belongs to a player") || !strings.Contains(p.Why, "DM access wouldn’t help") {
				t.Errorf("why %q", p.Why)
			}
			if len(p.Next) != 2 || len(p.Fix) != 2 {
				t.Errorf("next %v fix %v", p.Next, p.Fix)
			}
			if p.FixedBy != "s3-0" || p.FixedByWho != "Key Maker" {
				t.Errorf("fixed by %q %q", p.FixedBy, p.FixedByWho)
			}
			if p.RepeatCount != 3 || p.RepeatKey != "Ren’s table" || !p.Repeats[0].Before(p.Repeats[2]) {
				t.Errorf("repeats %d %q %v", p.RepeatCount, p.RepeatKey, p.Repeats)
			}
			if p.OpenHistory != "" {
				t.Error("campaign view links to itself")
			}
		}},
		{"admin", true, func(t *testing.T, p *FlowProblem) {
			if p.Headline != "Chronicle refused Ren’s date" || p.RepeatCount != 3 {
				t.Errorf("admin problem %+v", p)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := buildCallFlow(flowFixtureInput(tc.admin))
			p := stepByID(f, "s2-1").Problem
			if p == nil {
				t.Fatal("no problem story")
			}
			tc.want(t, p)
		})
	}
}

func TestExplainProblem_ByAnswer(t *testing.T) {
	cases := []struct {
		name     string
		ev       SyncEvent
		headline string
		short    string
	}{
		{"key not accepted", SyncEvent{Direction: DirToChronicle, ReportedBy: reportedByChronicle, Call: "PUT /entities/:entityID", Status: "401"}, "Chronicle didn’t accept the key", "Refused: key not accepted"},
		{"other refusal", SyncEvent{Direction: DirToChronicle, ReportedBy: reportedByChronicle, Call: "PUT /entities/:entityID", Status: "403", UserName: "Ren"}, "Chronicle refused Ren’s change", "Refused"},
		{"gone", SyncEvent{Direction: DirToChronicle, ReportedBy: reportedByChronicle, Call: "PUT /entities/:entityID", Status: "404"}, "Chronicle couldn’t find it", "Not found"},
		{"conflict", SyncEvent{Direction: DirToChronicle, ReportedBy: reportedByChronicle, Call: "PUT /entities/:entityID", Status: "409"}, "The change clashed with another", "Conflict"},
		{"date can't be held", SyncEvent{Direction: DirToChronicle, ReportedBy: reportedByChronicle, Call: "PUT /calendar/date", Status: "422"}, "Chronicle’s calendar couldn’t take that date", "Rejected"},
		{"server", SyncEvent{Direction: DirToChronicle, ReportedBy: reportedByChronicle, Call: "GET /entities", Status: "500"}, "Chronicle hit a problem", "Server problem"},
		{"foundry couldn't apply", SyncEvent{Direction: DirToFoundry, ReportedBy: reportedByClient, Status: "failed", Message: "folder missing"}, "Foundry couldn’t apply a change", "Failed"},
		{"foundry problem", SyncEvent{Direction: DirLink, ReportedBy: reportedByClient, Status: "failed"}, "Foundry reported a problem", "Failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := explainProblem(tc.ev, nil, flowInput{})
			if p.Headline != tc.headline {
				t.Errorf("headline %q, want %q", p.Headline, tc.headline)
			}
			if got := shortProblem(tc.ev); got != tc.short {
				t.Errorf("short %q, want %q", got, tc.short)
			}
			if p.What == "" || p.Why == "" || len(p.Fix) == 0 {
				t.Errorf("incomplete story %+v", p)
			}
		})
	}
	// Foundry's own words can carry page names: admin pages leave them out.
	ev := SyncEvent{Direction: DirToFoundry, ReportedBy: reportedByClient, Status: "failed", Message: "Harbour folder missing"}
	if p := explainProblem(ev, nil, flowInput{Admin: true}); strings.Contains(p.Why, "Harbour") {
		t.Fatalf("admin story repeats Foundry's words: %q", p.Why)
	}
}

func TestFlowPick(t *testing.T) {
	evs := flowFixture()
	cal := flowPick(evs, flowGroupKey(evs[1]), flowT0)
	if len(cal) != 2 || cal[0].ID != 2 || cal[1].ID != 3 {
		t.Fatalf("calendar rows %+v", cal)
	}
	var many []SyncEvent
	for i := 0; i < flowMaxEvents+10; i++ {
		many = append(many, SyncEvent{ID: int64(i + 1), OccurredAt: flowT0.Add(time.Duration(i) * time.Second), Kind: "page", ResourceID: "e1"})
	}
	got := flowPick(many, "", flowT0)
	if len(got) != flowMaxEvents || got[0].ID != 1 || got[len(got)-1].ID != int64(flowMaxEvents) {
		t.Fatalf("kept %d rows, %d..%d", len(got), got[0].ID, got[len(got)-1].ID)
	}
}

func TestSyncFlow_Render(t *testing.T) {
	cases := []struct {
		name     string
		admin    bool
		want     []string
		wantNone []string
	}{
		{"campaign", false, []string{
			"Chronicle refused Ren’s date", "The date, before and after", "Frostfall 15, 1214", "Frostfall 16, 1214",
			"The Drowned Bell", "Port Ashwick", `data-step="s2-1"`, "sf-sel", "sf-a21", "sf-dash", "403 owner role required",
			"“Ren’s table” · made by Ren · player", "3 times in the last 24 hours", "5 steps", "28 s later", `data-go="s3-0"`,
		}, []string{"Open in the campaign"}},
		{"admin hides names", true, []string{"Page 1", "Page 2", "Calendar", "Chronicle refused Ren’s date"}, []string{
			"The Drowned Bell", "Port Ashwick",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := buildCallFlow(flowFixtureInput(tc.admin))
			f.ID = "sf-2"
			var buf bytes.Buffer
			if err := SyncFlow(f).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.wantNone {
				if strings.Contains(html, w) {
					t.Errorf("should not contain %q", w)
				}
			}
		})
	}
	// Names are text, never markup.
	evs := flowFixture()
	evs[0].ResourceName = "<img src=x onerror=alert(1)>"
	in := flowFixtureInput(false)
	in.Events = evs
	var buf bytes.Buffer
	if err := SyncFlow(buildCallFlow(in)).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<img") {
		t.Fatal("a page name was rendered as markup")
	}
}

// stubKeysSvc lists the campaign's keys for the flow.
type stubKeysSvc struct {
	SyncAPIService
	keys []APIKey
}

func (s *stubKeysSvc) ListKeysByCampaign(context.Context, string) ([]APIKey, error) { return s.keys, nil }

func TestSyncFlowHandlers(t *testing.T) {
	rows := flowFixture()
	repo := &fakeHistoryRepo{rows: rows, latest: map[bool]*SyncEvent{true: &rows[1]}, failures: rows[1:2]}
	keys := &stubKeysSvc{keys: []APIKey{{ID: 7, Name: "Ren’s table", UserID: "u2"}}}
	h := NewSyncHistoryHandler(repo, &stubCampaignSvcForDmGrant{role: campaigns.RolePlayer}, keys, nil)
	h.now = func() time.Time { return flowT0.Add(time.Minute) }

	serve := func(handler echo.HandlerFunc, names, values []string, cc *campaigns.CampaignContext) (string, error) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.SetParamNames(names...)
		c.SetParamValues(values...)
		if cc != nil {
			c.Set("campaign_context", cc)
		}
		err := handler(c)
		return rec.Body.String(), err
	}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}

	t.Run("campaign flow shows only the calendar", func(t *testing.T) {
		html, err := serve(h.Flow, []string{"id", "eventID"}, []string{"c1", "2"}, cc)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html, "Chronicle refused") || strings.Contains(html, "Port Ashwick") || !strings.Contains(html, "player") {
			t.Fatalf("unexpected flow:\n%s", html)
		}
	})
	t.Run("unknown row", func(t *testing.T) {
		if _, err := serve(h.Flow, []string{"id", "eventID"}, []string{"c1", "999"}, cc); err == nil {
			t.Fatal("a missing row drew a flow")
		}
	})
	t.Run("admin flow centres on the latest failure", func(t *testing.T) {
		html, err := serve(h.AdminFlow, []string{"campaignID"}, []string{"c1"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html, `data-selected="s2-1"`) || strings.Contains(html, "Port Ashwick") || !strings.Contains(html, "Page 2") {
			t.Fatalf("unexpected admin flow:\n%s", html)
		}
	})
	t.Run("admin flow with nothing synced", func(t *testing.T) {
		h2 := NewSyncHistoryHandler(&fakeHistoryRepo{}, &stubCampaignSvcForDmGrant{}, nil, nil)
		html, err := serve(h2.AdminFlow, []string{"campaignID"}, []string{"c1"}, nil)
		if err != nil || !strings.Contains(html, "Nothing has synced yet") {
			t.Fatalf("got %q, %v", html, err)
		}
	})
}
