package systems

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestChoiceContext pins who a sheet's pick list is read for. A Director with
// "view as player" on gets the list a player gets, rules applied, so entries
// marked Not here are gone from the sheet's picker in that view too; the
// cookie changes nothing for anyone else.
func TestChoiceContext(t *testing.T) {
	tests := []struct {
		name         string
		role         campaigns.Role
		cookie       bool
		query        string
		wantDirector bool
		wantUnlimit  bool
		wantFor      string
	}{
		{"director", campaigns.RoleOwner, false, "", true, false, "u1"},
		{"director viewing as player", campaigns.RoleOwner, true, "", false, false, "u1"},
		{"director previewing one player", campaigns.RoleOwner, false, "?as=p2", false, false, "p2"},
		{"scribe", campaigns.RoleScribe, false, "", false, true, "u1"},
		{"player with a stray cookie", campaigns.RolePlayer, true, "", false, false, "u1"},
		{"player asking as another", campaigns.RolePlayer, false, "?as=p2", false, false, "u1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/character-choices/ancestry"+tc.query, nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "c1"})
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.Set("auth_user_id", "u1")
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role}
			ctx := choiceContext(c, cc)
			if got := ChoiceViewerIsDirector(ctx); got != tc.wantDirector {
				t.Errorf("director = %v, want %v", got, tc.wantDirector)
			}
			if got := choiceUnlimited(ctx); got != tc.wantUnlimit {
				t.Errorf("unlimited = %v, want %v", got, tc.wantUnlimit)
			}
			if got := ChoiceFor(ctx); got != tc.wantFor {
				t.Errorf("for = %q, want %q", got, tc.wantFor)
			}
		})
	}
}
