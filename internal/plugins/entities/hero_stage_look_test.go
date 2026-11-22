package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The creator's stage look is asked for with ?look=stage and nothing else:
// that value puts data-look on the mount and links the stage stylesheet; no
// parameter, or any other value, leaves the page as it was.
func TestNewHero_StageLook(t *testing.T) {
	player := tryoutViewer{user: "u1", member: true, role: campaigns.RolePlayer}
	page := func(query string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/x"+query, nil).WithContext(context.Background())
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
		c.Set("campaign_context", player.cc())
		c.Set("auth_user_id", player.user)
		if err := termsCreatorHandler(nil).NewHero(c); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("creator page for %q: %v %d", query, err, rec.Code)
		}
		return rec.Body.String()
	}
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"stage", "?look=stage", true},
		{"no parameter", "", false},
		{"another value", "?look=neon", false},
		{"not the exact value", "?look=Stage", false},
		{"empty", "?look=", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := page(tt.query)
			if got := strings.Contains(body, `data-look="stage"`); got != tt.want {
				t.Errorf("data-look on the mount = %v, want %v", got, tt.want)
			}
			if got := strings.Contains(body, "/static/plugins/entities/css/hero_stage.css"); got != tt.want {
				t.Errorf("stage stylesheet linked = %v, want %v", got, tt.want)
			}
			if !strings.Contains(body, "/static/plugins/entities/css/hero_creator.css") {
				t.Error("the creator's own stylesheet is always linked")
			}
			if !strings.Contains(body, `data-widget="hero-creator"`) {
				t.Error("the mount is missing")
			}
		})
	}
}
