package syncapi

// calendar_date_beacon_handler_test.go — C-SYNC-DATE-BEACON. Pins the
// CRITICAL auth gate on GetCurrentDate's beacon write: only a real
// Bearer-authed API key (key.ID != synthKeySessionID) may beacon. This
// endpoint is dual-auth (RequireAuthOrAPIKey accepts either a session
// cookie OR a Bearer key), so a member browsing Chronicle's own calendar
// over the session path must never beacon — that's what these tests pin.
//
// The write itself is fire-and-forget (recordCalendarDateBeaconIfModule
// spawns a goroutine so the GET hot path isn't blocked by the throttle
// SELECT + conditional UPSERT), so the positive case synchronizes via a
// buffered channel with a timeout. The negative cases need no such wait:
// the auth-gate check happens synchronously BEFORE any goroutine spawns,
// so a non-blocking read immediately after the handler call is sufficient
// and keeps the test fast and deterministic.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// calendarBeaconCall captures one RecordCalendarDateBeacon invocation.
type calendarBeaconCall struct {
	campaignID         string
	year, month, day int
}

// stubSyncSvcForBeacon embeds SyncAPIService (nil); only
// RecordCalendarDateBeacon is reachable from GetCurrentDate's beacon path.
type stubSyncSvcForBeacon struct {
	SyncAPIService
	calls chan calendarBeaconCall
}

func (s *stubSyncSvcForBeacon) RecordCalendarDateBeacon(_ context.Context, campaignID string, year, month, day int) error {
	s.calls <- calendarBeaconCall{campaignID, year, month, day}
	return nil
}

// newCurrentDateTestContext builds an Echo context for GET .../calendar/date,
// optionally carrying an API key in context the way RequireAuthOrAPIKey /
// RequireAPIKey would have placed one (real or session-synthesized).
func newCurrentDateTestContext(campaignID string, key *APIKey) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+campaignID+"/calendar/date", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(campaignID)
	if key != nil {
		c.Set(apiKeyContextKey, key)
	}
	return c
}

func currentDateStubCalendarSvc() *stubCalendarSvc {
	cal := &calendar.Calendar{ID: "cal-1", CampaignID: "camp-1", CurrentYear: 2026, CurrentMonth: 7, CurrentDay: 17}
	return &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) { return cal, nil },
	}
}

func TestGetCurrentDate_RealBearerKey_RecordsBeacon(t *testing.T) {
	sync := &stubSyncSvcForBeacon{calls: make(chan calendarBeaconCall, 1)}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	c := newCurrentDateTestContext("camp-1", &APIKey{ID: 42, CampaignID: "camp-1"})
	if err := h.GetCurrentDate(c); err != nil {
		t.Fatalf("GetCurrentDate: %v", err)
	}

	select {
	case call := <-sync.calls:
		if call.campaignID != "camp-1" || call.year != 2026 || call.month != 7 || call.day != 17 {
			t.Errorf("beacon call = %+v, want {camp-1 2026 7 17}", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a beacon write for a real Bearer key (ID=42), got none")
	}
}

func TestGetCurrentDate_SessionSynthesizedKey_DoesNotBeacon(t *testing.T) {
	sync := &stubSyncSvcForBeacon{calls: make(chan calendarBeaconCall, 1)}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	// synthKeySessionID (0) is exactly what tryAuthFromSession synthesizes
	// for a member browsing over the session-cookie path on this dual-auth
	// route — see middleware.go RequireAuthOrAPIKey / tryAuthFromSession.
	c := newCurrentDateTestContext("camp-1", &APIKey{ID: synthKeySessionID, CampaignID: "camp-1"})
	if err := h.GetCurrentDate(c); err != nil {
		t.Fatalf("GetCurrentDate: %v", err)
	}

	select {
	case call := <-sync.calls:
		t.Fatalf("expected NO beacon write for a session-authed caller, got %+v", call)
	default:
		// expected: the auth gate must reject before any goroutine spawns.
	}
}

func TestGetCurrentDate_NoAPIKeyInContext_DoesNotBeacon(t *testing.T) {
	sync := &stubSyncSvcForBeacon{calls: make(chan calendarBeaconCall, 1)}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	c := newCurrentDateTestContext("camp-1", nil)
	if err := h.GetCurrentDate(c); err != nil {
		t.Fatalf("GetCurrentDate: %v", err)
	}

	select {
	case call := <-sync.calls:
		t.Fatalf("expected NO beacon write with no API key in context, got %+v", call)
	default:
		// expected: nil key must not beacon either.
	}
}
