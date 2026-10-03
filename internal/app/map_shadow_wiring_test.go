package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type shadowTestMapRepo struct{ maps.MapRepository }

func (shadowTestMapRepo) ListMarkers(context.Context, string, int, string) ([]maps.Marker, error) {
	return []maps.Marker{{ID: "under", MapID: "m", X: 20, Y: 20}, {ID: "clear", MapID: "m", X: 90, Y: 90}}, nil
}

type shadowTestDrawRepo struct{ maps.DrawingRepository }

func (shadowTestDrawRepo) ListShadows(context.Context, string) ([]maps.Drawing, error) {
	return []maps.Drawing{{ID: "s", MapID: "m", DrawingType: "shadow", Points: []byte(`[{"x":10,"y":10},{"x":30,"y":30}]`)}}, nil
}

// The production wiring must hand the map service its shadow source: a player's
// marker list loses the pin under the shadow, an owner's does not.
func TestWireMapShadows_PlayersLosePinsUnderShadow(t *testing.T) {
	mapsSvc := maps.NewMapService(shadowTestMapRepo{})
	drawSvc := maps.NewDrawingService(shadowTestDrawRepo{})
	wireMapShadows(mapsSvc, drawSvc)

	got, err := mapsSvc.ListMarkers(context.Background(), "c", "m", permissions.RolePlayer, "u")
	if err != nil {
		t.Fatal(err)
	}
	// The entity gate is unwired here, but these markers carry no entity.
	if len(got) != 1 || got[0].ID != "clear" {
		t.Fatalf("player sees %v, want only the pin outside the shadow", got)
	}
	owner, _ := mapsSvc.ListMarkers(context.Background(), "c", "m", permissions.RoleOwner, "u")
	if len(owner) != 2 {
		t.Fatalf("owner sees %d pins, want 2", len(owner))
	}
}

func TestNewMapEventPublisher_HasShadowLookup(t *testing.T) {
	a := newMapEventPublisher(nil, maps.NewDrawingService(shadowTestDrawRepo{}))
	if a.shadows == nil {
		t.Fatal("publisher built without a shadow lookup")
	}
}

type errShadowLookup struct{}

func (errShadowLookup) ShadowAreas(context.Context, string) ([]maps.ShadowArea, error) {
	return nil, errors.New("db down")
}

type fixedShadowLookup struct{ areas []maps.ShadowArea }

func (f fixedShadowLookup) ShadowAreas(context.Context, string) ([]maps.ShadowArea, error) {
	return f.areas, nil
}

func TestMapEventPublisher_UnderShadow(t *testing.T) {
	inside := func(a []maps.ShadowArea) bool { return len(a) > 0 && a[0].Contains(20, 20) }
	area := []maps.ShadowArea{{MinX: 10, MinY: 10, MaxX: 30, MaxY: 30}}
	cases := []struct {
		name string
		a    *mapEventPublisherAdapter
		want bool
	}{
		{"nil lookup restricts", &mapEventPublisherAdapter{}, true},
		{"lookup error restricts", &mapEventPublisherAdapter{shadows: errShadowLookup{}}, true},
		{"item under shadow restricts", &mapEventPublisherAdapter{shadows: fixedShadowLookup{area}}, true},
		{"no shadows does not", &mapEventPublisherAdapter{shadows: fixedShadowLookup{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.underShadow("m", inside); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// The restriction must reach the published message, not just the helper.
type shadowCaptureBus struct {
	ws.EventBus
	msgs []ws.Message
}

func (b *shadowCaptureBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, *m) }

func TestMapEventPublisher_MarkerEventRestrictedWhenShadowed(t *testing.T) {
	bus := &shadowCaptureBus{}
	a := &mapEventPublisherAdapter{bus: bus}
	a.PublishMarkerEvent("created", "c", &maps.Marker{ID: "k", MapID: "m"})
	if len(bus.msgs) != 1 || !bus.msgs[0].RequiresDM {
		t.Fatalf("expected one DM-only message, got %+v", bus.msgs)
	}
}

// stubPictureMedia serves one image file for wireMapPictures.
type stubPictureMedia struct {
	media.MediaService
	campaign string
	path     string
}

func (s stubPictureMedia) GetByID(_ context.Context, id string) (*media.MediaFile, error) {
	c := s.campaign
	return &media.MediaFile{ID: id, CampaignID: &c, MimeType: "image/png"}, nil
}
func (s stubPictureMedia) FilePath(*media.MediaFile) string { return s.path }

// The production wiring must give the media server its guard and the map
// service its originals, with the cache beside (not inside) the media root.
func TestWireMapPictures(t *testing.T) {
	root := t.TempDir()
	mediaRoot := filepath.Join(root, "media")
	if err := os.MkdirAll(mediaRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	picture := filepath.Join(mediaRoot, "map.png")
	img := image.NewNRGBA(image.Rect(0, 0, 60, 40))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(picture, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	mapsSvc := maps.NewMapService(shadowTestMapRepo{})
	wireMapShadows(mapsSvc, maps.NewDrawingService(shadowTestDrawRepo{}))
	mediaHandler := media.NewHandler(nil)
	wireMapPictures(mapsSvc, stubPictureMedia{campaign: "c", path: picture}, mediaHandler, mediaRoot)

	if !mediaHandler.MapImageGuardWired() {
		t.Fatal("media server has no map picture guard; the original would stay downloadable")
	}
	pic := "media-1"
	data, err := mapsSvc.PlayerImage(context.Background(), &maps.Map{ID: "m", CampaignID: "c", ImageID: &pic})
	if err != nil || len(data) == 0 {
		t.Fatalf("player image: %d bytes, err=%v", len(data), err)
	}
	if cached, _ := filepath.Glob(filepath.Join(root, "map-player-images", "m-*.jpg")); len(cached) != 1 {
		t.Errorf("cache files %v, want one in a sibling of the media root", cached)
	}
	if inside, _ := filepath.Glob(filepath.Join(mediaRoot, "*.jpg")); len(inside) != 0 {
		t.Errorf("cache must not live inside the media root (the orphan sweep deletes it): %v", inside)
	}

	// A map cannot be pointed at another campaign's picture.
	src := &mapImageSourceAdapter{svc: stubPictureMedia{campaign: "other", path: picture}}
	if _, err := src.ReadImage(context.Background(), "c", "media-1"); err == nil {
		t.Error("ReadImage must refuse a file from another campaign")
	}
}
