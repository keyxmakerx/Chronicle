package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// membersSvc overrides only ListMembers; the embedded nil interface panics if
// the handler strays into any other service call.
type membersSvc struct {
	CampaignService
}

func (membersSvc) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return []CampaignMember{{UserID: "u1", DisplayName: "Ann", Email: "ann@example.com", Role: RolePlayer}}, nil
}

// The JSON member list is open to every member, so an address reaches only
// Owners and the member it belongs to.
func TestMembersJSON_EmailOnlyForOwner(t *testing.T) {
	tests := []struct {
		name      string
		role      Role
		viewerID  string
		wantEmail bool
	}{
		{"player", RolePlayer, "u2", false},
		{"scribe", RoleScribe, "u2", false},
		{"player sees own address", RolePlayer, "u1", true},
		{"owner", RoleOwner, "u2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/members", nil)
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "c1"}, MemberRole: tt.role})
			c.Set("auth_user_id", tt.viewerID)

			h := &Handler{service: membersSvc{}}
			if err := h.Members(c); err != nil {
				t.Fatalf("Members: %v", err)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, "ann@example.com"); got != tt.wantEmail {
				t.Errorf("email present = %v, want %v; body %s", got, tt.wantEmail, body)
			}
			if !strings.Contains(body, "Ann") {
				t.Errorf("display name missing: %s", body)
			}
		})
	}
}

// heroesSvc lists one shared hero, one hidden hero and the viewer's own.
type heroesSvc struct {
	CampaignService
}

func (heroesSvc) ListMembers(context.Context, string) ([]CampaignMember, error) {
	shown, hidden, own := "e-shown", "e-hidden", "e-own"
	shownName, hiddenName, ownName := "Tyne", "Kestrel", "Orrin"
	return []CampaignMember{
		{UserID: "ginko", Role: RolePlayer, CharacterEntityID: &shown, CharacterName: &shownName, CharacterShared: true},
		{UserID: "kit", Role: RolePlayer, CharacterEntityID: &hidden, CharacterName: &hiddenName},
		{UserID: "me", Role: RolePlayer, CharacterEntityID: &own, CharacterName: &ownName},
	}, nil
}

// The JSON member list is open to every member, so another player's hidden
// hero reaches only Owners and co-DMs; shared heroes and the viewer's own
// always show.
func TestMembersJSON_HiddenHeroOnlyForOwner(t *testing.T) {
	tests := []struct {
		name       string
		role       Role
		dmGranted  bool
		wantHidden bool
	}{
		{"player", RolePlayer, false, false},
		{"scribe", RoleScribe, false, false},
		{"co-DM", RolePlayer, true, true},
		{"owner", RoleOwner, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/members", nil)
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "c1"}, MemberRole: tt.role, IsDmGranted: tt.dmGranted})
			c.Set("auth_user_id", "me")

			h := &Handler{service: heroesSvc{}}
			if err := h.Members(c); err != nil {
				t.Fatalf("Members: %v", err)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, "Kestrel") || strings.Contains(body, "e-hidden"); got != tt.wantHidden {
				t.Errorf("hidden hero present = %v, want %v; body %s", got, tt.wantHidden, body)
			}
			if !strings.Contains(body, "Tyne") || !strings.Contains(body, "Orrin") {
				t.Errorf("a shared hero or the viewer's own was cleared: %s", body)
			}
		})
	}
}
