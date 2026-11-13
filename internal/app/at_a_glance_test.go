package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type fakeGlanceAddons struct {
	list []addons.CampaignAddon
	err  error
}

func (f fakeGlanceAddons) ListForCampaign(context.Context, string) ([]addons.CampaignAddon, error) {
	return f.list, f.err
}

type fakeGlanceNights struct {
	next     *sessions.NextNight
	last     *sessions.Session
	lastErr  error
	asked    bool
	lastDate string
}

func (f *fakeGlanceNights) NextGameNight(context.Context, string, time.Time) (*sessions.NextNight, error) {
	f.asked = true
	return f.next, nil
}

func (f *fakeGlanceNights) LastGameNight(_ context.Context, _, today string) (*sessions.Session, error) {
	f.lastDate = today
	return f.last, f.lastErr
}

type fakeGlancePages struct {
	types  []entities.EntityType
	counts map[int]int
	role   int
}

func (f *fakeGlancePages) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}

func (f *fakeGlancePages) CountByType(_ context.Context, _ string, role int, _ string) (map[int]int, error) {
	f.role = role
	return f.counts, nil
}

func glanceAddonList(system string, nights bool) []addons.CampaignAddon {
	list := []addons.CampaignAddon{}
	if system != "" {
		list = append(list, addons.CampaignAddon{Enabled: true, AddonSlug: "dnd5e", AddonName: system, AddonCategory: addons.CategorySystem})
	}
	if nights {
		list = append(list,
			addons.CampaignAddon{Enabled: true, AddonSlug: calendar.PluginSlug},
			addons.CampaignAddon{Enabled: true, AddonSlug: sessions.SessionsAddonSlug})
	}
	return list
}

func glanceLabels(facts []glanceFact) string {
	var out []string
	for _, f := range facts {
		out = append(out, f.Label+"="+f.Value)
	}
	return strings.Join(out, "; ")
}

// The sheet says only what the viewer may know: game nights are for members,
// and a source that fails or is switched off leaves its row out.
func TestBuildAtAGlance(t *testing.T) {
	now := time.Date(2026, time.November, 11, 18, 0, 0, 0, time.UTC)
	fri := time.Date(2026, time.November, 13, 19, 0, 0, 0, time.UTC)
	lastDate := "2026-11-06"
	parent := 1
	pages := func() *fakeGlancePages {
		return &fakeGlancePages{
			types:  []entities.EntityType{{ID: 1}, {ID: 2, ParentTypeID: &parent}, {ID: 3}},
			counts: map[int]int{1: 10, 2: 4, 3: 7}, // 1 already holds its sub-category's 4
		}
	}
	nights := func() *fakeGlanceNights {
		return &fakeGlanceNights{
			next: &sessions.NextNight{SessionID: "s9", Name: "Session 13", Date: "2026-11-13", Time: "19:00", At: fri},
			last: &sessions.Session{ID: "s8", Name: "Session 12", ScheduledDate: &lastDate},
		}
	}
	tests := []struct {
		name string
		src  func() glanceSources
		req  glanceRequest
		want string
	}{
		{"member sees all four", func() glanceSources {
			return glanceSources{Addons: fakeGlanceAddons{list: glanceAddonList("D&D 5e", true)}, Nights: nights(), Pages: pages()}
		}, glanceRequest{Member: true}, "System=D&D 5e; Next game=Friday, 7 pm; Last session=Session 12; Pages=17 pages"},
		{"visitor gets no game nights", func() glanceSources {
			return glanceSources{Addons: fakeGlanceAddons{list: glanceAddonList("D&D 5e", true)}, Nights: nights(), Pages: pages()}
		}, glanceRequest{}, "System=D&D 5e; Pages=17 pages"},
		{"game nights off", func() glanceSources {
			return glanceSources{Addons: fakeGlanceAddons{list: glanceAddonList("", false)}, Nights: nights(), Pages: pages()}
		}, glanceRequest{Member: true}, "Pages=17 pages"},
		{"sessions plugin down", func() glanceSources {
			return glanceSources{Addons: fakeGlanceAddons{list: glanceAddonList("", true)}, Nights: nights(), Pages: pages(), NightsHealthy: func() bool { return false }}
		}, glanceRequest{Member: true}, "Pages=17 pages"},
		{"addons fail: no system, no nights", func() glanceSources {
			return glanceSources{Addons: fakeGlanceAddons{err: errors.New("db down")}, Nights: nights(), Pages: pages()}
		}, glanceRequest{Member: true}, "Pages=17 pages"},
		{"last session fails, the rest stays", func() glanceSources {
			n := nights()
			n.lastErr = errors.New("db down")
			return glanceSources{Addons: fakeGlanceAddons{list: glanceAddonList("", true)}, Nights: n, Pages: pages()}
		}, glanceRequest{Member: true}, "Next game=Friday, 7 pm; Pages=17 pages"},
		{"no pages: a visitor sees no count", func() glanceSources {
			return glanceSources{Pages: &fakeGlancePages{types: []entities.EntityType{{ID: 1}}, counts: map[int]int{}}}
		}, glanceRequest{}, ""},
		{"no pages: the Director sees 0", func() glanceSources {
			return glanceSources{Pages: &fakeGlancePages{types: []entities.EntityType{{ID: 1}}, counts: map[int]int{}}}
		}, glanceRequest{Director: true}, "Pages=0 pages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.CampaignID, tt.req.Now = "c1", now
			got := glanceLabels(buildAtAGlance(context.Background(), tt.src(), tt.req))
			if got != tt.want {
				t.Errorf("facts = %q\nwant    %q", got, tt.want)
			}
		})
	}
}

