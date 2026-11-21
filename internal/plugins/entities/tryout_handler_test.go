package entities

// tryout_handler_test.go pins who may use the try-out and that it only reads:
// a signed-in non-member on a live look-around pass, and nobody else. Every
// other viewer gets not found (or, for a member, the creator they already
// have), the stage-one routes and the hero creator keep refusing a visitor,
// and the plan a visitor reads is the sanitised one.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeVisitorPlanner hands back a plan full of what must never reach a
// visitor, and records that it was asked.
type fakeVisitorPlanner struct {
	calls int
	plan  *HeroPlan
}

func (f *fakeVisitorPlanner) VisitorPlan(context.Context, string) (*HeroPlan, error) {
	f.calls++
	return f.plan, nil
}

func hostilePlan() *HeroPlan {
	return &HeroPlan{SystemName: "Test", Steps: []HeroStep{{
		FieldKey: "class", Label: "Class", CanMake: true,
		Choices: []HeroChoice{{Name: "Conduit", Source: "package"}, {Name: "Tactician", Source: "package", State: HeroChoiceStateAskFirst},
			{Name: "Wren's Own", Source: "campaign", Mine: true}},
		Held:  []HeroChoice{{Name: "Fury", Source: "package"}},
		Rules: &HeroStepRules{Limited: true, NotHere: []HeroNotHere{{Name: "Fury", Reason: "Too many"}}},
	}}}
}

type tryoutViewer struct {
	name      string
	user      string
	member    bool
	role      campaigns.Role
	anonymous bool
	until     *time.Time
}

func (v tryoutViewer) cc() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"},
		MemberRole: v.role, IsMember: v.member, IsAnonymous: v.anonymous, VisitUntil: v.until}
}

func tryRig() (*practiceRig, *fakeVisitorPlanner) {
	r := newPracticeRig()
	vp := &fakeVisitorPlanner{plan: hostilePlan()}
	r.h.SetVisitorPlanner(vp)
	return r, vp
}

func (v tryoutViewer) run(t *testing.T, h *Handler, method, target string, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(""))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set("campaign_context", v.cc())
	if v.user != "" {
		c.Set("auth_user_id", v.user)
	}
	return rec, fn(h, c)
}

func code(err error, rec *httptest.ResponseRecorder) int {
	if err != nil {
		return apperror.SafeCode(err)
	}
	return rec.Code
}

func TestTryout_AccessMatrix(t *testing.T) {
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	tests := []struct {
		v        tryoutViewer
		wantPage int
		wantPlan int
		redirect bool
	}{
		{tryoutViewer{name: "anonymous visitor to a public campaign", anonymous: true, role: campaigns.RoleNone}, 404, 404, false},
		{tryoutViewer{name: "signed-in non-member of a public campaign", user: "u9", role: campaigns.RoleNone}, 404, 404, false},
		{tryoutViewer{name: "visitor on a live pass", user: "u9", role: campaigns.RoleNone, until: &future}, 200, 200, false},
		{tryoutViewer{name: "expired pass", user: "u9", role: campaigns.RoleNone, until: &past}, 404, 404, false},
		{tryoutViewer{name: "pass but no account", role: campaigns.RoleNone, until: &future}, 404, 404, false},
		{tryoutViewer{name: "player", user: "u1", member: true, role: campaigns.RolePlayer}, 302, 404, true},
		{tryoutViewer{name: "Director", user: "dm", member: true, role: campaigns.RoleOwner}, 302, 404, true},
	}
	for _, tc := range tests {
		t.Run(tc.v.name, func(t *testing.T) {
			r, vp := tryRig()
			rec, err := tc.v.run(t, r.h, http.MethodGet, "/x", func(h *Handler, c *echo.Context) error { return h.TryoutNew(c) })
			if got := code(err, rec); got != tc.wantPage {
				t.Errorf("page = %d, want %d (%v)", got, tc.wantPage, err)
			}
			if tc.redirect && rec.Header().Get("Location") != "/campaigns/camp-1/characters/practice/new" {
				t.Errorf("redirect = %q", rec.Header().Get("Location"))
			}
			rec, err = tc.v.run(t, r.h, http.MethodGet, "/x", func(h *Handler, c *echo.Context) error { return h.TryoutPlanAPI(c) })
			if got := code(err, rec); got != tc.wantPlan {
				t.Errorf("plan = %d, want %d (%v)", got, tc.wantPlan, err)
			}
			if tc.wantPlan != 200 && vp.calls != 0 {
				t.Errorf("the planner was asked for a viewer who may not read the plan")
			}
		})
	}
}

