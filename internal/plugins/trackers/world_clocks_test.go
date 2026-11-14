package trackers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// recDir records the viewer each page check was made for.
type recDir struct {
	*fakeDir
	seen []Viewer
}

func (d *recDir) Access(ctx context.Context, c, id string, v Viewer) (PageAccess, error) {
	d.seen = append(d.seen, v)
	return d.fakeDir.Access(ctx, c, id, v)
}

func sharedRig(t *testing.T) (*rig, *recDir) {
	t.Helper()
	r := newRig(t)
	dir := &recDir{fakeDir: &fakeDir{pages: map[string]PageAccess{
		pageA:   {Exists: true, CanView: true},
		pageHid: {Exists: true, CanView: false},
	}}}
	r.svc = NewService(r.repo, dir, nil)
	return r, dir
}

func TestShared(t *testing.T) {
	r, dir := sharedRig(t)
	flood := r.addKind(t, Kind{ID: "k-flood", Name: "The flood rises", Shape: ShapeClock, Max: ip(6), Bands: []Band{{At: 0, Text: "Puddles"}, {At: 4, Text: "Streets under water at 6"}}})
	trust := r.addKind(t, Kind{ID: "k-trust", Name: "Iron Pact trust", Shape: ShapeSteps, Steps: []Step{{Name: "Hostile"}, {Name: "Wary", Text: "Wary"}, {Name: "Friendly"}}})
	food := r.addKind(t, Kind{ID: "k-food", Name: "Days of food", Shape: ShapeCount, Bands: []Band{{At: 0, Text: "Starving"}, {At: 5, Text: "Running low at 5"}}})
	doom := r.addKind(t, Kind{ID: "k-doom", Name: "Cult doom", Shape: ShapeClock, Max: ip(4), Visibility: SeeDirectors})
	r.addTracker(t, Tracker{ID: "t-flood", KindID: flood.ID, Name: flood.Name})
	r.addTracker(t, Tracker{ID: "t-trust", KindID: trust.ID, Name: trust.Name})
	r.addTracker(t, Tracker{ID: "t-food", KindID: food.ID, Name: food.Name})
	r.addTracker(t, Tracker{ID: "t-doom", KindID: doom.ID, Name: doom.Name, Visibility: SeeDirectors})
	// Shown to everyone, but on a page the viewer may not open, or gone.
	r.addTracker(t, Tracker{ID: "t-hid", KindID: flood.ID, EntityID: pageHid, Name: "Secret page flood"})
	r.addTracker(t, Tracker{ID: "t-gone", KindID: flood.ID, EntityID: pageGone, Name: "Gone page flood"})
	// A tracker whose kind was removed is skipped, not an error.
	r.addTracker(t, Tracker{ID: "t-nokind", KindID: "k-missing", Name: "Orphan"})
	r.repo.entries = []Entry{
		{ID: 1, TrackerID: "t-flood", Op: OpSet, Amount: 4},
		{ID: 2, TrackerID: "t-trust", Op: OpSet, Amount: 1},
		{ID: 3, TrackerID: "t-food", Op: OpSet, Amount: 9},
		{ID: 4, TrackerID: "t-doom", Op: OpSet, Amount: 3},
	}
	ctx := context.Background()
	for name, v := range map[string]Viewer{"player": player, "the owner reads as a player": dm, "guest": {}} {
		t.Run(name, func(t *testing.T) {
			dir.seen = nil
			got, err := r.svc.Shared(ctx, cmp, v)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, s := range got {
				names = append(names, s.Name)
			}
			if want := "Days of food,Iron Pact trust,The flood rises"; strings.Join(names, ",") != want {
				t.Errorf("shown = %v, want %s", names, want)
			}
			for _, seen := range dir.seen {
				if seen.Director || seen.VisibilityRole > 1 || (v.Director && seen.UserID != "") {
					t.Errorf("page checked as %+v, want a plain player's", seen)
				}
			}
			byName := map[string]SharedTracker{}
			for _, s := range got {
				byName[s.Name] = s
			}
			if f := byName["The flood rises"]; f.Value != 4 || !f.Bounded || f.Max != 6 || f.Now != "Streets under water at 6" || f.EntityID != pageA {
				t.Errorf("flood = %+v", f)
			}
			if s := byName["Iron Pact trust"]; s.Value != 1 || s.Count != 3 || s.Now != "Wary" {
				t.Errorf("trust = %+v", s)
			}
			if s := byName["Days of food"]; s.Value != 9 || s.Bounded || s.Now != "Running low at 5" {
				t.Errorf("food = %+v", s)
			}
		})
	}
	t.Run("nothing placed", func(t *testing.T) {
		r2, _ := sharedRig(t)
		got, err := r2.svc.Shared(ctx, cmp, player)
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

func TestRingFor(t *testing.T) {
	tests := []struct {
		name    string
		t       SharedTracker
		label   string
		pct     int
		bounded bool
	}{
		{"clock four of six", SharedTracker{Shape: ShapeClock, Value: 4, Bounded: true, Max: 6}, "4/6", 66, true},
		{"empty clock", SharedTracker{Shape: ShapeClock, Value: 0, Bounded: true, Max: 6}, "0/6", 0, true},
		{"full clock", SharedTracker{Shape: ShapeClock, Value: 6, Bounded: true, Max: 6}, "6/6", 100, true},
		{"count against a floor", SharedTracker{Shape: ShapeCount, Value: 7, Bounded: true, Min: 5, Max: 10}, "7/10", 40, true},
		{"steps count from one", SharedTracker{Shape: ShapeSteps, Value: 1, Count: 4}, "2/4", 50, true},
		{"unbounded count is only a number", SharedTracker{Shape: ShapeCount, Value: 9}, "9", 0, false},
		{"a value past the top stays in the ring", SharedTracker{Shape: ShapeCount, Value: 12, Bounded: true, Max: 10}, "12/10", 100, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ringFor("c1", tt.t)
			if r.Label != tt.label || r.Pct != tt.pct || r.Bounded != tt.bounded {
				t.Errorf("got %+v", r)
			}
		})
	}
}

func TestBuildWorldClocks(t *testing.T) {
	mk := func(n int) []SharedTracker {
		var out []SharedTracker
		for i := 0; i < n; i++ {
			out = append(out, SharedTracker{EntityID: "p", Name: fmt.Sprintf("Clock %d", i), Shape: ShapeCount, Value: i})
		}
		return out
	}
	tests := []struct {
		name      string
		n         int
		owner     bool
		wantRings int
		wantMore  string
		wantHint  bool
	}{
		{"six fit", 6, false, 6, "", false},
		{"nine leave three", 9, false, 6, "3 more, on their pages", false},
		{"none for a player", 0, false, 0, "", false},
		{"none for the owner gets a hint", 0, true, 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := buildWorldClocks("c1", mk(tt.n), tt.owner)
			if len(v.Rings) != tt.wantRings || v.More != tt.wantMore || (v.Hint != "") != tt.wantHint {
				t.Errorf("got %+v", v)
			}
		})
	}
}

func clocksHTML(t *testing.T, v worldClocksView) string {
	t.Helper()
	var b bytes.Buffer
	if err := worldClocksBlock(v).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The block holds the worded rings and nothing of a Director-only tracker, and
// the only inline style is a ring's integer percent.
func TestWorldClocksBlockLeaksNothing(t *testing.T) {
	r, _ := sharedRig(t)
	k := r.addKind(t, Kind{ID: "k1", Name: "Flood", Shape: ShapeClock, Max: ip(6), Bands: []Band{{At: 0, Text: "Puddles"}}})
	d := r.addKind(t, Kind{ID: "k2", Name: "Doom", Shape: ShapeClock, Max: ip(4), Visibility: SeeDirectors})
	r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "The flood rises"})
	r.addTracker(t, Tracker{ID: "t2", KindID: d.ID, Name: "The cult's ritual", Visibility: SeeDirectors})
	r.repo.entries = []Entry{{ID: 1, TrackerID: "t1", Op: OpSet, Amount: 2, Note: "DM NOTE"}, {ID: 2, TrackerID: "t2", Op: OpSet, Amount: 3}}
	for name, v := range map[string]Viewer{"member": player, "owner": dm, "guest": {}} {
		t.Run(name, func(t *testing.T) {
			list, err := r.svc.Shared(context.Background(), cmp, v)
			if err != nil {
				t.Fatal(err)
			}
			html := clocksHTML(t, buildWorldClocks(cmp, list, false))
			for _, not := range []string{"cult", "ritual", "Doom", "DM NOTE", "Director", "dm-only"} {
				if strings.Contains(html, not) {
					t.Errorf("block holds %q:\n%s", not, html)
				}
			}
			for _, want := range []string{"The world's clocks", "The flood rises", "2/6", "Puddles"} {
				if !strings.Contains(html, want) {
					t.Errorf("block missing %q:\n%s", want, html)
				}
			}
			for _, m := range regexp.MustCompile(`style="([^"]*)"`).FindAllStringSubmatch(html, -1) {
				if !regexp.MustCompile(`^background:conic-gradient\(var\(--color-accent\) 0 \d{1,3}%, var\(--color-bg-tertiary\) 0\);?$`).MatchString(m[1]) {
					t.Errorf("unexpected inline style %q", m[1])
				}
			}
		})
	}
	t.Run("the owner's hint", func(t *testing.T) {
		html := clocksHTML(t, buildWorldClocks(cmp, nil, true))
		if !strings.Contains(html, "Nothing is shown to everyone yet") {
			t.Errorf("hint missing:\n%s", html)
		}
	})
}

