package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// C-CAL-WORLDSTATE-WIRE — GET /api/v1/campaigns/:id/calendar/world-state.
//
// The endpoint exists because the web route of the same path lives on the
// session-authenticated plugin group, so a Bearer client (the Foundry module)
// could not reach the world-state seed at all. The security question that
// comes with it is the one every syncapi calendar read answers: does an
// ordinary read key see dm_only content? These tests pin the answer to the
// role the handler forwards, because that role is the ONLY gate — the filter
// itself lives in calendar.celestialSeeds and is tested there.

// worldStateCall records what the handler forwarded into the seed builder.
type worldStateCall struct {
	calID            string
	year, month, day int
	role             int
	userID           string
}

// newWorldStateCtx builds a request context for the world-state endpoint with
// the given API key and optional date-pin query string.
func newWorldStateCtx(query string, key *APIKey) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	target := "/api/v1/campaigns/camp-1/calendar/world-state"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	if key != nil {
		c.Set(apiKeyContextKey, key)
	}
	return c, rec
}

// worldStateSvc returns a stub whose GetCalendar resolves and whose seed
// builder records its arguments into got.
func worldStateSvc(got *worldStateCall) *stubCalendarSvc {
	return &stubCalendarSvc{
		onGet: func(context.Context, string) (*calendar.Calendar, error) {
			return &calendar.Calendar{ID: "cal-1", CampaignID: "camp-1"}, nil
		},
		onWorldState: func(_ context.Context, calID string, year, month, day, role int, userID string) (*calendar.WorldStateSeed, error) {
			*got = worldStateCall{calID: calID, year: year, month: month, day: day, role: role, userID: userID}
			return &calendar.WorldStateSeed{
				Date: calendar.WorldStateDate{Year: year, Month: month, Day: day},
				Events: []calendar.WorldStateEvent{
					{ID: 7, Type: "meteor-shower", Name: "Tears of Selune", StartTime: 22, Duration: 4, Visibility: "everyone"},
				},
			}, nil
		},
	}
}

func readKey(perms ...APIKeyPermission) *APIKey {
	return &APIKey{ID: 1, CampaignID: "camp-1", IsActive: true, Permissions: perms}
}

// TestGetWorldState_RoleGating is the dm_only gate. A sync-permission key is
// the module's own key and gets Owner visibility; a plain read/write key must
// get Player visibility, which is what makes celestialSeeds drop dm_only
// rows. If this ever forwards Owner for a read key, every dm_only meteor and
// eclipse the GM authored leaks to whoever holds that key.
func TestGetWorldState_RoleGating(t *testing.T) {
	tests := []struct {
		name     string
		perms    []APIKeyPermission
		wantRole int
	}{
		{"sync key sees dm_only", []APIKeyPermission{PermRead, PermSync}, permissions.RoleOwner},
		{"read-only key does not", []APIKeyPermission{PermRead}, permissions.RolePlayer},
		{"write key does not", []APIKeyPermission{PermRead, PermWrite}, permissions.RolePlayer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got worldStateCall
			h := NewCalendarAPIHandler(nil, worldStateSvc(&got))
			c, rec := newWorldStateCtx("", readKey(tt.perms...))
			if err := h.GetWorldState(c); err != nil {
				t.Fatalf("GetWorldState: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got.role != tt.wantRole {
				t.Errorf("forwarded role = %d, want %d", got.role, tt.wantRole)
			}
			if permissions.CanSeeDmOnly(got.role) != (tt.wantRole == permissions.RoleOwner) {
				t.Errorf("CanSeeDmOnly(%d) disagrees with the intended audience", got.role)
			}
		})
	}
}

// TestGetWorldState_NoKeyIsNotDM guards the keyless shape. Route middleware
// rejects a keyless request before the handler runs, but resolveRole's
// fallback must still be the LEAST privileged value — a handler reached by
// some future direct mount must not fail open.
func TestGetWorldState_NoKeyIsNotDM(t *testing.T) {
	var got worldStateCall
	h := NewCalendarAPIHandler(nil, worldStateSvc(&got))
	c, _ := newWorldStateCtx("", nil)
	if err := h.GetWorldState(c); err != nil {
		t.Fatalf("GetWorldState: %v", err)
	}
	if permissions.CanSeeDmOnly(got.role) {
		t.Fatalf("keyless request resolved to a DM-visible role (%d)", got.role)
	}
}

