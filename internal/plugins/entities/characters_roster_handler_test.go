package entities

// characters_roster_handler_test.go pins the "Who plays whom" routes: they
// vanish when claiming is off, validate the new owner against the campaign's
// members, answer with the refreshed page body carrying a notice, and the
// finder answers with result rows shaped by who it is searching for.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// rosterSvc serves one character and records the writes the handlers make.
type rosterSvc struct {
	EntityService
	ent      Entity
	owner    *string
	ownerSet bool
	playable *bool
	onRoster []Entity
	found    []Entity
	searchF  RosterSearch
	searched bool
	claimed  string
}

func (s *rosterSvc) GetByID(_ context.Context, id string) (*Entity, error) {
	if id != s.ent.ID {
		return nil, apperror.NewNotFound("entity not found")
	}
	e := s.ent
	return &e, nil
}
func (s *rosterSvc) AssignOwner(_ context.Context, _ string, o *string) (*Entity, error) {
	s.owner, s.ownerSet = o, true
	return &s.ent, nil
}
func (s *rosterSvc) SetPlayable(_ context.Context, _, _ string, p bool) (*Entity, error) {
	s.playable = &p
	return &s.ent, nil
}
func (s *rosterSvc) ListRoster(context.Context, string, []int, int, string) ([]Entity, error) {
	return s.onRoster, nil
}
func (s *rosterSvc) SearchRoster(_ context.Context, _ string, _ []int, f RosterSearch, _ int, _ string) ([]Entity, error) {
	s.searched, s.searchF = true, f
	return s.found, nil
}
func (s *rosterSvc) ListClaimed(context.Context, string, int, string) ([]Entity, error) {
	return nil, nil
}
func (s *rosterSvc) ListByOwner(context.Context, string, string) ([]Entity, error) { return nil, nil }

type rosterMembers struct{}

func (rosterMembers) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{
		{UserID: "u1", DisplayName: "Ana", Role: campaigns.RolePlayer},
		{UserID: "u2", DisplayName: "Bo", Role: campaigns.RolePlayer},
	}, nil
}

func rosterCall(t *testing.T, svc *rosterSvc, addonOn bool, method, target string, form url.Values, call func(*Handler, echo.Context) error) (*httptest.ResponseRecorder, error) {
	t.Helper()
	h := NewHandler(svc)
	h.SetCharacterLists(&fakeCastLists{chars: []int{7}, chosen: CharacterLists{CharacterTypeIDs: []int{7}}})
	h.SetMemberLister(rosterMembers{})
	h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: addonOn}})

	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Test"},
		MemberRole: campaigns.RoleScribe,
		IsMember:   true,
	})
	c.Set("auth_user_id", "gm")
	return rec, call(h, c)
}

