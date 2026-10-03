package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// stubMapSvcLook embeds maps.MapService; only GetCampaignFrame is reachable
// from GetMapLook.
type stubMapSvcLook struct {
	maps.MapService
	frame string
	err   error
}

func (s *stubMapSvcLook) GetCampaignFrame(context.Context, string) (string, error) {
	return s.frame, s.err
}

func TestGetMapLook(t *testing.T) {
	tests := []struct {
		name      string
		svc       *stubMapSvcLook
		wantFrame string
	}{
		{"campaign frame is passed through", &stubMapSvcLook{frame: "arcane"}, "arcane"},
		{"lookup error falls back to the default", &stubMapSvcLook{err: errors.New("db down")}, maps.DefaultFrame},
		{"unknown stored frame falls back to the default", &stubMapSvcLook{frame: "neon"}, maps.DefaultFrame},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/c1/maps/look", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("c1")

			h := NewMapAPIHandler(nil, tt.svc, nil, nil)
			if err := h.GetMapLook(c); err != nil {
				t.Fatalf("GetMapLook: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var got mapLookResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.CampaignFrame != tt.wantFrame {
				t.Errorf("campaign_frame = %q, want %q", got.CampaignFrame, tt.wantFrame)
			}
			if len(got.Frames) != len(maps.FrameStyles) || got.Frames[0] != maps.DefaultFrame {
				t.Errorf("frames = %v, want every frame with the default first", got.Frames)
			}
			if len(got.Kinds) != len(maps.KindDefaults) {
				t.Errorf("kinds = %d, want %d", len(got.Kinds), len(maps.KindDefaults))
			}
			if len(got.Icons) != len(maps.MarkerIconCatalog()) || got.DefaultIcon != maps.DefaultMarkerIcon {
				t.Errorf("icons = %d (default %q), want the full catalog", len(got.Icons), got.DefaultIcon)
			}
		})
	}
}

// The wire keys are what the Foundry module reads; renaming one breaks it.
func TestGetMapLook_WireKeys(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.SetParamNames("id")
	c.SetParamValues("c1")
	if err := NewMapAPIHandler(nil, &stubMapSvcLook{frame: "old"}, nil, nil).GetMapLook(c); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"campaign_frame", "frames", "kinds", "icons", "default_icon"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing wire key %q", k)
		}
	}
}
