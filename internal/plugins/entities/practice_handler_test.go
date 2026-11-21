package entities

// practice_handler_test.go pins how practice heroes meet the rest of the app:
// who may open one, that making them never touches the one-hero rule, and
// that a Director's yes makes the real hero through the hero creator's own
// function, releases the old hero after it, and never loses the practice hero
// on a failure.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const practiceChar = 7

// heroSvc is the entity service as the practice flows use it: a list of pages,
// the creates made, and an order of events shared with the releaser.
type heroSvc struct {
	EntityService
	owned   map[string][]Entity // by owner
	created []CreateEntityInput
	by      []string
	failNew error
	events  *[]string
}

func (s *heroSvc) ListByOwner(_ context.Context, _, userID string) ([]Entity, error) {
	return s.owned[userID], nil
}
func (s *heroSvc) ListClaimed(context.Context, string, int, string) ([]Entity, error) { return nil, nil }
func (s *heroSvc) ListRoster(context.Context, string, []int, int, string) ([]Entity, error) {
	return nil, nil
}
func (s *heroSvc) CountByType(context.Context, string, int, string) (map[int]int, error) {
	return map[int]int{}, nil
}
func (s *heroSvc) GetEntityTypes(context.Context, string) ([]EntityType, error) {
	yes := true
	return []EntityType{{ID: practiceChar, Name: "Character", Enabled: true, Claimable: &yes,
		Fields: []FieldDefinition{{Key: "level", Type: "number"}}}}, nil
}
func (s *heroSvc) Create(_ context.Context, campaignID, userID string, in CreateEntityInput) (*Entity, error) {
	if s.failNew != nil {
		return nil, s.failNew
	}
	s.created = append(s.created, in)
	s.by = append(s.by, userID)
	*s.events = append(*s.events, "create")
	e := Entity{ID: "new-hero", CampaignID: campaignID, Name: in.Name, EntityTypeID: in.EntityTypeID, FieldsData: in.FieldsData}
	if in.OwnerUserID != nil {
		e.OwnerUserID = in.OwnerUserID
		s.owned[*in.OwnerUserID] = append(s.owned[*in.OwnerUserID], e)
	}
	return &e, nil
}

type fakeReleaser struct {
	events *[]string
	freed  []string
	fail   error
}

func (r *fakeReleaser) ReleaseOne(_ context.Context, _, entityID, _ string) (bool, error) {
	if r.fail != nil {
		return false, r.fail
	}
	*r.events = append(*r.events, "release")
	r.freed = append(r.freed, entityID)
	return true, nil
}

type practiceRig struct {
	h        *Handler
	svc      *heroSvc
	repo     *fakePracticeRepo
	planner  *fakePlanner
	releaser *fakeReleaser
	events   *[]string
}

func newPracticeRig() *practiceRig {
	events := &[]string{}
	svc := &heroSvc{owned: map[string][]Entity{}, events: events}
	repo, planner := newFakePracticeRepo(), &fakePlanner{plan: testHeroPlan()}
	rel := &fakeReleaser{events: events}
	h := NewHandler(svc)
	h.SetCharacterLists(&fakeCastLists{chars: []int{practiceChar}, chosen: CharacterLists{CharacterTypeIDs: []int{practiceChar}}})
	h.SetMemberLister(rosterMembers{})
	h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})
	h.SetHeroPlanner(planner)
	h.SetPracticeService(NewPracticeService(repo, planner))
	h.SetHeroReleaser(rel)
	return &practiceRig{h: h, svc: svc, repo: repo, planner: planner, releaser: rel, events: events}
}

type viewer struct {
	id   string
	role campaigns.Role
	dm   bool
}

var (
	vPat      = viewer{"u1", campaigns.RolePlayer, false}
	vSam      = viewer{"u2", campaigns.RolePlayer, false}
	vScribe   = viewer{"scribe", campaigns.RoleScribe, false}
	vDirector = viewer{"dm", campaigns.RoleOwner, true}
)

