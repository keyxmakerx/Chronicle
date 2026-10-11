package entities

// characters_roster_handler_test.go pins the "Who plays whom" routes: they
// vanish when claiming is off, validate the new owner against the campaign's
// members, answer with the refreshed page body carrying a notice, and the
// search returns the compact JSON the picker reads.

import (
	"context"
	"encoding/json"
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
	ent       Entity
	owner     *string
	ownerSet  bool
	playable  *bool
	onRoster  []Entity
	searchHit []Entity
	searchQ   string
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
func (s *rosterSvc) SearchRosterCandidates(_ context.Context, _ string, _ []int, q string, _ int, _ string, _ int) ([]Entity, error) {
	s.searchQ = q
	return s.searchHit, nil
}
func (s *rosterSvc) ListClaimed(context.Context, string, int, string) ([]Entity, error) {
	return nil, nil
}
func (s *rosterSvc) ListByOwner(context.Context, string, string) ([]Entity, error) { return nil, nil }

type rosterMembers struct{}

func (rosterMembers) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{{UserID: "u1", DisplayName: "Ana"}}, nil
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
		{"empty owner unclaims", true, url.Values{"entity_id": {"e1"}, "owner_user_id": {""}}, 200, "", true, "Tyne is unclaimed now."},
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
		{"mark", true, "1", 200, true, "Tyne is on the list now."},
		{"unmark", true, "0", 200, false, "Tyne is off the list."},
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

func TestSearchRosterCandidates(t *testing.T) {
	t.Run("addon off is a 404", func(t *testing.T) {
		_, err := rosterCall(t, &rosterSvc{}, false, http.MethodGet, "/campaigns/camp-1/characters/playable/search?q=ty", nil,
			func(h *Handler, c echo.Context) error { return h.SearchRosterCandidates(c) })
		assertAppError(t, err, 404)
	})
	t.Run("returns compact JSON", func(t *testing.T) {
		svc := &rosterSvc{searchHit: []Entity{{ID: "e1", Name: "Tyne", TypeName: "Villain"}}}
		rec, err := rosterCall(t, svc, true, http.MethodGet, "/campaigns/camp-1/characters/playable/search?q=%20ty%20", nil,
			func(h *Handler, c echo.Context) error { return h.SearchRosterCandidates(c) })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got []map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
		}
		if len(got) != 1 || got[0]["id"] != "e1" || got[0]["name"] != "Tyne" || got[0]["typeName"] != "Villain" {
			t.Errorf("got %v", got)
		}
		if svc.searchQ != "ty" {
			t.Errorf("query = %q, want trimmed %q", svc.searchQ, "ty")
		}
	})
}

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