func TestTryout_NotFoundWhenTheCampaignDoesNotOfferIt(t *testing.T) {
	future := time.Now().Add(time.Hour)
	visitor := tryoutViewer{user: "u9", role: campaigns.RoleNone, until: &future}
	tests := []struct {
		name  string
		tweak func(*practiceRig)
	}{
		{"claiming is off", func(r *practiceRig) { r.h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{}}) }},
		{"no planner", func(r *practiceRig) { r.h.SetVisitorPlanner(nil) }},
		{"no page type players may claim", func(r *practiceRig) { r.h.SetCharacterLists(&fakeCastLists{}) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := tryRig()
			tc.tweak(r)
			for _, fn := range []func(*Handler, *echo.Context) error{(*Handler).TryoutNew, (*Handler).TryoutPlanAPI} {
				rec, err := visitor.run(t, r.h, http.MethodGet, "/x", fn)
				if got := code(err, rec); got != 404 {
					t.Errorf("code = %d, want 404", got)
				}
			}
		})
	}
}

// The plan a visitor reads has none of what the planner's plan held for a
// player: rules, reasons, held entries, seals, a player's own entries.
func TestTryout_PlanServedIsSanitised(t *testing.T) {
	future := time.Now().Add(time.Hour)
	r, _ := tryRig()
	rec, err := tryoutViewer{user: "u9", role: campaigns.RoleNone, until: &future}.run(t, r.h, http.MethodGet, "/x",
		func(h *Handler, c *echo.Context) error { return h.TryoutPlanAPI(c) })
	if err != nil || rec.Code != 200 {
		t.Fatalf("plan: %v %d", err, rec.Code)
	}
	var got HeroPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Tryout || !got.Practice || len(got.Steps) != 1 || len(got.Steps[0].Choices) != 2 {
		t.Fatalf("plan = %+v", got)
	}
	for _, leak := range []string{"Fury", "Too many", "Wren", "ask_first", "canMake", `"rules"`} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("served plan carries %q: %s", leak, rec.Body.String())
		}
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