func (r *practiceRig) call(t *testing.T, v viewer, method, target string, body any, pid string, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	var rd *strings.Reader
	ctype := echo.MIMEApplicationJSON
	switch b := body.(type) {
	case nil:
		rd = strings.NewReader("")
	case url.Values:
		rd, ctype = strings.NewReader(b.Encode()), echo.MIMEApplicationForm
	default:
		raw, _ := json.Marshal(b)
		rd = strings.NewReader(string(raw))
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set(echo.HeaderContentType, ctype)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	pv := echo.PathValues{{Name: "id", Value: "camp-1"}}
	if pid != "" {
		pv = append(pv, echo.PathValue{Name: "pid", Value: pid})
	}
	c.SetPathValues(pv)
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: v.role, IsMember: true, IsDmGranted: v.dm}
	c.Set("campaign_context", cc)
	c.Set("auth_user_id", v.id)
	return rec, fn(r.h, c)
}

// seed saves a practice hero for a maker straight into the repo.
func (r *practiceRig) seed(maker, name string, picks map[string]HeroPick, status string) string {
	id := generateUUID()
	r.repo.rows[id] = PracticeHero{ID: id, CampaignID: "camp-1", UserID: maker, Name: name, Picks: picks, AskStatus: status}
	return id
}

func TestPractice_MakingThemNeverTouchesTheOneHeroRule(t *testing.T) {
	r := newPracticeRig()
	r.svc.owned["u1"] = []Entity{{ID: "tess", CampaignID: "camp-1", Name: "Tess", EntityTypeID: practiceChar}}

	// A player WITH a hero may make practice heroes.
	for i := 0; i < MaxPracticeHeroes; i++ {
		rec, err := r.call(t, vPat, http.MethodPost, "/x", map[string]any{"name": "Try", "picks": map[string]any{"class": map[string]any{"name": "Fury"}}}, "",
			func(h *Handler, c *echo.Context) error { return h.PracticeCreate(c) })
		if err != nil || rec.Code != http.StatusCreated {
			t.Fatalf("create %d: %v code %d", i, err, rec.Code)
		}
	}
	if got := r.svc.owned["u1"]; len(got) != 1 || got[0].ID != "tess" {
		t.Errorf("their pages changed: %+v", got)
	}
	if len(r.svc.created) != 0 {
		t.Errorf("a practice hero created a page: %+v", r.svc.created)
	}

	// A player with three practice heroes and no hero may still make a real one.
	r2 := newPracticeRig()
	for i := 0; i < MaxPracticeHeroes; i++ {
		r2.seed("u1", "Try", nil, "")
	}
	hc := r2.h
	c := func() *echo.Context {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		ctx := echo.New().NewContext(req, httptest.NewRecorder())
		ctx.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true})
		ctx.Set("auth_user_id", "u1")
		return ctx
	}()
	_, access, err := hc.heroTarget(c, campaigns.GetCampaignContext(c))
	if err != nil || !access.Allowed || !access.ForSelf {
		t.Errorf("one-hero check with three practice heroes: %+v %v", access, err)
	}
}

func TestPractice_WhoMayOpen(t *testing.T) {
	r := newPracticeRig()
	id := r.seed("u1", "Old Marrow", map[string]HeroPick{"class": {Name: "Fury"}}, "")
	tests := []struct {
		name string
		v    viewer
		pid  string
		code int
	}{
		{"the maker", vPat, id, 200},
		{"a Director", vDirector, id, 200},
		{"another player is told it is not there", vSam, id, 404},
		{"a Scribe who is not a Director is told it is not there", vScribe, id, 404},
		{"an unknown id", vPat, "nope", 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := r.call(t, tc.v, http.MethodGet, "/x", nil, tc.pid, func(h *Handler, c *echo.Context) error { return h.PracticeShow(c) })
			if tc.code != 200 {
				assertAppError(t, err, tc.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			for _, want := range []string{"Practice hero.", "Not in the party, not sent to Foundry.", "Old Marrow", "Class: Fury",
				"Not offered at this table. Needs the Director&#39;s yes to play."} {
				if !strings.Contains(body, want) {
					t.Errorf("page lacks %q", want)
				}
			}
			// The maker gets the buttons; a Director viewing gets none.
			hasAsk := strings.Contains(body, "Ask to play this one")
			if hasAsk != (tc.v.id == "u1") {
				t.Errorf("ask button shown = %v for %s", hasAsk, tc.v.id)
			}
		})
	}
}

func TestPractice_NotAvailableWhenOffOrNotAPlayer(t *testing.T) {
	r := newPracticeRig()
	r.h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: false}})
	_, err := r.call(t, vPat, http.MethodGet, "/x", nil, "", func(h *Handler, c *echo.Context) error { return h.PracticeNew(c) })
	assertAppError(t, err, 404)

	r = newPracticeRig()
	visitor := viewer{"v", campaigns.RoleNone, false}
	_, err = r.call(t, visitor, http.MethodPost, "/x", map[string]any{"name": "x"}, "", func(h *Handler, c *echo.Context) error { return h.PracticeCreate(c) })
	assertAppError(t, err, 404)
}