type sharedStub struct {
	Service
	list  []SharedTracker
	calls int
}

func (s *sharedStub) Shared(context.Context, string, Viewer) ([]SharedTracker, error) {
	s.calls++
	return s.list, nil
}

func TestWorldClocksBlockHandler(t *testing.T) {
	list := []SharedTracker{{EntityID: "p", Name: "The flood rises", Shape: ShapeClock, Value: 2, Bounded: true, Max: 6}}
	placed := func() *campaigns.Campaign {
		camp := &campaigns.Campaign{ID: "c1", IsPublic: true}
		layout := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{
			Blocks: []campaigns.DashboardBlock{{ID: "b", Type: campaigns.BlockWorldClocks}}}}}}}
		full, err := camp.SetRoleDashboardJSON(campaigns.HomeGuest, layout)
		if err != nil {
			t.Fatal(err)
		}
		camp.DashboardLayout = full
		return camp
	}
	member := func(role campaigns.Role) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsMember: true}
	}
	tests := []struct {
		name      string
		cc        *campaigns.CampaignContext
		list      []SharedTracker
		wantBody  string
		wantReads int
	}{
		{"a guest where it is not placed: nothing, nothing read", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}}, list, "", 0},
		{"a guest where it is placed", &campaigns.CampaignContext{Campaign: placed()}, list, "The flood rises", 1},
		{"a guest with no clocks gets no card", &campaigns.CampaignContext{Campaign: placed()}, nil, "", 1},
		{"a player with no clocks gets no card", member(campaigns.RolePlayer), nil, "", 1},
		{"the owner with none is told", member(campaigns.RoleOwner), nil, "Nothing is shown to everyone yet", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &sharedStub{list: tt.list}
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/world-clocks", nil), rec)
			c.Set("campaign_context", tt.cc)
			if err := NewHandler(stub).WorldClocksBlock(c); err != nil {
				t.Fatal(err)
			}
			if stub.calls != tt.wantReads {
				t.Errorf("reads = %d, want %d", stub.calls, tt.wantReads)
			}
			body := rec.Body.String()
			if (tt.wantBody == "") != (body == "") || !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %q, want it to hold %q", body, tt.wantBody)
			}
		})
	}
}
