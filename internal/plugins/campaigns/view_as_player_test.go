package campaigns

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestViewingAsPlayer(t *testing.T) {
	tests := []struct {
		name    string
		role    Role
		cookie  string
		want    bool
		granted bool
	}{
		{"owner with toggle on", RoleOwner, "1", true, false},
		{"owner with toggle off", RoleOwner, "0", false, false},
		{"owner without cookie", RoleOwner, "", false, false},
		{"player with a stray cookie", RolePlayer, "1", false, false},
		{"scribe with a stray cookie", RoleScribe, "1", false, false},
		{"co-DM with toggle on", RolePlayer, "1", true, true},
		{"co-DM scribe with toggle on", RoleScribe, "1", true, true},
		{"co-DM without cookie", RolePlayer, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: viewAsPlayerCookie, Value: tt.cookie})
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			cc := &CampaignContext{MemberRole: tt.role, IsDmGranted: tt.granted}
			if got := ViewingAsPlayer(c, cc); got != tt.want {
				t.Errorf("ViewingAsPlayer = %v, want %v", got, tt.want)
			}
		})
	}
	if ViewingAsPlayer(echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder()), nil) {
		t.Error("nil campaign context must not be previewing")
	}
}
