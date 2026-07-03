// worldstate_api_test.go — the token-surface world-state seed GET
// (cordinator#34 W5 bridge). Pins the role resolution (Bearer=Owner,
// session=live role, fail-closed without a campaign service), the
// current-vs-pinned date passthrough, and the no-calendar 404.
package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newWorldStateContext builds an Echo context targeting the world-state GET
// with the given query string and API key planted in context.
func newWorldStateContext(query string, key *APIKey) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/calendar/world-state"+query, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	if key != nil {
		c.Set(apiKeyContextKey, key)
	}
	return c, rec
}

func worldStateStubSvc(t *testing.T, wantRole int, gotDate *[3]int) *stubCalendarSvc {
	t.Helper()
	return &stubCalendarSvc{
		onGet: func(_ context.Context, _ string) (*calendar.Calendar, error) {
			return &calendar.Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
		onSeed: func(_ context.Context, calID string, year, month, day, role int, _ string) (*calendar.WorldStateSeed, error) {
			if calID != "cal-1" {
				t.Errorf("seed built for calendar %q, want cal-1", calID)
			}
			if role != wantRole {
				t.Errorf("seed role = %d, want %d", role, wantRole)
			}
			if gotDate != nil {
				*gotDate = [3]int{year, month, day}
			}
			return &calendar.WorldStateSeed{}, nil
		},
	}
}

// TestGetWorldState_BearerKeyGetsOwnerSeed: a stored Bearer key resolves to
// Owner-level visibility (mirrors APIHandler.resolveRole / the WS path), so
// dm_only celestial events ship to the Foundry module.
func TestGetWorldState_BearerKeyGetsOwnerSeed(t *testing.T) {
	var date [3]int
	h := NewCalendarAPIHandler(nil, worldStateStubSvc(t, int(campaigns.RoleOwner), &date))
	c, rec := newWorldStateContext("", &APIKey{ID: 42, UserID: "u-1", CampaignID: "camp-1", IsActive: true})
	if err := h.GetWorldState(c); err != nil {
		t.Fatalf("GetWorldState: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if date != [3]int{0, 0, 0} {
		t.Errorf("date = %v, want zeros (current date)", date)
	}
}

// TestGetWorldState_DatePinPassthrough: explicit ?year&month&day reach the
// seed builder (the bridge's day-change lazy sync depends on this).
func TestGetWorldState_DatePinPassthrough(t *testing.T) {
	var date [3]int
	h := NewCalendarAPIHandler(nil, worldStateStubSvc(t, int(campaigns.RoleOwner), &date))
	c, _ := newWorldStateContext("?year=1492&month=6&day=16", &APIKey{ID: 42, UserID: "u-1", CampaignID: "camp-1", IsActive: true})
	if err := h.GetWorldState(c); err != nil {
		t.Fatalf("GetWorldState: %v", err)
	}
	if date != [3]int{1492, 6, 16} {
		t.Errorf("date = %v, want [1492 6 16]", date)
	}
}

// TestGetWorldState_SessionCallerKeepsLiveRole: a synthetic session key
// resolves through the campaign service to the member's live role — a
// Player session must NOT receive the Owner seed through the API when the
// web GET would filter dm_only events for them.
func TestGetWorldState_SessionCallerKeepsLiveRole(t *testing.T) {
	h := NewCalendarAPIHandler(nil, worldStateStubSvc(t, int(campaigns.RolePlayer), nil))
	h.SetCampaignService(&stubCampaignSvcForRole{
		getMemberFn: func(_ context.Context, _, userID string) (*campaigns.CampaignMember, error) {
			if userID != "u-player" {
				t.Errorf("GetMember userID = %q, want u-player", userID)
			}
			return &campaigns.CampaignMember{UserID: userID, Role: campaigns.RolePlayer}, nil
		},
	})
	c, _ := newWorldStateContext("", &APIKey{ID: synthKeySessionID, UserID: "u-player", CampaignID: "camp-1", IsActive: true})
	if err := h.GetWorldState(c); err != nil {
		t.Fatalf("GetWorldState: %v", err)
	}
}

// TestGetWorldState_SessionCallerFailsClosedWithoutCampaignSvc: when the
// campaign service was never wired, a session caller degrades to RoleNone
// (most-filtered seed) rather than escalating.
func TestGetWorldState_SessionCallerFailsClosedWithoutCampaignSvc(t *testing.T) {
	h := NewCalendarAPIHandler(nil, worldStateStubSvc(t, int(campaigns.RoleNone), nil))
	c, _ := newWorldStateContext("", &APIKey{ID: synthKeySessionID, UserID: "u-x", CampaignID: "camp-1", IsActive: true})
	if err := h.GetWorldState(c); err != nil {
		t.Fatalf("GetWorldState: %v", err)
	}
}

// TestGetWorldState_NoCalendar404: a campaign without a calendar returns the
// standard not-found error, matching the sibling calendar endpoints.
func TestGetWorldState_NoCalendar404(t *testing.T) {
	h := NewCalendarAPIHandler(nil, &stubCalendarSvc{})
	c, _ := newWorldStateContext("", &APIKey{ID: 42, UserID: "u-1", CampaignID: "camp-1", IsActive: true})
	if err := h.GetWorldState(c); err == nil {
		t.Fatal("GetWorldState: want not-found error for a calendar-less campaign, got nil")
	}
}
