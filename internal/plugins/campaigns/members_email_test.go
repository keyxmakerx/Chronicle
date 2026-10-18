package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
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
