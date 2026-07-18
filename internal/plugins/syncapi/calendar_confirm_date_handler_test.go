package syncapi

// calendar_confirm_date_handler_test.go — C-SYNC-APPLIED-BEACON. Pins
// ConfirmDate (POST /calendar/date/confirm), the "applied" half of the
// #548 served-date beacon: same dual-auth route shape as GetCurrentDate
// (RequireAuthOrAPIKey), so a session-authed caller must be rejected —
// but unlike GetCurrentDate's fire-and-forget beacon write, this
// endpoint's entire purpose IS the write, so the rejection is a
// synchronous 403 rather than a silent no-op with a 200 elsewhere.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// confirmCall captures one ConfirmCalendarDate invocation.
type confirmCall struct {
	campaignID       string
	year, month, day int
}

// stubSyncSvcForConfirm embeds SyncAPIService (nil); only
// ConfirmCalendarDate is reachable from ConfirmDate's handler path.
type stubSyncSvcForConfirm struct {
	SyncAPIService
	calls []confirmCall
	err   error
}

func (s *stubSyncSvcForConfirm) ConfirmCalendarDate(_ context.Context, campaignID string, year, month, day int) error {
	s.calls = append(s.calls, confirmCall{campaignID, year, month, day})
	return s.err
}

// newConfirmDateTestContext builds an Echo context for POST
// .../calendar/date/confirm, optionally carrying an API key in context the
// way RequireAuthOrAPIKey / RequireAPIKey would have placed one (real or
// session-synthesized), with the given JSON body.
func newConfirmDateTestContext(campaignID string, key *APIKey, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/"+campaignID+"/calendar/date/confirm", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(campaignID)
	if key != nil {
		c.Set(apiKeyContextKey, key)
	}
	return c, rec
}

func TestConfirmDate_RealBearerKey_RecordsConfirmationAnd204s(t *testing.T) {
	sync := &stubSyncSvcForConfirm{}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	c, rec := newConfirmDateTestContext("camp-1", &APIKey{ID: 42, CampaignID: "camp-1"}, `{"year":2026,"month":7,"day":18}`)
	if err := h.ConfirmDate(c); err != nil {
		t.Fatalf("ConfirmDate: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(sync.calls) != 1 {
		t.Fatalf("expected 1 confirm call, got %d", len(sync.calls))
	}
	got := sync.calls[0]
	if got.campaignID != "camp-1" || got.year != 2026 || got.month != 7 || got.day != 18 {
		t.Errorf("confirm call = %+v, want {camp-1 2026 7 18}", got)
	}
}

func TestConfirmDate_SessionSynthesizedKey_RejectedNotRecorded(t *testing.T) {
	sync := &stubSyncSvcForConfirm{}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	// synthKeySessionID (0) is exactly what tryAuthFromSession synthesizes
	// for a member browsing over the session-cookie path on this dual-auth
	// route — see middleware.go RequireAuthOrAPIKey / tryAuthFromSession.
	c, _ := newConfirmDateTestContext("camp-1", &APIKey{ID: synthKeySessionID, CampaignID: "camp-1"}, `{"year":2026,"month":7,"day":18}`)
	err := h.ConfirmDate(c)
	assertAppError(t, err, http.StatusForbidden)

	if len(sync.calls) != 0 {
		t.Fatalf("expected NO confirm write for a session-authed caller, got %+v", sync.calls)
	}
}

func TestConfirmDate_NoAPIKeyInContext_RejectedNotRecorded(t *testing.T) {
	sync := &stubSyncSvcForConfirm{}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	c, _ := newConfirmDateTestContext("camp-1", nil, `{"year":2026,"month":7,"day":18}`)
	err := h.ConfirmDate(c)
	assertAppError(t, err, http.StatusForbidden)

	if len(sync.calls) != 0 {
		t.Fatalf("expected NO confirm write with no API key in context, got %+v", sync.calls)
	}
}

func TestConfirmDate_InvalidBody_BadRequest(t *testing.T) {
	sync := &stubSyncSvcForConfirm{}
	h := NewCalendarAPIHandler(sync, currentDateStubCalendarSvc())

	c, _ := newConfirmDateTestContext("camp-1", &APIKey{ID: 42, CampaignID: "camp-1"}, `{not json`)
	err := h.ConfirmDate(c)
	assertAppError(t, err, http.StatusBadRequest)

	if len(sync.calls) != 0 {
		t.Fatalf("expected NO confirm write for an invalid body, got %+v", sync.calls)
	}
}
