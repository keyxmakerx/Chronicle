package calendar

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The players audience only ever narrows a read: an owner or co-DM reads
// as an anonymous Player, while a lower viewer and any write keep their own.
func TestViewerFromPlayersAudience(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		query    string
		cc       campaigns.CampaignContext
		wantRole int
		wantUser string
	}{
		{"owner read", http.MethodGet, "?audience=players", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}, int(campaigns.RolePlayer), ""},
		{"co-DM read", http.MethodGet, "?audience=players", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsDmGranted: true}, int(campaigns.RolePlayer), ""},
		{"owner without the audience", http.MethodGet, "", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}, int(campaigns.RoleOwner), "u-1"},
		{"owner write ignores it", http.MethodPut, "?audience=players", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}, int(campaigns.RoleOwner), "u-1"},
		{"player keeps their own", http.MethodGet, "?audience=players", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer}, int(campaigns.RolePlayer), "u-1"},
		{"public never widens", http.MethodGet, "?audience=players", campaigns.CampaignContext{}, 0, "u-1"},
		{"another audience", http.MethodGet, "?audience=everyone", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}, int(campaigns.RoleOwner), "u-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := echo.New().NewContext(httptest.NewRequest(tt.method, "/"+tt.query, nil), httptest.NewRecorder())
			c.Set("auth_user_id", "u-1")
			v := viewerFrom(c, &tt.cc)
			if v.Role() != tt.wantRole || v.UserID() != tt.wantUser {
				t.Errorf("viewer = (%d, %q), want (%d, %q)", v.Role(), v.UserID(), tt.wantRole, tt.wantUser)
			}
		})
	}
}
