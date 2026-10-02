package restore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestRunRequiresReauthBeforeRateLimit pins that a restore without a recent
// password confirmation is refused, and that refusals do not use up the
// one-per-hour restore allowance.
func TestRunRequiresReauthBeforeRateLimit(t *testing.T) {
	e := echo.New()
	denied := 0
	reauth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			denied++
			return c.NoContent(http.StatusForbidden)
		}
	}
	// A nil handler is safe: the stub refuses before any handler runs.
	RegisterRoutes(e.Group("/admin"), nil, reauth)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/restore/run", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d: got %d, want 403 from reauth (a 429 means the rate limit ran first)", i+1, rec.Code)
		}
	}
	if denied != 3 {
		t.Fatalf("reauth ran %d times, want 3", denied)
	}
}
