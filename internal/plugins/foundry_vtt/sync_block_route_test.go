package foundry_vtt

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

// The Foundry sync block can be placed on any page, and its chrome answers the
// owner alone, like the status it loads: a member who requests the fragment
// directly is stopped before the handler runs.
func TestDashboardSyncBlockRouteIsOwnerOnly(t *testing.T) {
	tests := []struct {
		name      string
		owner     bool
		wantBlock bool
	}{
		{"a member who is not the owner", false, true},
		{"the owner", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			cg := e.Group("/campaigns/:id")
			requireOwner := func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c *echo.Context) error {
					if !tt.owner {
						return c.NoContent(http.StatusForbidden)
					}
					return next(c)
				}
			}
			RegisterOwnerRoutes(cg, &Handler{}, requireOwner)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/campaigns/c1/foundry-vtt/dashboard-sync-block", nil))
			if blocked := rec.Code == http.StatusForbidden; blocked != tt.wantBlock {
				t.Errorf("status %d: blocked = %v, want %v", rec.Code, blocked, tt.wantBlock)
			}
		})
	}
}