func TestPractice_CopyOfMyHero(t *testing.T) {
	r := newPracticeRig()
	r.svc.owned["u1"] = []Entity{{ID: "tess", CampaignID: "camp-1", Name: "Tess", EntityTypeID: practiceChar,
		FieldsData: map[string]any{"ancestry": "Dwarf", "class": "Conduit", "level": float64(3)}}}
	rec, err := r.call(t, vPat, http.MethodPost, "/x", map[string]any{"name": "Tess as a Fury", "copy": true,
		"picks": map[string]any{"class": map[string]any{"name": "Fury"}}}, "", func(h *Handler, c *echo.Context) error { return h.PracticeCreate(c) })
	if err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create: %v", err)
	}
	var made *PracticeHero
	for _, h := range r.repo.rows {
		h := h
		made = &h
	}
	if made.CopiedFromName != "Tess" || made.Level == nil || *made.Level != 3 {
		t.Errorf("copy record = %+v", made)
	}
	// The preload for a copy carries the hero's own picks.
	rec, err = r.call(t, vPat, http.MethodGet, "/x?from=hero", nil, "", func(h *Handler, c *echo.Context) error { return h.PracticeNew(c) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `data-practice="true"`) || !strings.Contains(rec.Body.String(), "Conduit") {
		t.Errorf("creator page: %s", rec.Body.String())
	}
	// No hero to copy is a plain refusal.
	r2 := newPracticeRig()
	_, err = r2.call(t, vPat, http.MethodGet, "/x?from=hero", nil, "", func(h *Handler, c *echo.Context) error { return h.PracticeNew(c) })
	assertAppError(t, err, 400)
}

