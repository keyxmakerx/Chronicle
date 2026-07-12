// realtime_date_signal_test.go — C-REAL-CALENDAR-P2 / RC-4. Pins that
// GET /api/v1/campaigns/:id/calendar/date carries the `tracks_real_time` signal so
// the Foundry module can treat a wall-clock-authoritative calendar as read-only for
// dates (pausing its date-push). Uses the package's stubCalendarSvc; the syncSvc is
// unused by GetCurrentDate so it is passed as nil.
package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func getCurrentDateSignal(t *testing.T, cal *calendar.Calendar) map[string]any {
	t.Helper()
	svc := &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) { return cal, nil },
	}
	h := NewCalendarAPIHandler(nil, svc)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/calendar/date", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")

	if err := h.GetCurrentDate(c); err != nil {
		t.Fatalf("GetCurrentDate: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	return body
}

// TestRealTimeP2_DateSignalTrueForRealTime pins the signal is present and true for
// a real-time (reallife + tracks_real_time) calendar.
func TestRealTimeP2_DateSignalTrueForRealTime(t *testing.T) {
	zone := "America/New_York"
	cal := &calendar.Calendar{
		ID: "cal-rt", CampaignID: "camp-1", Mode: calendar.ModeRealLife,
		TracksRealTime: true, RealTimeZone: &zone,
		CurrentYear: 2024, CurrentMonth: 2, CurrentDay: 29, CurrentHour: 7,
	}
	body := getCurrentDateSignal(t, cal)
	got, ok := body["tracks_real_time"].(bool)
	if !ok {
		t.Fatalf("tracks_real_time missing or non-bool in %v", body)
	}
	if !got {
		t.Error("tracks_real_time should be true for a real-time calendar (RC-4)")
	}
}

// TestRealTimeP2_DateSignalFalseForManual pins the signal is present and false for
// a reallife-but-manual calendar (and, by the same predicate, for fantasy), so the
// module keeps normal two-way date sync.
func TestRealTimeP2_DateSignalFalseForManual(t *testing.T) {
	cal := &calendar.Calendar{
		ID: "cal-m", CampaignID: "camp-1", Mode: calendar.ModeRealLife,
		TracksRealTime: false,
		CurrentYear:    2024, CurrentMonth: 2, CurrentDay: 29,
	}
	body := getCurrentDateSignal(t, cal)
	got, ok := body["tracks_real_time"].(bool)
	if !ok {
		t.Fatalf("tracks_real_time missing or non-bool in %v", body)
	}
	if got {
		t.Error("tracks_real_time should be false for a manual calendar")
	}
}
