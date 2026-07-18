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

// TestGetCalendarSyncBeacon_ExposesAppliedFields pins C-SYNC-APPLIED-BEACON:
// the response gains the applied-date half alongside the pre-existing
// served-date fields when a confirm has landed for this campaign.
func TestGetCalendarSyncBeacon_ExposesAppliedFields(t *testing.T) {
	served := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	applied := time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC)
	appliedYear, appliedMonth, appliedDay := 2026, 7, 18
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(_ context.Context, _ string) (*CalendarDateBeacon, error) {
			return &CalendarDateBeacon{
				CampaignID: "camp-1", Year: 2026, Month: 7, Day: 17, ServedAt: served,
				AppliedYear: &appliedYear, AppliedMonth: &appliedMonth, AppliedDay: &appliedDay, AppliedAt: &applied,
			}, nil
		},
	}
	h := NewHandler(NewSyncAPIService(repo))
	c, rec := newCampaignMemberTestContext("camp-1")

	if err := h.GetCalendarSyncBeacon(c); err != nil {
		t.Fatalf("GetCalendarSyncBeacon: %v", err)
	}

	var body CalendarSyncBeaconResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if body.Date != "2026-07-17" || body.ServedAt == nil || !body.ServedAt.Equal(served) {
		t.Errorf("served fields = %q/%v, want 2026-07-17/%v", body.Date, body.ServedAt, served)
	}
	if body.AppliedDate != "2026-07-18" {
		t.Errorf("AppliedDate = %q, want 2026-07-18", body.AppliedDate)
	}
	if body.AppliedAt == nil || !body.AppliedAt.Equal(applied) {
		t.Errorf("AppliedAt = %v, want %v", body.AppliedAt, applied)
	}
}

// TestGetCalendarSyncBeacon_ConfirmBeforeAnyGET_ServedFieldsOmitted pins the
// create-on-confirm case (repository.go ConfirmCalendarDateBeacon): a
// confirm can land before any served-date GET for this campaign, leaving a
// beacon row whose served fields are the 0/0 "unset" sentinel. The response
// MUST omit the served date rather than surfacing the fake "0000-00-00" —
// only the applied half is real.
func TestGetCalendarSyncBeacon_ConfirmBeforeAnyGET_ServedFieldsOmitted(t *testing.T) {
	applied := time.Date(2026, 7, 18, 9, 30, 0, 0, time.UTC)
	appliedYear, appliedMonth, appliedDay := 2026, 7, 18
	repo := &mockSyncAPIRepo{
		getBeaconFn: func(_ context.Context, _ string) (*CalendarDateBeacon, error) {
			// Year/Month/Day/ServedAt as ConfirmCalendarDateBeacon's insert
			// branch leaves them: 0/0/0 sentinel + appliedAt as a placeholder
			// last_served_at (never a real served date).
			return &CalendarDateBeacon{
				CampaignID: "camp-1", Year: 0, Month: 0, Day: 0, ServedAt: applied,
				AppliedYear: &appliedYear, AppliedMonth: &appliedMonth, AppliedDay: &appliedDay, AppliedAt: &applied,
			}, nil
		},
	}
	h := NewHandler(NewSyncAPIService(repo))
	c, rec := newCampaignMemberTestContext("camp-1")

	if err := h.GetCalendarSyncBeacon(c); err != nil {
		t.Fatalf("GetCalendarSyncBeacon: %v", err)
	}

	var body CalendarSyncBeaconResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v (raw: %q)", err, rec.Body.String())
	}
	if body.Date != "" || body.ServedAt != nil {
		t.Errorf("expected served fields omitted (never-served sentinel), got Date=%q ServedAt=%v", body.Date, body.ServedAt)
	}
	if body.AppliedDate != "2026-07-18" || body.AppliedAt == nil {
		t.Errorf("expected applied fields present, got AppliedDate=%q AppliedAt=%v", body.AppliedDate, body.AppliedAt)
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
