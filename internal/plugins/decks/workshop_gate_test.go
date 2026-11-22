package decks

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// What the site-wide gate answers is not the plugin's to change, so these name
// the answers it gives today: a signed-in person who is not in the campaign is
// told 403 by the campaign gate, a signed-out visitor is sent to sign in (a
// page or JSON caller 303 to /login, an HTMX fragment 204 with HX-Redirect), and a
// Director asking for another campaign's deck under their own campaign's
// address is told 404.
func TestWorkshopAnswersToPeopleOutsideTheCampaign(t *testing.T) {
	r := newWorkshopRig(t, true)
	for _, p := range r.paths() {
		for _, h := range [][]string{nil, {"HX-Request", "true"}, {"Accept", "application/json"}} {
			rec := r.get(p, "u-other", h...)
			if rec.Code != http.StatusForbidden {
				t.Errorf("a signed-in non-member at %s %v: %d, want 403", p, h, rec.Code)
			}
			if strings.Contains(rec.Body.String(), "The dragon stirs") || strings.Contains(rec.Body.String(), "dk-card") {
				t.Errorf("a signed-in non-member at %s was sent the deck", p)
			}
		}
		page := r.get(p, "")
		if page.Code != http.StatusSeeOther || !strings.HasPrefix(page.Header().Get("Location"), "/login") {
			t.Errorf("signed out at %s (page): %d to %q, want 303 to /login", p, page.Code, page.Header().Get("Location"))
		}
		frag := r.get(p, "", "HX-Request", "true")
		if frag.Code != http.StatusNoContent || !strings.HasPrefix(frag.Header().Get("HX-Redirect"), "/login") {
			t.Errorf("signed out at %s (HTMX): %d, HX-Redirect %q, want 204 to /login", p, frag.Code, frag.Header().Get("HX-Redirect"))
		}
		// Only an /api address is told 401; a JSON Accept on a campaign address is sent to sign in like a page.
		if api := r.get(p, "", "Accept", "application/json"); api.Code != http.StatusSeeOther {
			t.Errorf("signed out at %s (JSON): %d, want 303", p, api.Code)
		}
		for _, rec := range []*httptest.ResponseRecorder{page, frag} {
			if strings.Contains(rec.Body.String(), "The dragon stirs") || strings.Contains(rec.Body.String(), "dk-card") {
				t.Errorf("a signed-out visitor at %s was sent the deck", p)
			}
		}
	}
	// Camp-2's own Director, naming camp-1's deck under camp-2's address.
	for _, tail := range []string{"", "/table", "/cards", "/look", "/rules", "/players"} {
		rec := r.get("/campaigns/camp-2/decks/workshop/"+r.deck+tail, "u-other")
		if rec.Code != http.StatusNotFound {
			t.Errorf("camp-2's Director asking for camp-1's deck%s: %d, want 404", tail, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "The dragon stirs") {
			t.Errorf("camp-1's deck%s leaked under camp-2", tail)
		}
	}
	// Camp-2's Director may open camp-2's own pages, which hold nothing of camp-1.
	for _, p := range []string{"/campaigns/camp-2/decks/workshop", "/campaigns/camp-2/decks/workshop/new"} {
		rec := r.get(p, "u-other")
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "The dragon stirs") {
			t.Errorf("camp-2's own %s: %d", p, rec.Code)
		}
	}
}

// spyService answers every read the workshop makes, as a service with no
// rules would, and counts how often it was asked. A handler that reaches it
// for a player has dropped its own Director check.
type spyService struct {
	Service
	calls int
}

func (s *spyService) deck() DirectorDeck {
	return DirectorDeck{ID: "d-spy", Name: "Spy deck", Kind: KindHero, Look: defaultLook(), Settings: defaultSettings(KindHero), Version: 1,
		Cards: []DirectorCard{{ID: "c-spy", Name: "Spy card", Copies: 1}}}
}
func (s *spyService) Get(context.Context, string, string, Viewer) (DeckView, error) {
	s.calls++
	return s.deck(), nil
}
func (s *spyService) List(context.Context, string, Viewer) (DeckListView, error) {
	s.calls++
	return DirectorList{Decks: []DirectorDeck{s.deck()}}, nil
}
func (s *spyService) Roster(context.Context, string, Viewer) (Roster, error) {
	s.calls++
	return Roster{}, nil
}

