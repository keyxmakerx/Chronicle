package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
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

// The stage look carries the border library on the mount and links the
// borders stylesheet; the classic page carries neither.
func TestNewHero_StageBorders(t *testing.T) {
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
	stage, classic := page("?look=stage"), page("")
	if !strings.Contains(stage, "/static/plugins/entities/css/hero_borders.css") || !strings.Contains(stage, "data-borders=") {
		t.Error("the stage look must link hero_borders.css and carry data-borders")
	}
	if !strings.Contains(stage, "/static/plugins/entities/img/borders/fire-mask.webp?v=") {
		t.Error("the library's pictures must carry the content hash")
	}
	if strings.Contains(classic, "hero_borders.css") || strings.Contains(classic, "data-borders") {
		t.Error("the classic page must not carry borders")
	}
}

// Every picture the border library names is in the embedded static files.
func TestCardBorderPicturesEmbedded(t *testing.T) {
	for _, f := range cardborders.Files() {
		if _, err := StaticAssetsFS.Open("static/img/borders/" + f); err != nil {
			t.Errorf("%s is named by the library but not embedded: %v", f, err)
		}
	}
	for _, f := range []string{"static/css/hero_borders.css", "static/js/hero_borders.js"} {
		if _, err := StaticAssetsFS.Open(f); err != nil {
			t.Errorf("%s not embedded: %v", f, err)
		}
	}
}
