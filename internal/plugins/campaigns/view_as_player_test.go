package campaigns

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestViewingAsPlayer(t *testing.T) {
	tests := []struct {
		name    string
		role    Role
		cookie  string
		want    bool
		granted bool
	}{
		{"owner with toggle on", RoleOwner, "camp-1", true, false},
		{"owner with toggle off", RoleOwner, "0", false, false},
		{"owner without cookie", RoleOwner, "", false, false},
		{"owner, switched on in another campaign", RoleOwner, "camp-2", false, false},
		{"co-DM, switched on in another campaign", RolePlayer, "camp-2", false, true},
		{"owner with the old one-for-everywhere cookie", RoleOwner, "1", false, false},
		{"player with a stray cookie", RolePlayer, "camp-1", false, false},
		{"scribe with a stray cookie", RoleScribe, "camp-1", false, false},
		{"co-DM with toggle on", RolePlayer, "camp-1", true, true},
		{"co-DM scribe with toggle on", RoleScribe, "camp-1", true, true},
		{"co-DM without cookie", RolePlayer, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: viewAsPlayerCookie, Value: tt.cookie})
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: tt.role, IsDmGranted: tt.granted}
			if got := ViewingAsPlayer(c, cc); got != tt.want {
				t.Errorf("ViewingAsPlayer = %v, want %v", got, tt.want)
			}
		})
	}
	if ViewingAsPlayer(echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder()), nil) {
		t.Error("nil campaign context must not be previewing")
	}
}

// Turning the preview off in one campaign leaves another campaign's preview
// alone, and the cookie names the campaign it was turned on for.
func TestPreviewCookie_ScopedToCampaign(t *testing.T) {
	cc := func(id string) *CampaignContext {
		return &CampaignContext{Campaign: &Campaign{ID: id, IsPublic: true}, MemberRole: RoleOwner, IsMember: true}
	}
	for _, name := range []string{viewAsPlayerCookie, viewAsGuestCookie} {
		on := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), on)
		writePreviewCookie(c, name, "camp-a", true, false)
		cookies := on.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Value != "camp-a" {
			t.Fatalf("%s: set cookies = %+v, want one valued camp-a", name, cookies)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(cookies[0])
		read := echo.New().NewContext(req, httptest.NewRecorder())
		wantA, wantB := ViewingAsPlayer(read, cc("camp-a")), ViewingAsPlayer(read, cc("camp-b"))
		if name == viewAsGuestCookie {
			wantA, wantB = ViewingAsGuest(read, cc("camp-a")), ViewingAsGuest(read, cc("camp-b"))
		}
		if !wantA || wantB {
			t.Errorf("%s: honoured in A = %v, in B = %v; want true, false", name, wantA, wantB)
		}

		off := httptest.NewRecorder()
		writePreviewCookie(echo.New().NewContext(req, off), name, "camp-b", false, false)
		if got := off.Result().Cookies(); len(got) != 0 {
			t.Errorf("%s: turning it off in B touched A's cookie: %+v", name, got)
		}
	}
}