// "Last session" is before the viewer's own today: 02:00 UTC on the 12th is
// still the 11th in New York.
func TestBuildAtAGlance_LastSessionUsesViewerDay(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	n := &fakeGlanceNights{}
	src := glanceSources{Addons: fakeGlanceAddons{list: glanceAddonList("", true)}, Nights: n}
	buildAtAGlance(context.Background(), src, glanceRequest{CampaignID: "c1", Member: true, Now: time.Date(2026, time.November, 12, 2, 0, 0, 0, time.UTC), Loc: ny})
	if n.lastDate != "2026-11-11" {
		t.Errorf("asked for nights before %q, want 2026-11-11", n.lastDate)
	}
}

func TestGlanceNextLabel(t *testing.T) {
	now := time.Date(2026, time.November, 11, 18, 0, 0, 0, time.UTC)
	tests := []struct {
		at   time.Time
		hhmm string
		want string
	}{
		{time.Date(2026, time.November, 11, 19, 0, 0, 0, time.UTC), "19:00", "Today, 7 pm"},
		{time.Date(2026, time.November, 12, 19, 30, 0, 0, time.UTC), "19:30", "Tomorrow, 7:30 pm"},
		{time.Date(2026, time.November, 14, 23, 59, 0, 0, time.UTC), "", "Saturday"},
		{time.Date(2026, time.December, 4, 18, 0, 0, 0, time.UTC), "18:00", "Friday 4 December, 6 pm"},
	}
	for _, tt := range tests {
		if got := glanceNextLabel(&sessions.NextNight{At: tt.at, Time: tt.hhmm}, now); got != tt.want {
			t.Errorf("%v: got %q, want %q", tt.at, got, tt.want)
		}
	}
}

// A Director viewing as a player is counted as a player, and the sheet
// renders; nothing to say renders nothing, so the home drops the block.
func TestAtAGlanceHandler(t *testing.T) {
	tests := []struct {
		name     string
		pages    *fakeGlancePages
		cookie   bool
		wantRole int
		wantBody string
	}{
		{"owner", &fakeGlancePages{types: []entities.EntityType{{ID: 1}}, counts: map[int]int{1: 3}}, false, int(campaigns.RoleOwner), "3 pages"},
		{"owner viewing as a player", &fakeGlancePages{types: []entities.EntityType{{ID: 1}}, counts: map[int]int{1: 2}}, true, int(campaigns.RolePlayer), "2 pages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &atAGlanceHandler{src: glanceSources{Pages: tt.pages}, now: time.Now}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/at-a-glance", nil)
			if tt.cookie {
				req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true})
			if err := h.Show(c); err != nil {
				t.Fatal(err)
			}
			if tt.pages.role != tt.wantRole {
				t.Errorf("counted as role %d, want %d", tt.pages.role, tt.wantRole)
			}
			if body := rec.Body.String(); !strings.Contains(body, tt.wantBody) || !strings.Contains(body, `data-block="at-a-glance"`) {
				t.Errorf("body = %q", body)
			}
		})
	}

	h := &atAGlanceHandler{src: glanceSources{}, now: time.Now}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/at-a-glance", nil), rec)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}})
	if err := h.Show(c); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("nothing to say should be an empty body, got %q", rec.Body.String())
	}
}