// Every workshop page and fragment turns a player and a scribe away before it
// asks the service for anything; the same service answers a Director, so a
// handler that lost its own check would show here and not be hidden by the
// service's rules.
func TestEveryWorkshopHandlerChecksForADirectorBeforeAskingTheService(t *testing.T) {
	fx := newFixture(t)
	spy := &spyService{Service: fx.svc}
	e := echo.New()
	e.HTTPErrorHandler = newWorkshopRig(t, true).e.HTTPErrorHandler
	RegisterRoutes(e, NewHandler(spy), guardCampaigns{}, guardAuth{}, guardAddons{on: true})
	base := "/campaigns/camp-1/decks/workshop"
	paths := []string{base, base + "/new", base + "/d-spy", base + "/d-spy/table", base + "/d-spy/cards", base + "/d-spy/look", base + "/d-spy/rules", base + "/d-spy/players"}
	get := func(p, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	for _, p := range paths {
		spy.calls = 0
		if rec := get(p, "u-dir"); rec.Code != http.StatusOK || spy.calls == 0 {
			t.Fatalf("the control: a Director at %s got %d after %d service calls", p, rec.Code, spy.calls)
		}
		for _, user := range []string{"u-tam", "u-scribe"} {
			spy.calls = 0
			rec := get(p, user)
			if rec.Code != http.StatusNotFound || spy.calls != 0 {
				t.Errorf("%s at %s: %d after %d service calls, want 404 and none", user, p, rec.Code, spy.calls)
			}
		}
	}
}

// A player or scribe sending any of the writes the workshop's widget makes is
// refused by the plugin, and nothing in the deck changes.
func TestPlayersAndScribesCannotWriteWhatTheWorkshopWrites(t *testing.T) {
	r := newWorkshopRig(t, true)
	var card, copyID string
	for _, c := range r.fx.repo.cards {
		card = c.ID
	}
	copyID = r.fx.copiesOf(card)[0]
	state := func() string {
		return fmt.Sprintf("%+v|%+v|%+v|%d", r.fx.repo.decks, r.fx.repo.cards, r.fx.repo.copies, len(r.fx.repo.logs))
	}
	base := "/campaigns/camp-1/decks"
	d := base + "/" + r.deck
	type req struct{ method, path, body string }
	writes := []req{
		{http.MethodPost, base, `{"name":"Sneaky","kind":"event"}`},
		{http.MethodPut, d, `{"version":1,"name":"Sneaky"}`},
		{http.MethodDelete, d, ``},
		{http.MethodPut, d + "/players", `{"version":1,"players":[]}`},
		{http.MethodPost, d + "/cards", `{"name":"Sneaky"}`},
		{http.MethodPut, d + "/cards/" + card, `{"name":"Sneaky"}`},
		{http.MethodDelete, d + "/cards/" + card, ``},
		{http.MethodPost, d + "/draw", `{"count":1}`},
		{http.MethodPost, d + "/deal-card", `{"cardId":"` + card + `","to":"table"}`},
		{http.MethodPost, d + "/shuffle", `{}`},
		{http.MethodPost, d + "/gather", `{}`},
	}
	for verb := range verbs {
		writes = append(writes, req{http.MethodPost, d + "/copies/" + copyID + "/" + verb, `{"to":"table"}`})
	}
	send := func(user string, w req) *httptest.ResponseRecorder {
		var body *bytes.Reader
		if w.body != "" {
			body = bytes.NewReader([]byte(w.body))
		} else {
			body = bytes.NewReader(nil)
		}
		rq := httptest.NewRequest(w.method, w.path, body)
		rq.Header.Set("Content-Type", "application/json")
		rq.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
		rec := httptest.NewRecorder()
		r.e.ServeHTTP(rec, rq)
		return rec
	}
	for _, user := range []string{"u-tam", "u-scribe"} {
		before := state()
		for _, w := range writes {
			rec := send(user, w)
			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Errorf("%s %s %s by %s: %d %.120s, want 403 or 404", w.method, w.path, w.body, user, rec.Code, rec.Body.String())
			}
			if after := state(); after != before {
				t.Fatalf("%s %s by %s changed the deck", w.method, w.path, user)
			}
		}
	}
	// The control: a Director's draw through the same route does change it.
	before := state()
	if rec := send("u-dir", req{http.MethodPost, d + "/draw", `{"count":1}`}); rec.Code != http.StatusOK || state() == before {
		t.Errorf("the control: a Director's draw gave %d", rec.Code)
	}
}
