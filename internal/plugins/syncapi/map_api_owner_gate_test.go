package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// stubMapSvcOwnerGate embeds maps.MapService; only GetMap, GetMarker and
// DeleteMarker are reachable from the handlers under test. Every write path returns a
// nil error so a pre-fix test call reaches (and would perform) the
// operation, proving the role floor — not a downstream failure — is what
// blocks it.
type stubMapSvcOwnerGate struct {
	maps.MapService
	m *maps.Map
}

func (s *stubMapSvcOwnerGate) GetMap(context.Context, string) (*maps.Map, error) { return s.m, nil }
func (s *stubMapSvcOwnerGate) GetMarker(_ context.Context, id string) (*maps.Marker, error) {
	return &maps.Marker{ID: id, MapID: s.m.ID}, nil
}
func (s *stubMapSvcOwnerGate) DeleteMarker(context.Context, string, *time.Time, bool) error {
	return nil
}

// stubDrawingSvcOwnerGate embeds maps.DrawingService; every method the
// Owner-gated handlers under test can reach is overridden to succeed, so a
// missing role check would let the request through to a 200/201/204.
type stubDrawingSvcOwnerGate struct {
	maps.DrawingService
}

func (s *stubDrawingSvcOwnerGate) ListLayers(context.Context, string) ([]maps.Layer, error) {
	return nil, nil
}
func (s *stubDrawingSvcOwnerGate) CreateLayer(context.Context, maps.CreateLayerInput) (*maps.Layer, error) {
	return &maps.Layer{}, nil
}
func (s *stubDrawingSvcOwnerGate) UpdateLayer(context.Context, string, string, maps.UpdateLayerInput) error {
	return nil
}
func (s *stubDrawingSvcOwnerGate) DeleteLayer(context.Context, string, string, *time.Time) error {
	return nil
}
func (s *stubDrawingSvcOwnerGate) ListFog(context.Context, string) ([]maps.FogRegion, error) {
	return nil, nil
}
func (s *stubDrawingSvcOwnerGate) CreateFog(context.Context, maps.CreateFogInput) (*maps.FogRegion, error) {
	return &maps.FogRegion{}, nil
}
func (s *stubDrawingSvcOwnerGate) DeleteFog(context.Context, string, string) error { return nil }
func (s *stubDrawingSvcOwnerGate) ResetFog(context.Context, string) error          { return nil }
func (s *stubDrawingSvcOwnerGate) DeleteDrawing(context.Context, string, string, *time.Time) error {
	return nil
}
func (s *stubDrawingSvcOwnerGate) DeleteToken(context.Context, string, string, *time.Time) error {
	return nil
}

// stubCampaignSvcOwnerGate embeds campaigns.CampaignService; only GetMember
// is reachable from MapAPIHandler.resolveRole.
type stubCampaignSvcOwnerGate struct {
	campaigns.CampaignService
	role campaigns.Role
}

func (s *stubCampaignSvcOwnerGate) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{Role: s.role}, nil
}

// newMapAPIContext builds an Echo context for a map-scoped /api/v1 request
// carrying a real Bearer-style APIKey (not the synthetic session sentinel),
// with :id and :mapID params bound.
func newMapAPIContext(method, path string, key *APIKey) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	// A harmless empty JSON body — CreateLayer/UpdateLayer/CreateFog call
	// c.Bind; the other cases under test ignore the body entirely.
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "mapID")
	c.SetParamValues("camp-1", "map-1")
	c.Set(apiKeyContextKey, key)
	return c, rec
}

// TestMapAPIHandler_OwnerOnlyRoutes_MatchWebRoleFloor pins ADR-054's
// invariant for the six/seven map endpoints internal/plugins/maps/routes.go
// (RegisterDrawingRoutes) restricts to RoleOwner: fog of war (read, write,
// delete, reset), layer structure (create, update, delete), and
// marker/drawing/token deletion. A Scribe holds PermWrite — the API's only
// gate before this fix — so it could reach every one of these; the web twin
// requires Owner. A Scribe-scoped key must get 403 here too.
func TestMapAPIHandler_OwnerOnlyRoutes_MatchWebRoleFloor(t *testing.T) {
	m := &maps.Map{ID: "map-1", CampaignID: "camp-1"}

	newHandler := func(role campaigns.Role) *MapAPIHandler {
		return NewMapAPIHandler(
			nil,
			&stubMapSvcOwnerGate{m: m},
			&stubDrawingSvcOwnerGate{},
			&stubCampaignSvcOwnerGate{role: role},
		)
	}

	cases := []struct {
		name string
		call func(h *MapAPIHandler, c echo.Context) error
	}{
		{"ListFog", func(h *MapAPIHandler, c echo.Context) error { return h.ListFog(c) }},
		{"CreateFog", func(h *MapAPIHandler, c echo.Context) error { return h.CreateFog(c) }},
		{"DeleteFog", func(h *MapAPIHandler, c echo.Context) error { return h.DeleteFog(c) }},
		{"ResetFog", func(h *MapAPIHandler, c echo.Context) error { return h.ResetFog(c) }},
		{"CreateLayer", func(h *MapAPIHandler, c echo.Context) error { return h.CreateLayer(c) }},
		{"UpdateLayer", func(h *MapAPIHandler, c echo.Context) error { return h.UpdateLayer(c) }},
		{"DeleteLayer", func(h *MapAPIHandler, c echo.Context) error { return h.DeleteLayer(c) }},
		{"DeleteMarker", func(h *MapAPIHandler, c echo.Context) error { return h.DeleteMarker(c) }},
		{"DeleteDrawing", func(h *MapAPIHandler, c echo.Context) error { return h.DeleteDrawing(c) }},
		{"DeleteToken", func(h *MapAPIHandler, c echo.Context) error { return h.DeleteToken(c) }},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/scribe key rejected", func(t *testing.T) {
			key := &APIKey{ID: 42, CampaignID: "camp-1", UserID: "scribe-1", IsActive: true, Permissions: []APIKeyPermission{PermRead, PermWrite}}
			h := newHandler(campaigns.RoleScribe)
			c, _ := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps/map-1", key)

			err := tc.call(h, c)
			if err == nil {
				t.Fatalf("Scribe-scoped key: want error (403), got nil")
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
				t.Fatalf("Scribe-scoped key: want 403 Forbidden, got %v", err)
			}
		})

		t.Run(tc.name+"/owner key allowed", func(t *testing.T) {
			key := &APIKey{ID: 43, CampaignID: "camp-1", UserID: "owner-1", IsActive: true, Permissions: []APIKeyPermission{PermRead, PermWrite, PermSync}}
			h := newHandler(campaigns.RoleOwner)
			c, _ := newMapAPIContext(http.MethodGet, "/api/v1/campaigns/camp-1/maps/map-1", key)

			if err := tc.call(h, c); err != nil {
				t.Fatalf("Owner-scoped key: want no error, got %v", err)
			}
		})
	}
}
