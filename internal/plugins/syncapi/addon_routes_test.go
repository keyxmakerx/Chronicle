package syncapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// A path that doesn't exist answers 404, never the 403 of an add-on gate on
// some other routes; the gated routes still answer the gate.
func TestAddonRoutes_UnknownPathIsNotFound(t *testing.T) {
	e := echo.New()
	pass := func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	deny := func(echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusForbidden) }
	}
	cg := e.Group("/api/v1/campaigns/:id", pass)
	cg.GET("/entities", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	addonRoutes{cg, deny}.GET("/armory/shops/:eid/room", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	for path, want := range map[string]int{
		"/api/v1/campaigns/c1/entities":            http.StatusOK,
		"/api/v1/campaigns/c1/armory/shops/s/room": http.StatusForbidden,
		"/api/v1/campaigns/c1/sessions":            http.StatusNotFound,
		"/api/v1/campaigns/c1/game-nights":         http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

// The API's add-on gates go on routes, not on a group under the campaign
// prefix, which would claim every unknown path for its gate.
func TestAPIRoutes_NoGatedGroupUnderCampaign(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), `cg.Group("",`) {
		t.Error(`routes.go gates routes with cg.Group("", …); use addonRoutes`)
	}
}