func TestSetCharacterOwner(t *testing.T) {
	tests := []struct {
		name       string
		addonOn    bool
		form       url.Values
		wantCode   int
		wantOwner  string // "" = unclaim
		wantWrite  bool
		wantNotice string
	}{
		{"addon off is a 404", false, url.Values{"entity_id": {"e1"}, "owner_user_id": {"u1"}}, 404, "", false, ""},
		{"non-member owner is rejected", true, url.Values{"entity_id": {"e1"}, "owner_user_id": {"stranger"}}, 400, "", false, ""},
		{"other campaign's entity is a 404", true, url.Values{"entity_id": {"nope"}, "owner_user_id": {"u1"}}, 404, "", false, ""},
		{"assign", true, url.Values{"entity_id": {"e1"}, "owner_user_id": {"u1"}}, 200, "u1", true, "Tyne is now Ana&#39;s."},
		{"empty owner unclaims", true, url.Values{"entity_id": {"e1"}, "owner_user_id": {""}}, 200, "", true, "Tyne is unclaimed now, and free to pick."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &rosterSvc{ent: Entity{ID: "e1", CampaignID: "camp-1", Name: "Tyne"}}
			rec, err := rosterCall(t, svc, tc.addonOn, http.MethodPost, "/campaigns/camp-1/characters/owner", tc.form,
				func(h *Handler, c echo.Context) error { return h.SetCharacterOwner(c) })
			if tc.wantCode != 200 {
				assertAppError(t, err, tc.wantCode)
				if svc.ownerSet {
					t.Error("AssignOwner must not run")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc.ownerSet != tc.wantWrite {
				t.Fatalf("AssignOwner called = %v", svc.ownerSet)
			}
			if tc.wantOwner == "" && svc.owner != nil || tc.wantOwner != "" && (svc.owner == nil || *svc.owner != tc.wantOwner) {
				t.Errorf("owner = %v, want %q", svc.owner, tc.wantOwner)
			}
			if !strings.Contains(rec.Body.String(), tc.wantNotice) {
				t.Errorf("body lacks notice %q", tc.wantNotice)
			}
		})
	}
}

func TestSetCharacterPlayable(t *testing.T) {
	tests := []struct {
		name       string
		addonOn    bool
		playable   string
		wantCode   int
		wantVal    bool
		wantNotice string
	}{
		{"addon off is a 404", false, "1", 404, false, ""},
		{"junk value is rejected", true, "yes", 400, false, ""},
		{"mark", true, "1", 200, true, "Tyne is free to pick now."},
		{"unmark", true, "0", 200, false, "Tyne is off Free to pick."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &rosterSvc{ent: Entity{ID: "e1", CampaignID: "camp-1", Name: "Tyne"}}
			rec, err := rosterCall(t, svc, tc.addonOn, http.MethodPost, "/campaigns/camp-1/characters/playable",
				url.Values{"entity_id": {"e1"}, "playable": {tc.playable}},
				func(h *Handler, c echo.Context) error { return h.SetCharacterPlayable(c) })
			if tc.wantCode != 200 {
				assertAppError(t, err, tc.wantCode)
				if svc.playable != nil {
					t.Error("SetPlayable must not run")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc.playable == nil || *svc.playable != tc.wantVal {
				t.Errorf("playable = %v, want %v", svc.playable, tc.wantVal)
			}
			if !strings.Contains(rec.Body.String(), tc.wantNotice) {
				t.Errorf("body lacks notice %q", tc.wantNotice)
			}
		})
	}
}

func TestSearchRosterFinder(t *testing.T) {
	free := Entity{ID: "e1", Name: "Old Hob", Playable: true, TypeName: "Character"}
	owned := Entity{ID: "e2", Name: "Tyne", TypeName: "Character", OwnerUserID: ptr("u2")}
	plain := Entity{ID: "e3", Name: "Tavern Tom", TypeName: "Villain"}

	tests := []struct {
		name        string
		addonOn     bool
		target      string
		q           string
		found       []Entity
		wantCode    int
		wantSearch  bool
		wantF       RosterSearch
		wantBody    []string
		wantNotBody []string
	}{
		{name: "addon off is a 404", addonOn: false, target: "free", q: "ty", wantCode: 404},
		{name: "a target outside the campaign is rejected", addonOn: true, target: "stranger", q: "ty", wantCode: 400},
		{name: "a missing target is rejected", addonOn: true, target: "", q: "ty", wantCode: 400},
		{
			name: "free with nothing typed searches nothing", addonOn: true, target: "free", wantCode: 200,
			wantBody: []string{"Type a name to search all your characters."},
		},
		{
			name: "free with a name skips listed characters", addonOn: true, target: "free", q: " tom ", found: []Entity{plain},
			wantCode: 200, wantSearch: true, wantF: RosterSearch{Query: "tom", SkipListed: true, Limit: 8},
			wantBody:    []string{"Tavern Tom", `<span class="ag-tag">Villain</span>`, "/characters/playable", `&#34;playable&#34;:&#34;1&#34;`, ">Add</button>"},
			wantNotBody: []string{"Give"},
		},
		{
			name: "give with nothing typed offers only free characters", addonOn: true, target: "u1", found: []Entity{free},
			wantCode: 200, wantSearch: true, wantF: RosterSearch{OnlyFree: true, SkipOwnedBy: "u1", Limit: 8},
			wantBody: []string{"Old Hob", "Free to pick", "/characters/owner", `&#34;owner_user_id&#34;:&#34;u1&#34;`, ">Give</button>"},
		},
		{
			name: "give with a name searches everything the player does not own", addonOn: true, target: "u1", q: "ty", found: []Entity{free, owned},
			wantCode: 200, wantSearch: true, wantF: RosterSearch{Query: "ty", SkipOwnedBy: "u1", Limit: 8},
			wantBody: []string{"Played by Bo", "Free to pick"},
		},
		{
			name: "a played-by tag for someone who left says so", addonOn: true, target: "u1", q: "x",
			found:    []Entity{{ID: "e4", Name: "Ghost", OwnerUserID: ptr("gone")}},
			wantCode: 200, wantSearch: true, wantF: RosterSearch{Query: "x", SkipOwnedBy: "u1", Limit: 8},
			wantBody: []string{"Played by someone who left"},
		},
		{
			name: "no match says so", addonOn: true, target: "u1", q: "zzz", wantCode: 200, wantSearch: true,
			wantF:    RosterSearch{Query: "zzz", SkipOwnedBy: "u1", Limit: 8},
			wantBody: []string{"No character matches."},
		},
		{
			name: "give with nothing free and nothing typed shows the prompt", addonOn: true, target: "u1", wantCode: 200, wantSearch: true,
			wantF:    RosterSearch{OnlyFree: true, SkipOwnedBy: "u1", Limit: 8},
			wantBody: []string{"Type a name to search all your characters."},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &rosterSvc{found: tc.found}
			target := "/campaigns/camp-1/characters/players/search?q=" + url.QueryEscape(tc.q) + "&for=" + tc.target
			rec, err := rosterCall(t, svc, tc.addonOn, http.MethodGet, target, nil,
				func(h *Handler, c echo.Context) error { return h.SearchRosterFinder(c) })
			if tc.wantCode != 200 {
				assertAppError(t, err, tc.wantCode)
				if svc.searched {
					t.Error("the search must not run")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc.searched != tc.wantSearch {
				t.Fatalf("searched = %v, want %v", svc.searched, tc.wantSearch)
			}
			if tc.wantSearch && svc.searchF != tc.wantF {
				t.Errorf("filter = %+v, want %+v", svc.searchF, tc.wantF)
			}
			body := rec.Body.String()
			for _, want := range tc.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("body lacks %q:\n%s", want, body)
				}
			}
			for _, bad := range tc.wantNotBody {
				if strings.Contains(body, bad) {
					t.Errorf("body has %q", bad)
				}
			}
		})
	}
}

// The finder is staff-only and signed-in-only: an anonymous request, even to a
// public campaign, goes to the sign-in page and never reaches the search.
func TestSearchRosterFinder_NeedsSignIn(t *testing.T) {
	svc := &rosterSvc{}
	e := echo.New()
	h := NewHandler(svc)
	RegisterRoutes(e, h, guardCampaignSvc{public: true}, guardAuthSvc{})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/characters/players/search?q=a&for=free", nil))
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") || svc.searched {
		t.Errorf("anonymous finder search: code=%d location=%q searched=%v, want a sign-in redirect", rec.Code, rec.Header().Get("Location"), svc.searched)
	}
}

