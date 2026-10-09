package entities

// characters_public_test.go pins that the Characters page is a public-capable
// view route (a public campaign's cast is visible signed out), that it asks the
// service for only what an anonymous viewer may see, and that member-only
// affordances stay hidden for them.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"
)

// castEntitySvc records how ListClaimed was called and serves one claimed PC.
type castEntitySvc struct {
	EntityService
	role   int
	userID string
	called bool
}

func (s *castEntitySvc) ListClaimed(_ context.Context, _ string, role int, userID string) ([]Entity, error) {
	s.called, s.role, s.userID = true, role, userID
	owner := "u1"
	return []Entity{{ID: "pc1", Name: "Aldric", OwnerUserID: &owner, TypeName: "Player Character", EntityTypeID: 7}}, nil
}

func TestCharactersPage_AnonymousPublicVsPrivate(t *testing.T) {
	const path = "/campaigns/camp-1/characters"

	tests := []struct {
		name      string
		public    bool
		wantLogin bool
	}{
		{"public campaign is browsable signed out", true, false},
		{"private campaign still bounces to /login", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &castEntitySvc{}
			e := echo.New()
			e.Use(emw.Recover())
			h := NewHandler(svc)
			h.SetCharacterLists(&fakeCastLists{chars: []int{7}, chosen: CharacterLists{CharacterTypeIDs: []int{7}}})
			RegisterRoutes(e, h, guardCampaignSvc{public: tt.public}, guardAuthSvc{})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if got := isLoginRedirect(rec); got != tt.wantLogin {
				t.Fatalf("anonymous GET /characters (public=%v): login-redirect=%v (code=%d), want %v", tt.public, got, rec.Code, tt.wantLogin)
			}
			if tt.wantLogin {
				if svc.called {
					t.Error("a private campaign's party must not be queried for an anonymous visitor")
				}
				return
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("code = %d, want 200", rec.Code)
			}
			if !svc.called || svc.role != 0 || svc.userID != "" {
				t.Errorf("ListClaimed called=%v role=%d userID=%q, want role 0 and empty user (visibility-filtered as the public)", svc.called, svc.role, svc.userID)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Aldric") {
				t.Error("the claimed character should be listed for a signed-out visitor")
			}
			if strings.Contains(body, "My characters") {
				t.Error("the member-only My characters link must be hidden from anonymous viewers")
			}
		})
	}
}

func TestClaimTypeID(t *testing.T) {
	yes, no := true, false
	pc := PresetCategoryPlayerCharacter

	tests := []struct {
		name  string
		types []EntityType
		want  int
	}{
		{"no types", nil, 0},
		{"no claimable type", []EntityType{{ID: 1, Slug: "location"}}, 0},
		{"explicit claimable", []EntityType{{ID: 3, Slug: "hero", Claimable: &yes}}, 3},
		{"explicitly not claimable wins over the slug heuristic", []EntityType{{ID: 4, Slug: "character", Claimable: &no}}, 0},
		{"player-character sub-type preferred over the generic one", []EntityType{
			{ID: 5, Slug: "character"},
			{ID: 6, Slug: "player-character", Claimable: &yes},
		}, 6},
		{"player-character preset category preferred", []EntityType{
			{ID: 7, Slug: "character"},
			{ID: 8, Slug: "pcs", PresetCategory: &pc, Claimable: &yes},
		}, 8},
		{"first claimable when no PC sub-type", []EntityType{
			{ID: 9, Slug: "npc"},
			{ID: 10, Slug: "dnd5e-character"},
			{ID: 11, Slug: "drawsteel-character"},
		}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimTypeID(tt.types); got != tt.want {
				t.Errorf("claimTypeID = %d, want %d", got, tt.want)
			}
		})
	}
}
