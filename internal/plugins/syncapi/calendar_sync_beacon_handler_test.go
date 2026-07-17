package syncapi

// calendar_sync_beacon_handler_test.go — C-SYNC-DATE-BEACON. Pins
// GetCalendarSyncBeacon (the member-read GET /campaigns/:id/calendar-sync-beacon
// endpoint the sky strip's sync chip polls): the recorded-beacon shape, and
// the never-recorded degrade (empty response, not an error — the chip
// treats that the same as "no Foundry-confirmed date").

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newCampaignMemberTestContext builds an Echo context carrying a campaign
// context the way campaigns.RequireCampaignAccess would have set it (any
// member role — this endpoint is intentionally NOT owner-gated, see
// routes.go RegisterCampaignRoutes).
func newCampaignMemberTestContext(campaignID string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/calendar-sync-beacon", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(campaignID)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: campaignID},
		MemberRole: campaigns.RolePlayer,
		IsMember:   true,
	})
	return c, rec
}

func TestGetCalendarSyncBeacon_ReturnsRecordedBeacon(t *testing.T) {
	served := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(_ context.Context, campaignID string) (*CalendarDateBeacon, error) {
			if campaignID != "camp-1" {
				t.Errorf("campaignID = %q, want camp-1", campaignID)
			}
			return &CalendarDateBeacon{CampaignID: "camp-1", Year: 2026, Month: 7, Day: 17, ServedAt: served}, nil
		},
	}
	h := NewHandler(NewSyncAPIService(repo))
	c, rec := newCampaignMemberTestContext("camp-1")

	if err := h.GetCalendarSyncBeacon(c); err != nil {
		t.Fatalf("GetCalendarSyncBeacon: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body CalendarSyncBeaconResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if body.Date != "2026-07-17" {
		t.Errorf("Date = %q, want 2026-07-17", body.Date)
	}
	if body.ServedAt == nil || !body.ServedAt.Equal(served) {
		t.Errorf("ServedAt = %v, want %v", body.ServedAt, served)
	}
}

func TestGetCalendarSyncBeacon_NoneRecorded_ReturnsEmptyNotError(t *testing.T) {
	repo := &mockSyncAPIRepo{} // getBeaconFn nil -> (nil, nil): none recorded
	h := NewHandler(NewSyncAPIService(repo))
	c, rec := newCampaignMemberTestContext("camp-1")

	if err := h.GetCalendarSyncBeacon(c); err != nil {
		t.Fatalf("GetCalendarSyncBeacon: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never-recorded is a valid state, not an error)", rec.Code)
	}

	var body CalendarSyncBeaconResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if body.Date != "" || body.ServedAt != nil {
		t.Errorf("expected an empty beacon response, got %+v", body)
	}
}

func TestGetCalendarSyncBeacon_NoCampaignContext_Forbidden(t *testing.T) {
	repo := &mockSyncAPIRepo{}
	h := NewHandler(NewSyncAPIService(repo))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/calendar-sync-beacon", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	// No campaign_context set — simulates a request that bypassed
	// RequireCampaignAccess, which should never happen in production
	// routing but the handler must still fail closed.

	err := h.GetCalendarSyncBeacon(c)
	assertAppError(t, err, http.StatusForbidden)
}
