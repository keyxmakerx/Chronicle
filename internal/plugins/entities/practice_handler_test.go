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
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
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
func (s *heroSvc) ListClaimed(context.Context, string, int, string) ([]Entity, error) {
	return nil, nil
}
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
	lists := &fakeCastLists{chars: []int{practiceChar}, chosen: CharacterLists{CharacterTypeIDs: []int{practiceChar}}}
	h.SetPracticeService(NewPracticeService(repo, planner, PracticeDeps{Pages: svc, Types: lists, Members: rosterMembers{}, Releaser: rel}))
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

// privateByDefault is a campaign whose new pages start hidden from players.
const privateByDefault = `{"default_visibility":"dm_only"}`

func (r *practiceRig) call(t *testing.T, v viewer, method, target string, body any, pid string, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	return r.callIn(t, "", v, method, target, body, pid, fn)
}

// callIn is call in a campaign with the given settings JSON.
func (r *practiceRig) callIn(t *testing.T, settings string, v viewer, method, target string, body any, pid string, fn func(*Handler, *echo.Context) error) (*httptest.ResponseRecorder, error) {
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
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test", Settings: settings}, MemberRole: v.role, IsMember: true, IsDmGranted: v.dm}
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

// yes is the body of a Director's yes to what is on the shelf now.
func (r *practiceRig) yes(id string) url.Values {
	return url.Values{"seen": {strconv.FormatInt(r.repo.rows[id].Version(), 10)}}
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

		rec, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
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
		if _, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) }); err != nil {
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
				_, err := r.call(t, tc.v, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
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
		_, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		assertAppError(t, err, 400)
		if _, ok := r.repo.rows[id]; !ok || len(r.svc.created) != 0 {
			t.Error("practice hero lost or page made")
		}
	})

	t.Run("a failed create puts the practice hero back", func(t *testing.T) {
		r := newPracticeRig()
		r.svc.failNew = errors.New("db down")
		id := r.seed("u1", "X", map[string]HeroPick{"class": {Name: "Fury"}}, PracticeAskWaiting)
		_, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		if err == nil {
			t.Fatal("expected an error")
		}
		got, ok := r.repo.rows[id]
		if !ok || got.AskStatus != PracticeAskWaiting {
			t.Errorf("practice hero after failure: %+v ok=%v", got, ok)
		}
	})

	t.Run("a failed release keeps the new hero, is on the record, and names both pages", func(t *testing.T) {
		r := newPracticeRig()
		log := &loggedAudit{}
		r.h.SetAuditService(log)
		r.svc.owned["u1"] = []Entity{{ID: "tess", CampaignID: "camp-1", Name: "Tess", EntityTypeID: practiceChar}}
		r.releaser.fail = errors.New("db down")
		id := r.seed("u1", "Marrow", map[string]HeroPick{"class": {Name: "Fury"}}, PracticeAskWaiting)
		rec, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		if err != nil {
			t.Fatalf("a failed release should draw the page, not fail: %v", err)
		}
		if len(r.svc.created) != 1 {
			t.Error("the new hero should stand")
		}
		if len(log.entries) != 1 || log.entries[0].Action != audit.ActionEntityCreated || log.entries[0].EntityID != "new-hero" {
			t.Errorf("audit before the release step: %+v", log.entries)
		}
		body := rec.Body.String()
		want := "Marrow is now Ana&#39;s hero, but Tess couldn&#39;t be freed. Open Tess and free it."
		if rec.Code != http.StatusOK || !strings.Contains(body, want) || !strings.Contains(body, `role="alert"`) {
			t.Errorf("response %d lacks %q: %s", rec.Code, want, body)
		}
		if _, still := r.repo.rows[id]; still {
			t.Error("the practice hero should be gone: the new hero was made")
		}
	})

	t.Run("a maker's change and re-ask after the Director looked is not approved unseen", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "Marrow", map[string]HeroPick{"class": {Name: "Conduit"}}, PracticeAskWaiting)
		body := r.yes(id) // the Director's page was drawn here
		// The maker changes it (which clears the ask) and asks again.
		if _, err := r.call(t, vPat, http.MethodPut, "/x", map[string]any{"name": "Marrow", "picks": map[string]any{"class": map[string]any{"name": "Fury"}}}, id,
			func(h *Handler, c *echo.Context) error { return h.PracticeUpdate(c) }); err != nil {
			t.Fatal(err)
		}
		if _, err := r.call(t, vPat, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAsk(c) }); err != nil {
			t.Fatal(err)
		}
		_, err := r.call(t, vDirector, http.MethodPost, "/x", body, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		assertAppError(t, err, 409)
		if !strings.Contains(err.Error(), "This practice hero changed. Look again before saying yes.") {
			t.Errorf("words: %v", err)
		}
		if len(r.svc.created) != 0 {
			t.Error("a hero was made from picks the Director never saw")
		}
		if got, ok := r.repo.rows[id]; !ok || got.AskStatus != PracticeAskWaiting {
			t.Errorf("the changed practice hero should stay waiting: %+v ok=%v", got, ok)
		}
		// Looking again and saying yes then builds what is there now.
		if _, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) }); err != nil {
			t.Fatal(err)
		}
		if len(r.svc.created) != 1 || r.svc.created[0].FieldsData["class"] != "Fury" {
			t.Errorf("created = %+v", r.svc.created)
		}
	})

	t.Run("a yes that does not say what it looked at is refused", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "X", nil, PracticeAskWaiting)
		_, err := r.call(t, vDirector, http.MethodPost, "/x", nil, id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		assertAppError(t, err, 409)
		if len(r.svc.created) != 0 {
			t.Error("a hero was made")
		}
	})

	t.Run("the new hero is as open as the maker's own creation, not the Director's default", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "X", map[string]HeroPick{"class": {Name: "Conduit"}}, PracticeAskWaiting)
		// A campaign where new pages start hidden: a Director's own pages would
		// be private, a player's never are.
		_, err := r.callIn(t, privateByDefault, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
		if err != nil {
			t.Fatal(err)
		}
		if len(r.svc.created) != 1 || r.svc.created[0].IsPrivate {
			t.Errorf("the player's new hero was made private: %+v", r.svc.created)
		}
	})

	t.Run("two yes answers at once make one hero", func(t *testing.T) {
		r := newPracticeRig()
		id := r.seed("u1", "X", nil, PracticeAskWaiting)
		if _, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) }); err != nil {
			t.Fatal(err)
		}
		_, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
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
		{ID: "a", Name: "Tess as a Fury", Summary: "Polder Fury, level 3", Maker: "Pat", Waiting: true, OldHero: "Tess",
			Version: "1700000000123", Holds: "Holds picks you don't offer Pat: Fury."},
		{ID: "b", Name: "Old Marrow", Maker: "Sam"}}}
	got = renderCastContent(t, CastView{PracticeDirector: dir})
	for _, want := range []string{"Practice heroes", "made by Pat", "Pat asks to play this one.", "Say yes", "Not now", "Tess goes back to Free to pick.", "Made by Sam",
		"Holds picks you don&#39;t offer Pat: Fury.", `name="seen" value="1700000000123"`} {
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

// Making, changing, asking and the shelf are for Players exactly: a Scribe, a
// Director and a stranger are all told it is not there, while the Directors
// keep opening and answering.
func TestPractice_MakerRoutesArePlayersOnly(t *testing.T) {
	scribeDM := viewer{"co", campaigns.RoleScribe, true}
	stranger := viewer{"nobody", campaigns.RoleNone, false}
	body := map[string]any{"name": "Try", "picks": map[string]any{"class": map[string]any{"name": "Fury"}}}
	routes := []struct {
		name   string
		method string
		body   any
		fn     func(*Handler, *echo.Context) error
	}{
		{"the creator page", http.MethodGet, nil, (*Handler).PracticeNew},
		{"the plan", http.MethodGet, nil, (*Handler).PracticePlanAPI},
		{"saving a new one", http.MethodPost, body, (*Handler).PracticeCreate},
		{"changing one", http.MethodPut, body, (*Handler).PracticeUpdate},
		{"asking", http.MethodPost, nil, (*Handler).PracticeAsk},
		{"throwing one away", http.MethodDelete, nil, (*Handler).PracticeDelete},
	}
	viewers := []struct {
		name string
		v    viewer
		ok   bool
	}{
		{"a Player", vPat, true},
		{"a Scribe", vScribe, false},
		{"a Scribe who is a co-Director", scribeDM, false},
		{"the Director", vDirector, false},
		{"a stranger", stranger, false},
	}
	for _, rt := range routes {
		for _, vw := range viewers {
			t.Run(rt.name+" as "+vw.name, func(t *testing.T) {
				r := newPracticeRig()
				// The maker's own row, so a Player's change, ask and delete have something.
				id := r.seed("u1", "Mine", map[string]HeroPick{"class": {Name: "Conduit"}}, "")
				v := vw.v
				if vw.ok {
					v.id = "u1"
				}
				_, err := r.call(t, v, rt.method, "/x", rt.body, id, rt.fn)
				if vw.ok {
					if err != nil {
						t.Fatalf("a Player was refused: %v", err)
					}
					return
				}
				assertAppError(t, err, 404)
				if got, ok := r.repo.rows[id]; !ok || got.Name != "Mine" || got.AskStatus != "" {
					t.Errorf("a refused request changed the row: %+v ok=%v", got, ok)
				}
			})
		}
	}

	t.Run("the shelf is drawn for Players only", func(t *testing.T) {
		for _, vw := range viewers {
			r := newPracticeRig()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), httptest.NewRecorder())
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: vw.v.role, IsMember: vw.v.role > 0, IsDmGranted: vw.v.dm}
			c.Set("campaign_context", cc)
			c.Set("auth_user_id", vw.v.id)
			if got := r.h.practiceShelf(c, cc, nil) != nil; got != vw.ok {
				t.Errorf("%s: shelf drawn = %v, want %v", vw.name, got, vw.ok)
			}
		}
	})
}

