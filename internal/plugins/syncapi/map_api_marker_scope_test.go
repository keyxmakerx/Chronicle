package syncapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// stubMapSvcMarkerScope serves one map in the key's campaign and one marker
// whose map is whatever the case says. Writes record that they ran, so a
// case can prove the handler stopped before reaching the service.
type stubMapSvcMarkerScope struct {
	maps.MapService
	m      *maps.Map
	marker *maps.Marker
	wrote  bool
	// shadowed is what IsMarkerShadowed answers; shadowRole records the role asked about.
	shadowed   bool
	shadowRole int
}

func (s *stubMapSvcMarkerScope) GetMap(context.Context, string) (*maps.Map, error) { return s.m, nil }
func (s *stubMapSvcMarkerScope) GetMarker(context.Context, string) (*maps.Marker, error) {
	return s.marker, nil
}
func (s *stubMapSvcMarkerScope) IsMarkerShadowed(_ context.Context, _ *maps.Marker, role int) (bool, error) {
	s.shadowRole = role
	return s.shadowed, nil
}
func (s *stubMapSvcMarkerScope) UpdateMarker(context.Context, string, maps.UpdateMarkerInput, bool) error {
	s.wrote = true
	return nil
}
func (s *stubMapSvcMarkerScope) DeleteMarker(context.Context, string, *time.Time, bool, string, int) error {
	s.wrote = true
	return nil
}

// TestMapAPIHandler_MarkerRoutes_RequireMarkerOnURLMap pins that the marker
// read, update and delete routes refuse a marker that is not on the map in
// the URL, so a key can't reach another map's (or campaign's) marker by id.
func TestMapAPIHandler_MarkerRoutes_RequireMarkerOnURLMap(t *testing.T) {
	routes := []struct {
		name   string
		method string
		call   func(h *MapAPIHandler, c echo.Context) error
	}{
		{"GetMarker", http.MethodGet, func(h *MapAPIHandler, c echo.Context) error { return h.GetMarker(c) }},
		{"UpdateMarker", http.MethodPut, func(h *MapAPIHandler, c echo.Context) error { return h.UpdateMarker(c) }},
		{"DeleteMarker", http.MethodDelete, func(h *MapAPIHandler, c echo.Context) error { return h.DeleteMarker(c) }},
	}
	cases := []struct {
		name        string
		markerMapID string
		wantErr     bool
	}{
		{"marker on the URL map", "map-1", false},
		{"marker on another map", "map-other", true},
	}

	for _, r := range routes {
		for _, tc := range cases {
			t.Run(r.name+"/"+tc.name, func(t *testing.T) {
				svc := &stubMapSvcMarkerScope{
					m:      &maps.Map{ID: "map-1", CampaignID: "camp-1"},
					marker: &maps.Marker{ID: "mk-1", MapID: tc.markerMapID, Visibility: "everyone"},
				}
				h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, &stubCampaignSvcOwnerGate{role: campaigns.RoleOwner})
				key := &APIKey{ID: 1, CampaignID: "camp-1", UserID: "owner-1", IsActive: true, Permissions: []APIKeyPermission{PermRead, PermWrite}}
				c, _ := newMapAPIContext(r.method, "/api/v1/campaigns/camp-1/maps/map-1/markers/mk-1", key)
				c.SetParamNames("id", "mapID", "markerID")
				c.SetParamValues("camp-1", "map-1", "mk-1")

				err := r.call(h, c)
				if !tc.wantErr {
					if err != nil {
						t.Fatalf("want success, got %v", err)
					}
					return
				}
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
					t.Fatalf("want 404 NotFound, got %v", err)
				}
				if svc.wrote {
					t.Fatal("handler reached the write despite the map mismatch")
				}
			})
		}
	}
}

// TestMapAPIHandler_GetMarker_HidesDMOnlyFromNonDM pins that a dm_only marker
// answers NotFound to a key that can't author dm_only content, matching the
// update and delete routes.
func TestMapAPIHandler_GetMarker_HidesDMOnlyFromNonDM(t *testing.T) {
	svc := &stubMapSvcMarkerScope{
		m:      &maps.Map{ID: "map-1", CampaignID: "camp-1"},
		marker: &maps.Marker{ID: "mk-1", MapID: "map-1", Visibility: "dm_only"},
	}
	camp := &stubCampaignSvcMarkerScope{stubCampaignSvcOwnerGate{role: campaigns.RolePlayer}}
	h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, camp)
	key := &APIKey{ID: 1, CampaignID: "camp-1", UserID: "player-1", IsActive: true, Permissions: []APIKeyPermission{PermRead}}
	c, _ := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps/map-1/markers/mk-1", key)
	c.SetParamNames("id", "mapID", "markerID")
	c.SetParamValues("camp-1", "map-1", "mk-1")

	err := h.GetMarker(c)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("want 404 NotFound for a player key, got %v", err)
	}
}

// stubCampaignSvcMarkerScope adds the co-DM lookup canAuthorDmOnly makes for
// a non-Owner key.
type stubCampaignSvcMarkerScope struct {
	stubCampaignSvcOwnerGate
}

func (s *stubCampaignSvcMarkerScope) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return false, nil
}

