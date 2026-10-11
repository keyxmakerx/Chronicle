// my_characters_redirect_test.go pins that the old "My Characters" address
// sends players to the Characters page, where their own characters are the
// Yours band, for full loads and boosted navigation alike.
package entities

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestMyCharacters_RedirectsToCharactersPage(t *testing.T) {
	cases := []struct {
		name  string
		boost bool
	}{
		{name: "full page load"},
		{name: "boosted sidebar navigation", boost: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/me", nil)
			if tt.boost {
				req.Header.Set("HX-Request", "true")
				req.Header.Set("HX-Boosted", "true")
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Test"},
				MemberRole: campaigns.RolePlayer,
				IsMember:   true,
			})

			if err := (&Handler{}).MyCharacters(c); err != nil {
				t.Fatalf("MyCharacters returned %v", err)
			}
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != "/campaigns/camp-1/characters" {
				t.Fatalf("Location = %q, want the Characters page", got)
			}
		})
	}
}

func TestMyCharacters_NoCampaignContext(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/me", nil), httptest.NewRecorder())
	if err := (&Handler{}).MyCharacters(c); err == nil {
		t.Fatal("want an error without a campaign context")
	}
}
