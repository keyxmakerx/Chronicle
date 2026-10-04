package syncapi

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/labstack/echo/v4"
)

const playerImageAPIPrefix = `"player_image_url":"/api/v1/campaigns/camp-1/maps/map-1/player-image?v=`

// noShadows is a shadow lookup for a map that has none.
type noShadows struct{}

func (noShadows) ShadowAreas(context.Context, string) ([]maps.ShadowArea, error) { return nil, nil }

// pngSource serves a small PNG as every map picture.
type pngSource struct{ data []byte }

func (s pngSource) ReadImage(context.Context, string, string) ([]byte, error) { return s.data, nil }

func smallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 32))
	for x := 0; x < 40; x++ {
		for y := 0; y < 32; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 6), uint8(y * 7), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Every key, the owner's included, is told where the player copy of a shadowed
// map lives on this API: the module syncs with the owner's key and must hand
// players that copy, never the original. A map with no shadow has no address.
func TestMapAPIHandler_PlayerImageAPIURL(t *testing.T) {
	cases := []struct {
		name    string
		role    campaigns.Role
		shadows maps.ShadowLookup
		wantURL bool
	}{
		{"owner, shadowed", campaigns.RoleOwner, pictureShadows{}, true},
		{"scribe, shadowed", campaigns.RoleScribe, pictureShadows{}, true},
		{"player, shadowed", campaigns.RolePlayer, pictureShadows{}, true},
		{"owner, no shadow", campaigns.RoleOwner, noShadows{}, false},
		{"player, no shadow", campaigns.RolePlayer, noShadows{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := maps.NewMapService(pictureRepo{})
			svc.SetShadowLookup(tc.shadows)
			h := NewMapAPIHandler(nil, svc, nil, &stubCampaignSvcOwnerGate{role: tc.role})
			key := &APIKey{CampaignID: "camp-1", UserID: "u1"}
			for name, call := range map[string]func(echo.Context) error{"GetMap": h.GetMap, "ListMaps": h.ListMaps} {
				c, rec := newMapAPIContext(http.MethodGet, "/", key)
				if err := call(c); err != nil || rec.Code != http.StatusOK {
					t.Fatalf("%s: code=%d err=%v", name, rec.Code, err)
				}
				if has := strings.Contains(rec.Body.String(), playerImageAPIPrefix); has != tc.wantURL {
					t.Errorf("%s has player_image_url = %v, want %v: %s", name, has, tc.wantURL, rec.Body.String())
				}
			}
		})
	}
}

// The player copy is served to a key as a JPEG, and an unwired renderer is an
// error, never the original.
func TestMapAPIHandler_PlayerImage(t *testing.T) {
	original := smallPNG(t)
	for _, wired := range []bool{true, false} {
		svc := maps.NewMapService(pictureRepo{})
		svc.SetShadowLookup(pictureShadows{})
		if wired {
			svc.SetPlayerImageSource(pngSource{data: original}, t.TempDir())
		}
		h := NewMapAPIHandler(nil, svc, nil, &stubCampaignSvcOwnerGate{role: campaigns.RoleOwner})
		c, rec := newMapAPIContext(http.MethodGet, "/", &APIKey{CampaignID: "camp-1", UserID: "u1"})
		err := h.PlayerImage(c)
		if !wired {
			if err == nil || bytes.Equal(rec.Body.Bytes(), original) {
				t.Errorf("unwired: err=%v, served original=%v", err, bytes.Equal(rec.Body.Bytes(), original))
			}
			continue
		}
		if err != nil || rec.Code != http.StatusOK {
			t.Fatalf("PlayerImage: code=%d err=%v", rec.Code, err)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
			t.Errorf("Content-Type = %q, want image/jpeg", ct)
		}
		if bytes.Equal(rec.Body.Bytes(), original) {
			t.Error("served the original picture")
		}
	}
}