// A pin the service reports as under a shadow answers NotFound, and the check
// is made with the role the key resolves to.
func TestMapAPIHandler_GetMarker_HidesShadowedPin(t *testing.T) {
	for _, shadowed := range []bool{true, false} {
		svc := &stubMapSvcMarkerScope{
			m:        &maps.Map{ID: "map-1", CampaignID: "camp-1"},
			marker:   &maps.Marker{ID: "mk-1", MapID: "map-1", Visibility: "everyone"},
			shadowed: shadowed,
		}
		camp := &stubCampaignSvcMarkerScope{stubCampaignSvcOwnerGate{role: campaigns.RolePlayer}}
		h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, camp)
		key := &APIKey{ID: 1, CampaignID: "camp-1", UserID: "player-1", IsActive: true, Permissions: []APIKeyPermission{PermRead}}
		c, _ := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps/map-1/markers/mk-1", key)
		c.SetParamNames("id", "mapID", "markerID")
		c.SetParamValues("camp-1", "map-1", "mk-1")

		err := h.GetMarker(c)
		var ae *apperror.AppError
		if shadowed {
			if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
				t.Fatalf("shadowed: want 404, got %v", err)
			}
		} else if err != nil {
			t.Fatalf("not shadowed: want success, got %v", err)
		}
		if svc.shadowRole != int(campaigns.RolePlayer) {
			t.Errorf("asked about role %d, want the key's role", svc.shadowRole)
		}
	}
}

// A marker whose visibility rules leave the caller out answers NotFound, as
// the list leaves it out; the player the rules admit reads it.
func TestMapAPIHandler_GetMarker_AppliesVisibilityRules(t *testing.T) {
	rules := `{"allowed_users":["player-ann"]}`
	tests := []struct {
		name    string
		userID  string
		role    campaigns.Role
		wantErr bool
	}{
		{"admitted player", "player-ann", campaigns.RolePlayer, false},
		{"player left out", "player-bob", campaigns.RolePlayer, true},
		{"owner bypasses rules", "owner-1", campaigns.RoleOwner, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubMapSvcMarkerScope{
				m:      &maps.Map{ID: "map-1", CampaignID: "camp-1"},
				marker: &maps.Marker{ID: "mk-1", MapID: "map-1", Visibility: "everyone", VisibilityRules: &rules},
			}
			camp := &stubCampaignSvcMarkerScope{stubCampaignSvcOwnerGate{role: tt.role}}
			h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, camp)
			key := &APIKey{ID: 1, CampaignID: "camp-1", UserID: tt.userID, IsActive: true, Permissions: []APIKeyPermission{PermRead}}
			c, _ := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps/map-1/markers/mk-1", key)
			c.SetParamNames("id", "mapID", "markerID")
			c.SetParamValues("camp-1", "map-1", "mk-1")

			err := h.GetMarker(c)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
				t.Fatalf("want 404 NotFound, got %v", err)
			}
		})
	}
}

// stubMapSvcRulesCapture records the visibility rules an update passes on.
type stubMapSvcRulesCapture struct {
	stubMapSvcMarkerScope
	got maps.UpdateMarkerInput
}

func (s *stubMapSvcRulesCapture) UpdateMarker(_ context.Context, _ string, in maps.UpdateMarkerInput, _ bool) error {
	s.got = in
	return nil
}

// Only an Owner key may change per-player visibility rules; anyone else's are
// dropped to absent, so the Owner's stored rules survive untouched.
func TestMapAPIHandler_UpdateMarker_RulesOwnerOnly(t *testing.T) {
	tests := []struct {
		name        string
		role        campaigns.Role
		wantPresent bool
	}{
		{"owner sets rules", campaigns.RoleOwner, true},
		{"scribe's rules are dropped", campaigns.RoleScribe, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubMapSvcRulesCapture{stubMapSvcMarkerScope: stubMapSvcMarkerScope{
				m:      &maps.Map{ID: "map-1", CampaignID: "camp-1"},
				marker: &maps.Marker{ID: "mk-1", MapID: "map-1", Visibility: "everyone"},
			}}
			camp := &stubCampaignSvcMarkerScope{stubCampaignSvcOwnerGate{role: tt.role}}
			h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, camp)
			key := &APIKey{ID: 1, CampaignID: "camp-1", UserID: "u-1", IsActive: true, Permissions: []APIKeyPermission{PermRead, PermWrite}}
			c, _ := newMapAPIContext(http.MethodPut, "/api/v1/campaigns/camp-1/maps/map-1/markers/mk-1", key)
			c.Request().Body = io.NopCloser(strings.NewReader(`{"visibility_rules":"{\"allowed_users\":[\"u-1\"]}"}`))
			c.Request().Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c.SetParamNames("id", "mapID", "markerID")
			c.SetParamValues("camp-1", "map-1", "mk-1")

			if err := h.UpdateMarker(c); err != nil {
				t.Fatalf("update: %v", err)
			}
			if got := svc.got.VisibilityRules.Present(); got != tt.wantPresent {
				t.Fatalf("rules present = %v, want %v", got, tt.wantPresent)
			}
		})
	}
}