func ptr[T any](v T) *T { return &v }

// TestFillCastRoster pins who gets which roster data: staff the full roster,
// a character-less member only the playable unclaimed ones, a member who
// already has a character neither.
func TestFillCastRoster(t *testing.T) {
	u := "u1"
	onRoster := []Entity{
		{ID: "a", Name: "Free", Playable: true},
		{ID: "b", Name: "Taken", Playable: true, OwnerUserID: &u},
	}
	tests := []struct {
		name       string
		role       campaigns.Role
		yours      []CastMember
		wantRoster bool
		wantPick   []string
	}{
		{"scribe sees the roster but no pick box", campaigns.RoleScribe, nil, true, nil},
		{"player without a character may pick", campaigns.RolePlayer, nil, false, []string{"a"}},
		{"player with a character gets nothing", campaigns.RolePlayer, []CastMember{{}}, false, nil},
		{"scribe with a character still gets the roster", campaigns.RoleScribe, []CastMember{{}}, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&rosterSvc{onRoster: onRoster})
			h.SetMemberLister(rosterMembers{})
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tc.role, IsMember: true}
			view := CastView{Yours: tc.yours}
			h.fillCastRoster(context.Background(), cc, "me", []int{7}, &view)
			if (view.Roster != nil) != tc.wantRoster {
				t.Errorf("Roster set = %v, want %v", view.Roster != nil, tc.wantRoster)
			}
			var got []string
			for _, e := range view.Pickable {
				got = append(got, e.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantPick, ",") {
				t.Errorf("Pickable = %v, want %v", got, tc.wantPick)
			}
		})
	}
}

func (s *rosterSvc) GetEntityTypes(context.Context, string) ([]EntityType, error) { return nil, nil }

func (s *rosterSvc) CheckEntityAccess(context.Context, string, int, string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: true}, nil
}
func (s *rosterSvc) ClaimEntity(_ context.Context, id, _ string) (*Entity, error) {
	s.claimed = id
	return &s.ent, nil
}

// TestClaimFromPickBox pins that the pick box's claim answers with the page
// body and a notice (not a redirect to the entity), and that a plain claim
// from the entity page still redirects there.
func TestClaimFromPickBox(t *testing.T) {
	tests := []struct {
		name         string
		htmx         bool
		from         string
		wantCode     int
		wantBody     string
		wantLocation string
		wantHXRedir  string
	}{
		{"pick box with script returns the page body", true, "characters", 200, "Tyne is yours now.", "", ""},
		{"pick box without script goes back to the page", false, "characters", 303, "", "/campaigns/camp-1/characters", ""},
		{"the entity page's claim still opens the entity", true, "", 204, "", "", "/campaigns/camp-1/entities/e1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &rosterSvc{ent: Entity{ID: "e1", CampaignID: "camp-1", Name: "Tyne"}}
			h := NewHandler(svc)
			h.SetCharacterLists(&fakeCastLists{chars: []int{7}, chosen: CharacterLists{CharacterTypeIDs: []int{7}}})
			h.SetMemberLister(rosterMembers{})
			h.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: true}})
			form := url.Values{}
			if tc.from != "" {
				form.Set("from", tc.from)
			}
			req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/entities/e1/claim", strings.NewReader(form.Encode()))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("camp-1", "e1")
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: campaigns.RolePlayer, IsMember: true,
			})
			c.Set("auth_user_id", "u1")
			if err := h.ClaimEntity(c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if svc.claimed != "e1" {
				t.Errorf("claimed = %q, want e1", svc.claimed)
			}
			if rec.Code != tc.wantCode {
				t.Errorf("code = %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body lacks %q", tc.wantBody)
			}
			if got := rec.Header().Get("Location"); got != tc.wantLocation {
				t.Errorf("Location = %q, want %q", got, tc.wantLocation)
			}
			if got := rec.Header().Get("HX-Redirect"); got != tc.wantHXRedir {
				t.Errorf("HX-Redirect = %q, want %q", got, tc.wantHXRedir)
			}
		})
	}
}
