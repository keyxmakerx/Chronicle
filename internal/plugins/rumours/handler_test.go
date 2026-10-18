package rumours

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakePages stands in for the entities service: it knows one page and says
// "not found" for any other, as the real adapter does for a hidden or foreign
// page.
type fakePages struct{ open string }

func (f fakePages) Open(_ context.Context, _, entityID string, _ int, _ string) error {
	if entityID != f.open {
		return apperror.NewNotFound("page not found")
	}
	return nil
}

// harness mounts the real handler on the real route gates, with the caller
// chosen per request by a header the way the campaign middleware would set it.
type harness struct {
	e    *echo.Echo
	repo *fakeRepo
}

type who struct {
	header string
	userID string
}

var (
	wDM      = who{"owner", "dm-1"}
	wCoDM    = who{"dmgrant", "jo-1"}
	wScribe  = who{"scribe", "mira-1"}
	wPlayer  = who{"player", "jo-1"}
	wVisitor = who{"none", ""}
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	repo := seeded()
	h := NewHandler(newTestService(repo), fakePages{open: tPage})
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		_ = c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	cg := e.Group("/campaigns/:id", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: c.Param("id")}}
			switch c.Request().Header.Get("X-Test-Role") {
			case "owner":
				cc.MemberRole = campaigns.RoleOwner
			case "dmgrant":
				cc.MemberRole, cc.IsDmGranted = campaigns.RolePlayer, true
			case "scribe":
				cc.MemberRole = campaigns.RoleScribe
			case "player":
				cc.MemberRole = campaigns.RolePlayer
			}
			c.Set("campaign_context", cc)
			c.Set("auth_user_id", c.Request().Header.Get("X-Test-User"))
			return next(c)
		}
	})
	player := campaigns.RequireRole(campaigns.RolePlayer)
	cg.GET("/entities/:eid/rumours", h.List, player)
	cg.POST("/entities/:eid/rumours", h.Add, scribeOrDirector())
	cg.PUT("/entities/:eid/rumours/:rid", h.Update, scribeOrDirector())
	cg.PUT("/entities/:eid/rumours/:rid/truth", h.SetTruth, directorOnly())
	cg.PUT("/entities/:eid/rumours/:rid/belief", h.SetBelief, scribeOrDirector())
	cg.DELETE("/entities/:eid/rumours/:rid", h.Remove, scribeOrDirector())
	return &harness{e: e, repo: repo}
}

func (h *harness) do(w who, method, page, rest, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/campaigns/"+tCamp+"/entities/"+page+"/rumours"+rest, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Role", w.header)
	req.Header.Set("X-Test-User", w.userID)
	rec := httptest.NewRecorder()
	h.e.ServeHTTP(rec, req)
	return rec
}

