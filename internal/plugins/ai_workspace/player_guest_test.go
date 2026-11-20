package ai_workspace

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Every players' AI route refuses a guest on the server, whatever the topbar
// shows, and still lets a player with an account through.
func TestPlayerRoutes_RefuseGuests(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/campaigns/camp-1/ai/me/chip"},
		{http.MethodGet, "/campaigns/camp-1/ai/me/panel"},
		{http.MethodGet, "/campaigns/camp-1/ai/me/bubble"},
		{http.MethodGet, "/campaigns/camp-1/ai/me/copy"},
		{http.MethodPost, "/campaigns/camp-1/ai/me/key"},
		{http.MethodPost, "/campaigns/camp-1/ai/me/key/disconnect"},
		{http.MethodPost, "/campaigns/camp-1/ai/me/paste"},
		{http.MethodPost, "/campaigns/camp-1/ai/me/apply"},
		{http.MethodGet, "/campaigns/camp-1/ai/me/requests/1"},
		{http.MethodPost, "/campaigns/camp-1/ai/me/requests/1/turn-down"},
	}
	for _, guest := range []bool{true, false} {
		e := echo.New()
		e.HTTPErrorHandler = func(c *echo.Context, err error) {
			code := http.StatusInternalServerError
			if ae, ok := err.(*apperror.AppError); ok {
				code = ae.Code
			}
			_ = c.NoContent(code)
		}
		h, _, w, _ := playerRig(t)
		w.allowed = true
		member := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				s := &auth.Session{UserID: "player-1"}
				if guest {
					s.GuestCampaignID = "camp-1"
				}
				auth.SetSession(c, s)
				playerCtx(c)
				return next(c)
			}
		}
		RegisterPlayerRoutes(e.Group("/campaigns/:id"), h, member)
		for _, r := range routes {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(r.method, r.path, nil))
			if guest && rec.Code != http.StatusForbidden {
				t.Errorf("guest %s %s = %d, want 403", r.method, r.path, rec.Code)
			}
			if !guest && rec.Code == http.StatusForbidden {
				t.Errorf("player %s %s = 403, want it let through", r.method, r.path)
			}
		}
	}
}