// A maker who has since become a Scribe is no longer someone a yes can make a
// hero for.
func TestPractice_YesRefusedWhenMakerIsNoLongerAPlayer(t *testing.T) {
	r := newPracticeRig()
	r.h.SetPracticeService(NewPracticeService(r.repo, r.planner, PracticeDeps{Pages: r.svc, Types: &fakeCastLists{chars: []int{practiceChar}},
		Members: membersList{{UserID: "u1", DisplayName: "Ana", Role: campaigns.RoleScribe}}, Releaser: r.releaser}))
	id := r.seed("u1", "X", nil, PracticeAskWaiting)
	_, err := r.call(t, vDirector, http.MethodPost, "/x", r.yes(id), id, func(h *Handler, c *echo.Context) error { return h.PracticeAllow(c) })
	assertAppError(t, err, 409)
	if len(r.svc.created) != 0 {
		t.Error("a hero was made for a Scribe")
	}
}

type membersList []campaigns.CampaignMember

func (m membersList) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return m, nil
}

// The Directors' list says, on a waiting row only, which picks a yes would
// approve that the maker's rules do not offer them.
func TestPractice_DirectorListNamesHeldPicks(t *testing.T) {
	r := newPracticeRig()
	waiting := r.seed("u1", "Marrow", map[string]HeroPick{"ancestry": {Name: "Human"}, "class": {Name: "Fury"}}, PracticeAskWaiting)
	r.seed("u1", "Quiet", map[string]HeroPick{"class": {Name: "Fury"}}, "")
	ok := r.seed("u2", "Fine", map[string]HeroPick{"class": {Name: "Conduit"}}, PracticeAskWaiting)

	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), httptest.NewRecorder())
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner, IsMember: true, IsDmGranted: true}
	c.Set("campaign_context", cc)
	c.Set("auth_user_id", "dm")
	view := r.h.practiceDirectorList(c, cc)
	if view == nil {
		t.Fatal("no list")
	}
	holds := map[string]string{}
	for _, row := range view.Rows {
		holds[row.ID] = row.Holds
	}
	if holds[waiting] != "Holds picks you don't offer Ana: Fury." {
		t.Errorf("waiting row line = %q", holds[waiting])
	}
	if holds[ok] != "" {
		t.Errorf("a hero within the rules carries a line: %q", holds[ok])
	}
	for id, line := range holds {
		if id != waiting && line != "" {
			t.Errorf("row %s carries a line: %q", id, line)
		}
	}
	// One plan per waiting maker, never one per row of the table.
	if len(r.planner.users) > 2 {
		t.Errorf("planner asked %d times for 2 waiting makers", len(r.planner.users))
	}
}

func TestPracticeHoldsLine(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"Fury"}, "Holds picks you don't offer Pat: Fury."},
		{[]string{"Fury", "Troubadour"}, "Holds picks you don't offer Pat: Fury, Troubadour."},
		{[]string{"A", "B", "C"}, "Holds picks you don't offer Pat: A, B, C."},
		{[]string{"A", "B", "C", "D"}, "Holds picks you don't offer Pat: A, B, C and 1 more."},
		{[]string{"A", "B", "C", "D", "E", "F"}, "Holds picks you don't offer Pat: A, B, C and 3 more."},
	}
	for _, tc := range tests {
		if got := practiceHoldsLine("Pat", tc.names); got != tc.want {
			t.Errorf("practiceHoldsLine(%v) = %q, want %q", tc.names, got, tc.want)
		}
	}
}
