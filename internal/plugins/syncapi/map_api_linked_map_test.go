package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// stubMapSvcLinkedMap records the marker inputs the sync API hands the
// service, so a case can check how linked_map_id crossed the wire.
type stubMapSvcLinkedMap struct {
	maps.MapService
	created *maps.CreateMarkerInput
	updated *maps.UpdateMarkerInput
}

func (s *stubMapSvcLinkedMap) GetMap(context.Context, string) (*maps.Map, error) {
	return &maps.Map{ID: "map-1", CampaignID: "camp-1"}, nil
}
func (s *stubMapSvcLinkedMap) GetMarker(context.Context, string) (*maps.Marker, error) {
	return &maps.Marker{ID: "mk-1", MapID: "map-1", Visibility: "everyone"}, nil
}
func (s *stubMapSvcLinkedMap) CreateMarker(_ context.Context, in maps.CreateMarkerInput) (*maps.Marker, error) {
	s.created = &in
	return &maps.Marker{ID: "mk-new", MapID: in.MapID}, nil
}
func (s *stubMapSvcLinkedMap) UpdateMarker(_ context.Context, _ string, in maps.UpdateMarkerInput, _ bool) error {
	s.updated = &in
	return nil
}

func linkedMapContext(method, body string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(method, "/api/v1/campaigns/camp-1/maps/map-1/markers/mk-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("id", "mapID", "markerID")
	c.SetParamValues("camp-1", "map-1", "mk-1")
	c.Set(apiKeyContextKey, &APIKey{ID: 1, CampaignID: "camp-1", UserID: "owner-1", IsActive: true, Permissions: []APIKeyPermission{PermRead, PermWrite}})
	return c
}

// The sync API carries linked_map_id with the partial-update contract: a push
// that does not name it (the Foundry module's older payloads) keeps the link.
func TestMapAPIHandler_UpdateMarker_LinkedMapIsPartial(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantPresent bool
		wantNull    bool
		wantValue   string
	}{
		{"absent keeps the link", `{"name":"Renamed"}`, false, false, ""},
		{"null clears it", `{"linked_map_id":null}`, true, true, ""},
		{"a value replaces it", `{"linked_map_id":"map-2"}`, true, false, "map-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubMapSvcLinkedMap{}
			h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, &stubCampaignSvcOwnerGate{role: campaigns.RoleOwner})
			if err := h.UpdateMarker(linkedMapContext(http.MethodPut, tc.body)); err != nil {
				t.Fatalf("UpdateMarker: %v", err)
			}
			f := svc.updated.LinkedMapID
			if f.Present() != tc.wantPresent || f.IsNull() != tc.wantNull {
				t.Fatalf("present=%v null=%v, want %v/%v", f.Present(), f.IsNull(), tc.wantPresent, tc.wantNull)
			}
			if v, _ := f.Get(); v != tc.wantValue {
				t.Errorf("value %q, want %q", v, tc.wantValue)
			}
		})
	}
}

func TestMapAPIHandler_CreateMarker_PassesLinkedMap(t *testing.T) {
	svc := &stubMapSvcLinkedMap{}
	h := NewMapAPIHandler(nil, svc, &stubDrawingSvcOwnerGate{}, &stubCampaignSvcOwnerGate{role: campaigns.RoleOwner})
	if err := h.CreateMarker(linkedMapContext(http.MethodPost, `{"name":"Gate","x":1,"y":1,"linked_map_id":"map-2"}`)); err != nil {
		t.Fatalf("CreateMarker: %v", err)
	}
	if svc.created.LinkedMapID == nil || *svc.created.LinkedMapID != "map-2" {
		t.Errorf("linked_map_id did not reach the service: %v", svc.created.LinkedMapID)
	}
}
