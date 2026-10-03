package syncapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// retiredCalendarRoutes is every pre-V5 calendar route the sync API no longer
// serves. None is called by the Foundry module.
var retiredCalendarRoutes = []string{
	`GET("/calendar/event-categories"`,
	`GET("/calendar/world-state"`,
	`PUT("/calendar/settings"`,
	`PUT("/calendar/months"`,
	`PUT("/calendar/weekdays"`,
	`PUT("/calendar/moons"`,
	`PUT("/calendar/eras"`,
	`PUT("/calendar/seasons"`,
	`PUT("/calendar/event-categories"`,
	`PUT("/calendar/weather"`,
	`PUT("/calendar/cycles"`,
	`PUT("/calendar/festivals"`,
	`POST("/calendar/advance"`,
	`POST("/calendar/advance-time"`,
	`GET("/calendar/export"`,
	`POST("/calendar/import"`,
}

// TestRetiredCalendarRoute: a retired route answers 410 with a stable code
// and a message saying what to use instead.
func TestRetiredCalendarRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPut, "/", nil), rec)
	if err := retiredCalendarRoute(retiredAdvance)(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusGone {
		t.Errorf("status = %d, want 410", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "calendar_route_retired" || !strings.Contains(body["message"], "PUT /calendar/date") {
		t.Errorf("body = %v", body)
	}
}

// TestCalendarRoutes_RetiredAreRegisteredRetired: each retired path is still
// registered (so a caller gets the 410 reason, not a bare 404) and served by
// retiredCalendarRoute, never by a handler that touches the calendar.
func TestCalendarRoutes_RetiredAreRegisteredRetired(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(src), "\n")
	for _, route := range retiredCalendarRoutes {
		found := false
		for _, l := range lines {
			if strings.Contains(l, route+",") {
				found = true
				if !strings.Contains(l, "retiredCalendarRoute(") {
					t.Errorf("%s is not served by retiredCalendarRoute: %s", route, strings.TrimSpace(l))
				}
			}
		}
		if !found {
			t.Errorf("%s is not registered", route)
		}
	}
}