// TestGetWorldState_DatePinAndDefault pins the query contract against the web
// route's: explicit year/month/day forward verbatim, and absent or malformed
// params forward 0 so the SERVICE applies "the calendar's current date" —
// handlers stay thin, and both routes therefore default identically.
func TestGetWorldState_DatePinAndDefault(t *testing.T) {
	for _, tt := range []struct {
		name                string
		query               string
		wantY, wantM, wantD int
	}{
		{"explicit pin", "year=1492&month=4&day=15", 1492, 4, 15},
		{"no params defaults to zero", "", 0, 0, 0},
		{"garbage params degrade to zero", "year=soon&month=&day=x", 0, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got worldStateCall
			h := NewCalendarAPIHandler(nil, worldStateSvc(&got))
			c, rec := newWorldStateCtx(tt.query, readKey(PermRead))
			if err := h.GetWorldState(c); err != nil {
				t.Fatalf("GetWorldState: %v", err)
			}
			if got.year != tt.wantY || got.month != tt.wantM || got.day != tt.wantD {
				t.Errorf("forwarded date = %d-%d-%d, want %d-%d-%d",
					got.year, got.month, got.day, tt.wantY, tt.wantM, tt.wantD)
			}
			if got.calID != "cal-1" {
				t.Errorf("forwarded calendar id = %q, want cal-1", got.calID)
			}
			// API-key auth has no user session; forwarding a fabricated user
			// id would make per-user visibility filters mis-fire.
			if got.userID != "" {
				t.Errorf("forwarded userID = %q, want empty (API-key auth has no session user)", got.userID)
			}
			var body calendar.WorldStateSeed
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if len(body.Events) != 1 || body.Events[0].ID != 7 {
				t.Fatalf("seed events did not round-trip: %+v", body.Events)
			}
		})
	}
}

// TestGetWorldState_StableIDOnTheWire is the module-facing half of the
// contract: a celestial event must arrive with its stable id AND its
// visibility. The id is what lets a consumer dedupe a re-delivery instead of
// creating a duplicate note — Chronicle-Foundry-Module PR #82 named the
// missing id as the reason it could not ship celestial notes at all.
func TestGetWorldState_StableIDOnTheWire(t *testing.T) {
	var got worldStateCall
	h := NewCalendarAPIHandler(nil, worldStateSvc(&got))
	c, rec := newWorldStateCtx("", readKey(PermRead, PermSync))
	if err := h.GetWorldState(c); err != nil {
		t.Fatalf("GetWorldState: %v", err)
	}
	var raw struct {
		Events []map[string]json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(raw.Events))
	}
	for _, key := range []string{"id", "type", "name", "start_time", "duration", "visibility"} {
		if _, ok := raw.Events[0][key]; !ok {
			t.Errorf("celestial event on the wire is missing %q", key)
		}
	}
}

// TestGetWorldState_FailureSurfaces pins the two error paths: a campaign with
// no calendar is 404 (not a 500), and a seed-build failure is a 500 whose
// message does not carry the underlying DB error.
func TestGetWorldState_FailureSurfaces(t *testing.T) {
	t.Run("no calendar is 404", func(t *testing.T) {
		h := NewCalendarAPIHandler(nil, &stubCalendarSvc{})
		c, _ := newWorldStateCtx("", readKey(PermRead))
		err := h.GetWorldState(c)
		if err == nil {
			t.Fatal("want an error for a campaign with no calendar")
		}
		if code := apperror.SafeCode(err); code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", code)
		}
	})

	t.Run("seed failure is 500 without leaking detail", func(t *testing.T) {
		svc := worldStateSvc(&worldStateCall{})
		svc.onWorldState = func(context.Context, string, int, int, int, int, string) (*calendar.WorldStateSeed, error) {
			return nil, errors.New("db: table calendar_celestial_events does not exist")
		}
		h := NewCalendarAPIHandler(nil, svc)
		c, _ := newWorldStateCtx("", readKey(PermRead))
		err := h.GetWorldState(c)
		if err == nil {
			t.Fatal("want an error when the seed build fails")
		}
		if code := apperror.SafeCode(err); code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", code)
		}
		if strings.Contains(err.Error(), "calendar_celestial_events") {
			t.Fatalf("internal error text leaked to the client: %q", err.Error())
		}
	})
}
