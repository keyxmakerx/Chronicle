package entities

// hero_terms_test.go pins the builder's rules words: who may read them on each
// mount's route (the same people who may read that mount's plan), the wire
// shape and caching, and that each mount carries data-terms-url only when the
// system has a glossary and data-skin from the campaign's builder look.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

type fakeTerms struct {
	terms *HeroTerms
	err   error
	calls int
}

func (f *fakeTerms) HeroTerms(context.Context, string) (*HeroTerms, error) {
	f.calls++
	return f.terms, f.err
}

func someTerms() *HeroTerms {
	return &HeroTerms{Present: true, Terms: map[string]HeroTerm{
		"slowed": {Name: "Slowed", Category: "condition", Text: "Speed is 2."},
	}}
}

// termsCreatorHandler is the creator rig of TestHeroPlanAPI_Access: a claimable
// hero type, and a player who has none yet.
func termsCreatorHandler(terms HeroTermsReader) *Handler {
	yes := true
	h := &Handler{
		service:     &planAccessSvc{types: []EntityType{{ID: 1, Name: "Hero", Enabled: true, Claimable: &yes}}},
		charLists:   planAccessLists{ids: []int{1}},
		heroPlanner: &stubPlanner{plan: &HeroPlan{Steps: []HeroStep{}}},
	}
	if terms != nil {
		h.SetHeroTerms(terms)
	}
	return h
}

// serve runs fn as the viewer and returns the recorder and error.
func serve(t *testing.T, ctx context.Context, v tryoutViewer, h *Handler, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set("campaign_context", v.cc())
	if v.user != "" {
		c.Set("auth_user_id", v.user)
	}
	return rec, fn(h, c)
}

func TestHeroTermsAPI_Access(t *testing.T) {
	tests := []struct {
		name string
		v    tryoutViewer
		want int
	}{
		{"player who may make a hero", tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer}, http.StatusOK},
		{"Director", tryoutViewer{user: "dm", member: true, role: campaigns.RoleOwner}, http.StatusOK},
		{"stranger", tryoutViewer{user: "u9", role: campaigns.RoleNone}, http.StatusForbidden},
		{"anonymous", tryoutViewer{anonymous: true, role: campaigns.RoleNone}, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ft := &fakeTerms{terms: someTerms()}
			rec, err := serve(t, context.Background(), tc.v, termsCreatorHandler(ft), (*Handler).HeroTermsAPI)
			if got := code(err, rec); got != tc.want {
				t.Fatalf("status = %d, want %d (%v)", got, tc.want, err)
			}
			if tc.want != http.StatusOK && ft.calls != 0 {
				t.Error("the words were read for a viewer who may not have them")
			}
		})
	}
}

