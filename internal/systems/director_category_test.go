package systems

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newDirectorCategoryHarness serves the reference routes over a campaign
// system with one Director-only category (monsters), one open one (skills),
// and two widget-only data files, one listed in director_data.
func newDirectorCategoryHarness(t *testing.T) *echo.Echo {
	t.Helper()
	base := t.TempDir()
	dataDir := filepath.Join(base, testCampaign, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"monsters.json": `[{"slug":"goblin-sniper","name":"Goblin Sniper","source":"custom"}]`,
		"skills.json":   `[{"slug":"goblin-lore","name":"Goblin Lore","source":"custom"}]`,
		"roles.json":    `[{"slug":"artillery","name":"Artillery","source":"custom"}]`,
		"layout.json":   `[{"slug":"front","name":"Front","source":"custom"}]`,
	} {
		if err := os.WriteFile(filepath.Join(dataDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &SystemManifest{ID: "customds", Name: "Draw Steel", Categories: []CategoryDef{
		{Slug: "monsters", Name: "Monsters", Director: true},
		{Slug: "skills", Name: "Skills"},
	}, DirectorData: []string{"roles.json"}}
	sys, err := NewGenericSystem(m, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	h := NewSystemHandler()
	h.SetCampaignSystems(&CampaignSystemManager{
		baseDir:   base,
		modules:   map[string]*GenericSystem{testCampaign: sys},
		manifests: map[string]*SystemManifest{testCampaign: m},
	})

	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		_ = c.JSON(apperror.SafeCode(err), map[string]string{"error": apperror.SafeMessage(err)})
	}
	mg := e.Group("/campaigns/:id/systems/:mod", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: c.Param("id"), Name: "Test"}}
			switch c.Request().Header.Get("X-Test-Role") {
			case "owner":
				cc.MemberRole = campaigns.RoleOwner
			case "dmgrant":
				cc.MemberRole, cc.IsDmGranted = campaigns.RolePlayer, true
			case "scribe":
				cc.MemberRole = campaigns.RoleScribe
			case "player":
				cc.MemberRole = campaigns.RolePlayer
			}
			c.Set("campaign_context", cc)
			return next(c)
		}
	})
	mg.GET("/search", h.SearchAPI)
	mg.GET("/data/:file", h.SystemDataAPI)
	mg.GET("/:cat", h.CategoryList)
	mg.GET("/:cat/:item", h.ItemDetail)
	mg.GET("/:cat/:item/tooltip", h.TooltipAPI)
	return e
}

func directorGet(e *echo.Echo, role, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+testCampaign+"/systems/customds"+path, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Test-Role", role)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestDirectorCategory_HiddenFromPlayers: a Director-only category is
// unreadable below Director on every route, an open one stays readable, and
// Directors (the owner or a co-Director) read both.
func TestDirectorCategory_HiddenFromPlayers(t *testing.T) {
	e := newDirectorCategoryHarness(t)
	paths := []struct{ path, open string }{
		{"/data/monsters.json", "/data/skills.json"},
		{"/data/roles.json", "/data/layout.json"},
		{"/monsters", "/skills"},
		{"/monsters/goblin-sniper", "/skills/goblin-lore"},
		{"/monsters/goblin-sniper/tooltip", "/skills/goblin-lore/tooltip"},
	}
	for _, role := range []string{"player", "scribe", "nobody"} {
		for _, p := range paths {
			if rec := directorGet(e, role, p.path); rec.Code != http.StatusNotFound {
				t.Errorf("%s GET %s = %d, want 404", role, p.path, rec.Code)
			}
			if rec := directorGet(e, role, p.open); rec.Code != http.StatusOK {
				t.Errorf("%s GET %s = %d, want 200", role, p.open, rec.Code)
			}
		}
	}
	for _, role := range []string{"owner", "dmgrant"} {
		for _, p := range paths {
			for _, path := range []string{p.path, p.open} {
				if rec := directorGet(e, role, path); rec.Code != http.StatusOK {
					t.Errorf("%s GET %s = %d, want 200", role, path, rec.Code)
				}
			}
		}
	}
}

// TestDirectorCategory_SearchLeavesItOut: search hits from a Director-only
// category reach Directors only.
func TestDirectorCategory_SearchLeavesItOut(t *testing.T) {
	e := newDirectorCategoryHarness(t)
	for role, wantSniper := range map[string]bool{"player": false, "scribe": false, "owner": true, "dmgrant": true} {
		body := directorGet(e, role, "/search?q=goblin").Body.String()
		if got := strings.Contains(body, "Goblin Sniper"); got != wantSniper {
			t.Errorf("%s search shows the monster = %v, want %v: %s", role, got, wantSniper, body)
		}
		if !strings.Contains(body, "Goblin Lore") {
			t.Errorf("%s search lost the open category: %s", role, body)
		}
	}
}

// TestSystemData_NeverSharedCache: the raw data answer depends on who asks,
// so no shared cache may keep it.
func TestSystemData_NeverSharedCache(t *testing.T) {
	e := newDirectorCategoryHarness(t)
	for _, path := range []string{"/data/skills.json", "/skills/goblin-lore/tooltip"} {
		cc := directorGet(e, "player", path).Header().Get("Cache-Control")
		if !strings.HasPrefix(cc, "private") {
			t.Errorf("GET %s Cache-Control = %q, want private", path, cc)
		}
	}
}