func TestPractice_DirectorsYes(t *testing.T) {
	t.Run("makes the real hero, then releases the old one, then the practice hero is gone", func(t *testing.T) {
		r := newPracticeRig()
		r.svc.owned["u1"] = []Entity{{ID: "tess", CampaignID: "camp-1", Name: "Tess", EntityTypeID: practiceChar}}
		lvl := 3
		id := r.seed("u1", "Tess as a Fury", map[string]HeroPick{"class": {Name: "Fury"}, "ancestry": {Name: "Human", Options: []string{"Tough"}}}, PracticeAskWaiting)
		h := r.repo.rows[id]
		h.Level = &lvl
		r.repo.rows[id] = h

		rec, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if got := strings.Join(*r.events, ","); got != "create,release" {
			t.Errorf("order = %s, want create then release", got)
		}
		if len(r.svc.created) != 1 {
			t.Fatalf("created %d pages", len(r.svc.created))
		}
		in := r.svc.created[0]
		if in.Name != "Tess as a Fury" || in.OwnerUserID == nil || *in.OwnerUserID != "u1" || !in.Playable || in.EntityTypeID != practiceChar {
			t.Errorf("hero input = %+v", in)
		}
		if in.FieldsData["class"] != "Fury" || in.FieldsData["ancestry"] != "Human" || in.FieldsData["level"] != float64(3) {
			t.Errorf("fields = %+v", in.FieldsData)
		}
		if r.svc.by[0] != "u1" {
			t.Errorf("created by %q", r.svc.by[0])
		}
		if len(r.releaser.freed) != 1 || r.releaser.freed[0] != "tess" {
			t.Errorf("released %+v", r.releaser.freed)
		}
		if _, still := r.repo.rows[id]; still {
			t.Error("the practice hero is still on the shelf")
		}
		// The plan used for the yes is the unlimited one as the maker sees it.
		last := len(r.planner.users) - 1
		if !r.planner.unlimited[last] || r.planner.directors[last] || r.planner.users[last] != "u1" {
			t.Errorf("planner: unlimited=%v director=%v user=%q", r.planner.unlimited[last], r.planner.directors[last], r.planner.users[last])
		}
		if !strings.Contains(rec.Body.String(), "is now") {
			t.Errorf("no notice: %s", rec.Body.String())
		}
	})

	t.Run("a player with no hero has nothing to release", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "New", nil, PracticeAskWaiting)
		if _, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) }); err != nil {
			t.Fatal(err)
		}
		if len(r.releaser.freed) != 0 || len(r.svc.created) != 1 {
			t.Errorf("freed %v created %d", r.releaser.freed, len(r.svc.created))
		}
	})

	t.Run("a refused yes leaves everything alone", func(t *testing.T) {
		tests := []struct {
			name   string
			v      viewer
			maker  string
			status string
			code   int
		}{
			{"a player cannot say yes to their own", vPat, "u1", PracticeAskWaiting, 403},
			{"a Scribe cannot say yes", vScribe, "u1", PracticeAskWaiting, 404},
			{"nobody asked", vDirector, "u1", "", 409},
			{"already answered no", vDirector, "u1", PracticeAskDeclined, 409},
			{"the maker has left", vDirector, "gone", PracticeAskWaiting, 409},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				r := newPracticeRig()
				id := r.seed(tc.maker, "X", nil, tc.status)
				_, err := r.call(t, tc.v, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
				assertAppError(t, err, tc.code)
				if len(r.svc.created) != 0 || len(r.releaser.freed) != 0 {
					t.Error("a refused yes changed something")
				}
				if _, ok := r.repo.rows[id]; !ok {
					t.Error("a refused yes removed the practice hero")
				}
			})
		}
	})

	t.Run("an entry no longer on the list refuses before anything changes", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "X", map[string]HeroPick{"class": {Name: "Removed Class"}}, PracticeAskWaiting)
		_, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		assertAppError(t, err, 400)
		if _, ok := r.repo.rows[id]; !ok || len(r.svc.created) != 0 {
			t.Error("practice hero lost or page made")
		}
	})

	t.Run("a failed create puts the practice hero back", func(t *testing.T) {
		r := newPracticeRig()
		r.svc.failNew = errors.New("db down")
		id := r.seed("u1", "X", map[string]HeroPick{"class": {Name: "Fury"}}, PracticeAskWaiting)
		_, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		if err == nil {
			t.Fatal("expected an error")
		}
		got, ok := r.repo.rows[id]
		if !ok || got.AskStatus != PracticeAskWaiting {
			t.Errorf("practice hero after failure: %+v ok=%v", got, ok)
		}
	})

	t.Run("a failed release is loud and keeps the new hero", func(t *testing.T) {
		r := newPracticeRig()
		r.svc.owned["u1"] = []Entity{{ID: "tess", CampaignID: "camp-1", Name: "Tess", EntityTypeID: practiceChar}}
		r.releaser.fail = errors.New("db down")
		id := r.seed("u1", "X", map[string]HeroPick{"class": {Name: "Fury"}}, PracticeAskWaiting)
		_, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		assertAppError(t, err, 500)
		if !strings.Contains(err.Error(), "Tess") {
			t.Errorf("error should name the page left over: %v", err)
		}
		if len(r.svc.created) != 1 {
			t.Error("the new hero should stand")
		}
	})

	t.Run("two yes answers at once make one hero", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "X", nil, PracticeAskWaiting)
		if _, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) }); err != nil {
			t.Fatal(err)
		}
		_, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		assertAppError(t, err, 404)
		if len(r.svc.created) != 1 {
			t.Errorf("created %d heroes", len(r.svc.created))
		}
	})
}

func TestPractice_AskAndThrowAway(t *testing.T) {
	r := newPracticeRig()
	id := r.seed("u1", "X", nil, "")
	rec, err := r.call(t, vPat, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAsk(c) })
	if err != nil || r.repo.rows[id].AskStatus != PracticeAskWaiting {
		t.Fatalf("ask: %v %+v", err, r.repo.rows[id])
	}
	if !strings.Contains(rec.Body.String(), "The Director hasn't answered yet.") || strings.Contains(rec.Body.String(), "Ask to play this one") {
		t.Errorf("a waiting page offers no second ask: %s", rec.Body.String())
	}
	// Only the maker throws it away, and it goes outright.
	_, err = r.call(t, vDirector, http.MethodDelete, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeDelete(c) })
	assertAppError(t, err, 404)
	rec, err = r.call(t, vPat, http.MethodDelete, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeDelete(c) })
	if err != nil || rec.Header().Get("HX-Redirect") != "/campaigns/camp-1/characters" {
		t.Errorf("delete: %v headers %v", err, rec.Header())
	}
	if _, ok := r.repo.rows[id]; ok {
		t.Error("still there")
	}
}