func TestHeroTermsAPI_Response(t *testing.T) {
	player := tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer}
	tests := []struct {
		name  string
		terms HeroTermsReader
		want  string
	}{
		{"words are keyed and plain", &fakeTerms{terms: someTerms()},
			`{"terms":{"slowed":{"name":"Slowed","category":"condition","text":"Speed is 2."}}}`},
		{"system with no glossary answers empty", &fakeTerms{terms: &HeroTerms{Terms: map[string]HeroTerm{}}}, `{"terms":{}}`},
		{"nil words answer empty", &fakeTerms{}, `{"terms":{}}`},
		{"no reader wired answers empty", nil, `{"terms":{}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := serve(t, context.Background(), player, termsCreatorHandler(tc.terms), (*Handler).HeroTermsAPI)
			if err != nil || rec.Code != http.StatusOK {
				t.Fatalf("status = %d, err = %v", rec.Code, err)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tc.want {
				t.Errorf("body = %s, want %s", got, tc.want)
			}
			if cc := rec.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "private") {
				t.Errorf("Cache-Control = %q, want private", cc)
			}
		})
	}
}

func TestHeroTermsAPI_ReadFailureIsAnError(t *testing.T) {
	ft := &fakeTerms{err: errors.New("glossary broke")}
	_, err := serve(t, context.Background(), tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer},
		termsCreatorHandler(ft), (*Handler).HeroTermsAPI)
	if err == nil {
		t.Fatal("a failed read must not look like an empty glossary")
	}
}

func TestTryoutTermsAPI_Access(t *testing.T) {
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	tests := []struct {
		v    tryoutViewer
		want int
	}{
		{tryoutViewer{name: "visitor on a live pass", user: "u9", role: campaigns.RoleNone, until: &future}, 200},
		{tryoutViewer{name: "expired pass", user: "u9", role: campaigns.RoleNone, until: &past}, 404},
		{tryoutViewer{name: "signed-in reader without a pass", user: "u9", role: campaigns.RoleNone}, 404},
		{tryoutViewer{name: "anonymous", anonymous: true, role: campaigns.RoleNone}, 404},
		{tryoutViewer{name: "player", user: "u1", member: true, role: campaigns.RolePlayer}, 404},
		{tryoutViewer{name: "Director", user: "dm", member: true, role: campaigns.RoleOwner}, 404},
	}
	for _, tc := range tests {
		t.Run(tc.v.name, func(t *testing.T) {
			r, _ := tryRig()
			ft := &fakeTerms{terms: someTerms()}
			r.h.SetHeroTerms(ft)
			rec, err := serve(t, context.Background(), tc.v, r.h, (*Handler).TryoutTermsAPI)
			if got := code(err, rec); got != tc.want {
				t.Fatalf("status = %d, want %d (%v)", got, tc.want, err)
			}
			if tc.want == 200 && !strings.Contains(rec.Body.String(), `"slowed"`) {
				t.Errorf("body = %s", rec.Body.String())
			}
			if tc.want != 200 && ft.calls != 0 {
				t.Error("the words were read for a viewer who may not have them")
			}
		})
	}
}

func TestPracticeTermsAPI_Access(t *testing.T) {
	tests := []struct {
		name string
		v    viewer
		want int
	}{
		{"player", vPat, 200},
		{"Scribe", vScribe, 404},
		{"Director", vDirector, 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newPracticeRig()
			ft := &fakeTerms{terms: someTerms()}
			r.h.SetHeroTerms(ft)
			rec, err := r.call(t, tc.v, http.MethodGet, "/x", nil, "", (*Handler).PracticeTermsAPI)
			if got := code(err, rec); got != tc.want {
				t.Fatalf("status = %d, want %d (%v)", got, tc.want, err)
			}
			if tc.want != 200 && ft.calls != 0 {
				t.Error("the words were read for a viewer who may not have them")
			}
		})
	}
}

// A mount names the words' address only when the system has a glossary, and
// wears the campaign's builder look, Plain when none was chosen.
func TestBuilderMounts_TermsURLAndSkin(t *testing.T) {
	future := time.Now().Add(time.Hour)
	visitor := tryoutViewer{user: "u9", role: campaigns.RoleNone, until: &future}
	player := tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer}

	mounts := []struct {
		name string
		url  string
		page func(t *testing.T, ctx context.Context, terms HeroTermsReader) string
	}{
		{"creator", "/campaigns/camp-1/characters/new/terms", func(t *testing.T, ctx context.Context, terms HeroTermsReader) string {
			rec, err := serve(t, ctx, player, termsCreatorHandler(terms), (*Handler).NewHero)
			if err != nil || rec.Code != 200 {
				t.Fatalf("creator page: %v %d", err, rec.Code)
			}
			return rec.Body.String()
		}},
		{"try-out", "/campaigns/camp-1/characters/try/terms", func(t *testing.T, ctx context.Context, terms HeroTermsReader) string {
			r, _ := tryRig()
			if terms != nil {
				r.h.SetHeroTerms(terms)
			}
			rec, err := serve(t, ctx, visitor, r.h, (*Handler).TryoutNew)
			if err != nil || rec.Code != 200 {
				t.Fatalf("try-out page: %v %d", err, rec.Code)
			}
			return rec.Body.String()
		}},
		{"practice", "/campaigns/camp-1/characters/practice/terms", func(t *testing.T, ctx context.Context, terms HeroTermsReader) string {
			r := newPracticeRig()
			if terms != nil {
				r.h.SetHeroTerms(terms)
			}
			rec, err := r.callCtx(t, ctx, vPat, (*Handler).PracticeNew)
			if err != nil || rec.Code != 200 {
				t.Fatalf("practice page: %v %d", err, rec.Code)
			}
			return rec.Body.String()
		}},
	}
	looks := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"no Customize", context.Background(), "plain"},
		{"Customize without a builder look", layouts.SetAppearance(context.Background(), &layouts.AppearanceData{}), "plain"},
		{"paper", layouts.SetAppearance(context.Background(), &layouts.AppearanceData{BuilderLook: "paper"}), "paper"},
		{"ornate", layouts.SetAppearance(context.Background(), &layouts.AppearanceData{BuilderLook: "ornate"}), "ornate"},
		{"an unknown look is plain", layouts.SetAppearance(context.Background(), &layouts.AppearanceData{BuilderLook: "neon"}), "plain"},
	}
	for _, m := range mounts {
		for _, l := range looks {
			t.Run(m.name+"/"+l.name, func(t *testing.T) {
				body := m.page(t, l.ctx, &fakeTerms{terms: someTerms()})
				if want := `data-skin="` + l.want + `"`; !strings.Contains(body, want) {
					t.Errorf("mount lacks %s", want)
				}
				if want := `data-terms-url="` + m.url + `"`; !strings.Contains(body, want) {
					t.Errorf("mount lacks %s", want)
				}
			})
		}
		for _, tc := range []struct {
			name  string
			terms HeroTermsReader
		}{
			{"no glossary", &fakeTerms{terms: &HeroTerms{Terms: map[string]HeroTerm{}}}},
			{"no reader wired", nil},
			{"glossary fails to read", &fakeTerms{err: errors.New("broke")}},
		} {
			t.Run(m.name+"/"+tc.name+" leaves the URL off", func(t *testing.T) {
				body := m.page(t, context.Background(), tc.terms)
				if strings.Contains(body, "data-terms-url") {
					t.Error("mount names a terms address the system cannot serve")
				}
				if !strings.Contains(body, `data-skin="plain"`) {
					t.Error("the skin must be on every mount")
				}
			})
		}
	}
}

// callCtx runs a practice-page handler as v with ctx on the request, so the
// campaign's look reaches the template the way the layout injector sets it.
func (r *practiceRig) callCtx(t *testing.T, ctx context.Context, v viewer, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set("auth_user_id", v.id)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: v.role, IsMember: true,
	})
	return rec, fn(r.h, c)
}

func TestHeroTermsJSONShape(t *testing.T) {
	raw, err := json.Marshal(someTerms())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Present") || strings.Contains(string(raw), "present") {
		t.Errorf("the wire shape carries the internal flag: %s", raw)
	}
}