// TestRoutes_WhoMayDoWhat is the whole permission table over the real route
// gates: reads for members, writes for Scribes and the Director, and the
// verdict for the Director alone.
func TestRoutes_WhoMayDoWhat(t *testing.T) {
	const add = `{"quote":"A new thing people say.","who":"Someone","sure":2}`
	cases := []struct {
		name   string
		who    who
		method string
		rest   string
		body   string
		want   int
	}{
		{"visitor cannot read", wVisitor, "GET", "", "", http.StatusForbidden},
		{"player reads", wPlayer, "GET", "", "", http.StatusOK},
		{"scribe reads", wScribe, "GET", "", "", http.StatusOK},
		{"director reads", wDM, "GET", "", "", http.StatusOK},

		{"player cannot add", wPlayer, "POST", "", add, http.StatusForbidden},
		{"scribe adds", wScribe, "POST", "", add, http.StatusCreated},
		{"director adds", wDM, "POST", "", add, http.StatusCreated},
		{"co-director adds", wCoDM, "POST", "", add, http.StatusCreated},

		{"player cannot edit", wPlayer, "PUT", "/" + idFavour, `{"quote":"Changed."}`, http.StatusForbidden},
		{"scribe edits own", wScribe, "PUT", "/" + idTreas, `{"quote":"Changed."}`, http.StatusOK},
		{"scribe cannot edit the director's", wScribe, "PUT", "/" + idShip, `{"quote":"Changed."}`, http.StatusForbidden},
		{"director edits any", wDM, "PUT", "/" + idTreas, `{"quote":"Changed."}`, http.StatusOK},

		{"player cannot mark belief", wPlayer, "PUT", "/" + idLog + "/belief", `{"belief":"true"}`, http.StatusForbidden},
		{"scribe marks belief", wScribe, "PUT", "/" + idLog + "/belief", `{"belief":"true"}`, http.StatusOK},
		{"director marks belief", wDM, "PUT", "/" + idLog + "/belief", `{"belief":"false"}`, http.StatusOK},

		{"player cannot set truth", wPlayer, "PUT", "/" + idLog + "/truth", `{"truth":"true"}`, http.StatusForbidden},
		{"scribe cannot set truth", wScribe, "PUT", "/" + idLog + "/truth", `{"truth":"true"}`, http.StatusForbidden},
		{"director sets truth", wDM, "PUT", "/" + idLog + "/truth", `{"truth":"true"}`, http.StatusOK},
		{"co-director sets truth", wCoDM, "PUT", "/" + idLog + "/truth", `{"truth":"false"}`, http.StatusOK},

		{"player cannot remove", wPlayer, "DELETE", "/" + idFavour, "", http.StatusForbidden},
		{"scribe removes own", wScribe, "DELETE", "/" + idTreas, "", http.StatusNoContent},
		{"scribe cannot remove the director's", wScribe, "DELETE", "/" + idShip, "", http.StatusForbidden},
		{"director removes any", wDM, "DELETE", "/" + idShip, "", http.StatusNoContent},

		{"unknown rumour is not found", wDM, "PUT", "/nope", `{"quote":"x"}`, http.StatusNotFound},
		{"bad body is a 400", wScribe, "POST", "", `{"quote":`, http.StatusBadRequest},
		{"too-long quote is refused", wScribe, "POST", "", `{"quote":"` + strings.Repeat("a", QuoteMax+1) + `","who":"x","sure":2}`, http.StatusUnprocessableEntity},
		{"bad truth word is refused", wDM, "PUT", "/" + idLog + "/truth", `{"truth":"maybe"}`, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			rec := h.do(tc.who, tc.method, tPage, tc.rest, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%s %s as %s = %d, want %d: %s", tc.method, tc.rest, tc.who.header, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestRoutes_ForeignPageIs404 proves every route answers a page the caller
// may not open (or that does not exist) as not found, so a rumour id from one
// page is never reachable through another.
func TestRoutes_ForeignPageIs404(t *testing.T) {
	routes := []struct{ method, rest, body string }{
		{"GET", "", ""},
		{"POST", "", `{"quote":"A thing.","who":"x","sure":2}`},
		{"PUT", "/" + idShip, `{"quote":"A thing."}`},
		{"PUT", "/" + idShip + "/truth", `{"truth":"true"}`},
		{"PUT", "/" + idShip + "/belief", `{"belief":"true"}`},
		{"DELETE", "/" + idShip, ""},
	}
	for _, r := range routes {
		t.Run(r.method+r.rest, func(t *testing.T) {
			h := newHarness(t)
			before := len(h.repo.rows)
			rec := h.do(wDM, r.method, "other-page", r.rest, r.body)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("code = %d, want 404: %s", rec.Code, rec.Body.String())
			}
			if len(h.repo.rows) != before || h.repo.truthOf(idShip) != TruthTrue {
				t.Fatal("a refused request changed the store")
			}
		})
	}
}

// TestRoutes_NoTruthOutsideTheDirector reads every response a Scribe, a player
// and a Director get and checks the Director's verdict appears only in the
// Director's: no key, no value, no placeholder.
func TestRoutes_NoTruthOutsideTheDirector(t *testing.T) {
	cases := []struct {
		name      string
		who       who
		method    string
		rest      string
		body      string
		wantTruth bool
	}{
		{"player list", wPlayer, "GET", "", "", false},
		{"scribe list", wScribe, "GET", "", "", false},
		{"director list", wDM, "GET", "", "", true},
		{"scribe add", wScribe, "POST", "", `{"quote":"A new thing.","who":"x","sure":2}`, false},
		{"director add", wDM, "POST", "", `{"quote":"A new thing.","who":"x","sure":2}`, true},
		{"scribe belief", wScribe, "PUT", "/" + idShip + "/belief", `{"belief":"true"}`, false},
		{"scribe edit own", wScribe, "PUT", "/" + idTreas, `{"quote":"Edited."}`, false},
		{"director truth", wDM, "PUT", "/" + idTreas + "/truth", `{"truth":"true"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			rec := h.do(tc.who, tc.method, tPage, tc.rest, tc.body)
			if rec.Code >= 300 {
				t.Fatalf("code = %d: %s", rec.Code, rec.Body.String())
			}
			has := strings.Contains(rec.Body.String(), `"truth"`)
			if has != tc.wantTruth {
				t.Fatalf("truth present = %v, want %v: %s", has, tc.wantTruth, rec.Body.String())
			}
		})
	}
}

// TestRoutes_ListShape pins the JSON a block draws from, camelCase keys, and
// that a player with nothing to read is told to draw nothing.
func TestRoutes_ListShape(t *testing.T) {
	h := newHarness(t)
	rec := h.do(wScribe, "GET", tPage, "", "")
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"isDM", "canAdd", "canMark", "total", "rumours"} {
		if _, ok := got[k]; !ok {
			t.Errorf("list is missing key %q: %s", k, rec.Body.String())
		}
	}

	h.repo.rows = nil
	rec = h.do(wPlayer, "GET", tPage, "", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["canAdd"] != false || got["total"] != float64(0) {
		t.Errorf("a player with no rumours should get nothing to add and nothing to read: %s", rec.Body.String())
	}
	if list, ok := got["rumours"].([]any); !ok || len(list) != 0 {
		t.Errorf("rumours should be an empty list, not null: %s", rec.Body.String())
	}
}