func TestPractice_NotNow(t *testing.T) {
	r := newPracticeRig()
	id := r.seed("u1", "X", nil, PracticeAskWaiting)
	_, err := r.call(t, vDirector, http.MethodPost, "/x", url.Values{"note": {"Next arc."}}, id, func(h *Handler, c *echo.Context) error { return h.PracticeDecline(c) })
	if err != nil {
		t.Fatal(err)
	}
	if h := r.repo.rows[id]; h.AskStatus != PracticeAskDeclined || h.AskNote != "Next arc." {
		t.Errorf("hero = %+v", h)
	}
	// The maker sees the answer in the Director's words.
	rec, err := r.call(t, vPat, http.MethodGet, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeShow(c) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"The Director said not now.", "Next arc."} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestPractice_ShelfAndDirectorList(t *testing.T) {
	tess := &PracticeShelfView{CampaignID: "camp-1", Max: 3, HeroName: "Tess", Rows: []PracticeShelfRow{
		{ID: "a", Name: "Tess as a Fury", Line: "A copy of Tess"}, {ID: "b", Name: "Old Marrow", Line: "Dwarf Conduit, level 1"}}}
	got := renderCastContent(t, CastView{Practice: tess})
	for _, want := range []string{"Practice", "2 of 3", "Tess as a Fury", "A copy of Tess", "Dwarf Conduit, level 1", "Start blank",
		"Start as a copy of Tess", "Only you and the Directors see these. They never join the party."} {
		if !strings.Contains(got, want) {
			t.Errorf("shelf lacks %q", want)
		}
	}
	if strings.Contains(got, "Throw one away") {
		t.Error("the refusal shows below the cap")
	}

	empty := renderCastContent(t, CastView{Practice: &PracticeShelfView{CampaignID: "camp-1", Max: 3}})
	for _, want := range []string{"0 of 3", "Start a practice hero", "Only you and the Directors see these."} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty shelf lacks %q", want)
		}
	}

	full := &PracticeShelfView{CampaignID: "camp-1", Max: 3, HeroName: "Tess", Rows: []PracticeShelfRow{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}}}
	got = renderCastContent(t, CastView{Practice: full})
	if !strings.Contains(got, "You have three practice heroes. Throw one away to start another.") || strings.Contains(got, "Start blank") {
		t.Errorf("full shelf: %s", got)
	}

	dir := &PracticeDirectorView{CampaignID: "camp-1", Rows: []PracticeDirectorRow{
		{ID: "a", Name: "Tess as a Fury", Summary: "Polder Fury, level 3", Maker: "Pat", Waiting: true, OldHero: "Tess"},
		{ID: "b", Name: "Old Marrow", Maker: "Sam"}}}
	got = renderCastContent(t, CastView{PracticeDirector: dir})
	for _, want := range []string{"Practice heroes", "made by Pat", "Pat asks to play this one.", "Say yes", "Not now", "Tess goes back to Free to pick.", "Made by Sam"} {
		if !strings.Contains(got, want) {
			t.Errorf("director list lacks %q", want)
		}
	}
	if strings.Count(got, "asks to play this one.") != 1 {
		t.Error("only the waiting hero carries the ask")
	}
	// Each control carries its own inline handler: nothing emits a script tag
	// that an HTMX swap would drop.
	if strings.Contains(got, "<script") {
		t.Error("the Directors' list emits a script tag")
	}
	if !strings.Contains(got, `onclick="(function(b,k)`) {
		t.Error("the Say yes control has no inline handler")
	}
	noOld := renderCastContent(t, CastView{PracticeDirector: &PracticeDirectorView{CampaignID: "camp-1",
		Rows: []PracticeDirectorRow{{ID: "a", Name: "X", Maker: "Pat", Waiting: true}}}})
	if strings.Contains(noOld, "goes back to Free to pick") {
		t.Error("no confirm when the player has no hero")
	}
}