// The page is the creator in try-out mode: no CSRF token, no save address,
// and the plan address is the read-only one.
func TestTryout_PageCarriesNoWriteAddress(t *testing.T) {
	future := time.Now().Add(time.Hour)
	r, _ := tryRig()
	rec, err := tryoutViewer{user: "u9", role: campaigns.RoleNone, until: &future}.run(t, r.h, http.MethodGet, "/x",
		func(h *Handler, c *echo.Context) error { return h.TryoutNew(c) })
	if err != nil || rec.Code != 200 {
		t.Fatalf("page: %v %d", err, rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`data-tryout="true"`, `data-user-id="u9"`, `data-plan-url="/campaigns/camp-1/characters/try/plan"`, `data-back-url="/campaigns/camp-1/characters"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	for _, bad := range []string{"data-create-url", "data-update-url", "data-csrf", "data-practice="} {
		if strings.Contains(body, bad) {
			t.Errorf("page carries %s", bad)
		}
	}
}

// A visitor still gets refused by every route a player has: the stage-one
// practice routes and the hero creator, whatever the router in front says.
func TestTryout_VisitorStillRefusedEverywhereElse(t *testing.T) {
	future := time.Now().Add(time.Hour)
	visitor := tryoutViewer{user: "u9", role: campaigns.RoleNone, until: &future}
	r, _ := tryRig()
	body := `{"name":"Old Marrow","picks":{"class":{"name":"Conduit"}}}`
	tests := []struct {
		name   string
		method string
		fn     func(*Handler, *echo.Context) error
	}{
		{"POST /characters/practice", http.MethodPost, (*Handler).PracticeCreate},
		{"PUT /characters/practice/:pid", http.MethodPut, (*Handler).PracticeUpdate},
		{"DELETE /characters/practice/:pid", http.MethodDelete, (*Handler).PracticeDelete},
		{"POST /characters/practice/:pid/ask", http.MethodPost, (*Handler).PracticeAsk},
		{"GET /characters/practice/new", http.MethodGet, (*Handler).PracticeNew},
		{"GET /characters/practice/plan", http.MethodGet, (*Handler).PracticePlanAPI},
		{"POST /characters/new", http.MethodPost, (*Handler).CreateHero},
		{"GET /characters/new", http.MethodGet, (*Handler).NewHero},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/x", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "pid", Value: "p1"}})
			c.Set("campaign_context", visitor.cc())
			c.Set("auth_user_id", "u9")
			err := tc.fn(r.h, c)
			if got := code(err, rec); got != 403 && got != 404 {
				t.Errorf("a visitor got %d from %s, want 403 or 404", got, tc.name)
			}
		})
	}
	if len(r.svc.created) != 0 || len(r.repo.rows) != 0 {
		t.Errorf("a visitor wrote something: pages %v, practice heroes %v", r.svc.created, r.repo.rows)
	}
}

// The Characters page mounts: a visitor gets the Practice section shell, a
// player the offer, a hero-owning player the offer worded for practice, and
// everyone else nothing. The shells carry no stored hero.
func TestTryout_CharactersPageMounts(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name     string
		v        tryoutViewer
		owns     bool
		want     string // "" for no mount
		wantHTML []string
	}{
		{name: "visitor on a pass", v: tryoutViewer{user: "u9", role: campaigns.RoleNone, until: &future}, want: "visitor",
			wantHTML: []string{"Practice", "kept on this device", "Try the hero builder", "/campaigns/camp-1/characters/try", "Only you can see this. It stays in this browser."}},
		{name: "player without a hero", v: tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer}, want: "offer",
			wantHTML: []string{`data-mode="offer"`, `data-has-hero="false"`, `data-base="/campaigns/camp-1"`}},
		{name: "player with a hero", v: tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer}, owns: true, want: "offer",
			wantHTML: []string{`data-has-hero="true"`}},
		{name: "Scribe", v: tryoutViewer{user: "s", member: true, role: campaigns.RoleScribe}, want: ""},
		{name: "ordinary signed-in reader", v: tryoutViewer{user: "u9", role: campaigns.RoleNone}, want: ""},
		{name: "anonymous", v: tryoutViewer{anonymous: true, role: campaigns.RoleNone}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := tryRig()
			if tc.owns {
				r.svc.owned["u1"] = []Entity{{ID: "tess", CampaignID: "camp-1", Name: "Tess", EntityTypeID: practiceChar}}
			}
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.Set("campaign_context", tc.v.cc())
			if tc.v.user != "" {
				c.Set("auth_user_id", tc.v.user)
			}
			m := r.h.tryoutMount(c, tc.v.cc())
			switch tc.want {
			case "":
				if m != nil {
					t.Fatalf("mount = %+v, want none", m)
				}
				return
			case "visitor":
				if m == nil || !m.Visitor {
					t.Fatalf("mount = %+v, want the visitor's", m)
				}
			default:
				if m == nil || m.Visitor || m.HasHero != tc.owns {
					t.Fatalf("mount = %+v, want an offer (has hero %v)", m, tc.owns)
				}
			}
			html := renderCastContent(t, CastView{Tryout: m})
			for _, want := range tc.wantHTML {
				if !strings.Contains(html, want) {
					t.Errorf("page lacks %q:\n%s", want, html)
				}
			}
		})
	}
}
