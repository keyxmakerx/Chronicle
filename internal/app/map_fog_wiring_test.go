package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type fixedFogLookup struct{ mask *maps.FogMask }

func (f fixedFogLookup) FogMask(context.Context, string) (*maps.FogMask, error) { return f.mask, nil }

type errFogLookup struct{}

func (errFogLookup) FogMask(context.Context, string) (*maps.FogMask, error) {
	return nil, errors.New("db down")
}

// fogMask is a 1000x1000 map with hexes 100 wide; only hex (1,1) is explored.
func fogMask(anchor string) *maps.FogMask {
	return &maps.FogMask{
		Version: 3, Geo: maps.NewHexGeometry(100, 1000, 1000), MapW: 1000, MapH: 1000,
		Explored: map[maps.HexKey]bool{{Col: 1, Row: 1}: true}, AnchorID: anchor,
	}
}

func pct(m *maps.FogMask, col, row int) (float64, float64) {
	x, y := m.Geo.Center(col, row)
	return x / 10, y / 10
}

func TestMapEventPublisher_UnderFog(t *testing.T) {
	m := fogMask("")
	dx, dy := pct(m, 4, 4)
	check := func(f *maps.FogMask) bool { return f.HidesPoint(dx, dy) }
	cases := []struct {
		name string
		a    *mapEventPublisherAdapter
		want bool
	}{
		{"nil lookup restricts", &mapEventPublisherAdapter{}, true},
		{"lookup error restricts", &mapEventPublisherAdapter{fog: errFogLookup{}}, true},
		{"item in an unexplored hex restricts", &mapEventPublisherAdapter{fog: fixedFogLookup{m}}, true},
		{"no fog does not", &mapEventPublisherAdapter{fog: fixedFogLookup{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.underFog("m", check); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
	lx, ly := pct(m, 1, 1)
	a := &mapEventPublisherAdapter{fog: fixedFogLookup{m}}
	if a.underFog("m", func(f *maps.FogMask) bool { return f.HidesPoint(lx, ly) }) {
		t.Error("an item in an explored hex must not be restricted")
	}
}

func TestMapEventPublisher_MarkerAndDrawingEventsUnderFog(t *testing.T) {
	m := fogMask("pic")
	dx, dy := pct(m, 4, 4)
	lx, ly := pct(m, 1, 1)
	pts := func(x, y float64) json.RawMessage {
		b, _ := json.Marshal([]map[string]float64{{"x": x, "y": y}, {"x": x, "y": y}})
		return b
	}
	cases := []struct {
		name       string
		publish    func(a *mapEventPublisherAdapter)
		wantDMOnly bool
	}{
		{"a pin in an unexplored hex", func(a *mapEventPublisherAdapter) {
			a.PublishMarkerEvent("created", "c", &maps.Marker{ID: "k", MapID: "m", X: dx, Y: dy, Visibility: "everyone"})
		}, true},
		{"a pin in an explored hex", func(a *mapEventPublisherAdapter) {
			a.PublishMarkerEvent("created", "c", &maps.Marker{ID: "k", MapID: "m", X: lx, Y: ly, Visibility: "everyone"})
		}, false},
		{"a drawing wholly in the dark", func(a *mapEventPublisherAdapter) {
			a.PublishDrawingEvent("updated", "c", &maps.Drawing{ID: "d", MapID: "m", DrawingType: "freehand", Points: pts(dx, dy), Visibility: "everyone"})
		}, true},
		{"a drawing on explored land", func(a *mapEventPublisherAdapter) {
			a.PublishDrawingEvent("updated", "c", &maps.Drawing{ID: "d", MapID: "m", DrawingType: "freehand", Points: pts(lx, ly), Visibility: "everyone"})
		}, false},
		{"the picture a fogged layer is pinned to carries its file id", func(a *mapEventPublisherAdapter) {
			a.PublishDrawingEvent("updated", "c", &maps.Drawing{ID: "pic", MapID: "m", DrawingType: "image", Points: pts(lx, ly), Visibility: "everyone", ImageID: strPtr("media-1")})
		}, true},
		{"a shadow still reaches everyone", func(a *mapEventPublisherAdapter) {
			a.PublishDrawingEvent("updated", "c", &maps.Drawing{ID: "s", MapID: "m", DrawingType: maps.DrawingTypeShadow, Points: pts(dx, dy), Visibility: "everyone"})
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := &captureBus{}
			tc.publish(&mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{m}})
			if bus.last == nil {
				t.Fatal("nothing published")
			}
			if bus.last.RequiresDM != tc.wantDMOnly {
				t.Errorf("RequiresDM = %v, want %v", bus.last.RequiresDM, tc.wantDMOnly)
			}
		})
	}
}

// hex.changed carries the map id, the version and (only when given) the path:
// never a cell, a name or a note, whatever the hex service hands it.
func TestPublishHexChanged_PayloadNeverCarriesCells(t *testing.T) {
	cases := []struct {
		name     string
		path     []maps.HexKey
		wantKeys []string
	}{
		{"a paint or reveal batch", nil, []string{"map_id", "version"}},
		{"a party move", []maps.HexKey{{Col: 1, Row: 1}, {Col: 2, Row: 1}}, []string{"map_id", "party_path", "version"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := &captureBus{}
			a := &mapEventPublisherAdapter{bus: bus}
			a.PublishHexChanged("camp-1", "map-9", 12, tc.path)
			if bus.last == nil {
				t.Fatal("nothing published")
			}
			if bus.last.Type != ws.MsgHexChanged || string(ws.MsgHexChanged) != "hex.changed" {
				t.Errorf("type = %q", bus.last.Type)
			}
			if bus.last.RequiresDM || len(bus.last.AllowedUsers) != 0 {
				t.Error("hex.changed reaches every client; it carries nothing to gate")
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(bus.last.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			var got []string
			for k := range payload {
				got = append(got, k)
			}
			if len(got) != len(tc.wantKeys) {
				t.Fatalf("payload keys = %v, want %v", got, tc.wantKeys)
			}
			for _, k := range tc.wantKeys {
				if _, ok := payload[k]; !ok {
					t.Errorf("payload is missing %q", k)
				}
			}
			for _, banned := range []string{"cells", "cell", "terrain", "name", "notes", "explored", "layer"} {
				if _, ok := payload[banned]; ok {
					t.Errorf("payload carries %q", banned)
				}
			}
		})
	}
	// No campaign, no event.
	bus := &captureBus{}
	(&mapEventPublisherAdapter{bus: bus}).PublishHexChanged("", "map-9", 1, nil)
	if bus.last != nil {
		t.Error("an event with no campaign must not be published")
	}
}

type fogWiringMapRepo struct{ maps.MapRepository }

func (fogWiringMapRepo) ListMarkers(context.Context, string, int, string) ([]maps.Marker, error) {
	return []maps.Marker{{ID: "dark", MapID: "m", X: 70, Y: 70}, {ID: "lit", MapID: "m", X: 10, Y: 10}}, nil
}

// The production wiring must hand the map service, the drawing service and the
// publisher their fog source, and the hex service its publisher and map loader.
func TestWireHexFog_WiresEverySink(t *testing.T) {
	mapsSvc := maps.NewMapService(fogWiringMapRepo{})
	drawSvc := maps.NewDrawingService(shadowTestDrawRepo{})
	events := newMapEventPublisher(nil, drawSvc)
	hexSvc := &recordingHexService{HexService: maps.NewHexService(nil)}
	wireHexFog(mapsSvc, drawSvc, events, hexSvc)

	if events.fog == nil {
		t.Error("the event publisher was built without a fog lookup")
	}
	if !hexSvc.publisherSet || !hexSvc.loaderSet || !hexSvc.invalidatorSet || !hexSvc.artSet {
		t.Errorf("hex service wiring: publisher=%v loader=%v invalidator=%v art=%v", hexSvc.publisherSet, hexSvc.loaderSet, hexSvc.invalidatorSet, hexSvc.artSet)
	}
	// The map service now asks the hex service: its mask hides the dark pin.
	hexSvc.mask = fogMask("")
	hexSvc.mask.Explored = map[maps.HexKey]bool{}
	got, err := mapsSvc.ListMarkers(context.Background(), "c", "m", permissions.RolePlayer, "u")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("player sees %v, want every pin hidden under all-unexplored fog", got)
	}
	if owner, _ := mapsSvc.ListMarkers(context.Background(), "c", "m", permissions.RoleOwner, "u"); len(owner) != 2 {
		t.Errorf("owner sees %d pins, want 2", len(owner))
	}
}

// recordingHexService stands in for the hex service: it records the wiring
// calls and serves a chosen mask.
type recordingHexService struct {
	maps.HexService
	mask                                    *maps.FogMask
	publisherSet, loaderSet, invalidatorSet bool
	artSet                                  bool
}

func (r *recordingHexService) FogMask(context.Context, string) (*maps.FogMask, error) {
	return r.mask, nil
}
func (r *recordingHexService) SetEventPublisher(maps.HexEventPublisher) { r.publisherSet = true }
func (r *recordingHexService) SetMapLoader(func(context.Context, string) (*maps.Map, error)) {
	r.loaderSet = true
}
func (r *recordingHexService) SetPictureInvalidator(func(string)) { r.invalidatorSet = true }
func (r *recordingHexService) SetArtWriter(func(context.Context, string, string) error) {
	r.artSet = true
}

// The wiring helper only protects anyone if the route setup calls it, so the
// call is pinned in source.
func TestRoutes_CallsWireHexFog(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "wireHexFog(mapsService, drawingService, mapEvents, hexService)") {
		t.Error("routes.go no longer wires the hex fog; players would see every pin and the whole picture")
	}
}

// fogMediaMapRepo has one map in the campaign, so the media guard has a map to
// ask about.
type fogMediaMapRepo struct{ fogWiringMapRepo }

func (fogMediaMapRepo) ListMaps(context.Context, string) ([]maps.Map, error) {
	return []maps.Map{{ID: "m", CampaignID: "c"}}, nil
}

// fogTokenDrawRepo has a token and a picture in unexplored land (only hex
// (1,1) is explored in fogMask) and a token in hex (1,1).
type fogTokenDrawRepo struct{ shadowTestDrawRepo }

func (fogTokenDrawRepo) ListTokens(context.Context, string, int) ([]maps.Token, error) {
	m := fogMask("")
	dx, dy := pct(m, 4, 4)
	lx, ly := pct(m, 1, 1)
	return []maps.Token{{ID: "dark", MapID: "m", X: dx, Y: dy}, {ID: "lit", MapID: "m", X: lx, Y: ly}}, nil
}

func (fogTokenDrawRepo) ListDrawings(context.Context, string, int, string) ([]maps.Drawing, error) {
	return []maps.Drawing{{ID: "p", MapID: "m", DrawingType: "image", ImageID: strPtr("media-dark"),
		Points: json.RawMessage(`[{"x":38,"y":44},{"x":42,"y":48}]`)}}, nil
}

// The same wiring also covers tokens and the media server: a player's token
// list loses the token in the dark, and the file of a picture in the dark is
// refused as a withheld map picture.
func TestWireHexFog_TokensAndMediaGuard(t *testing.T) {
	mapsSvc := maps.NewMapService(fogMediaMapRepo{})
	drawSvc := maps.NewDrawingService(fogTokenDrawRepo{})
	hexSvc := &recordingHexService{HexService: maps.NewHexService(nil), mask: fogMask("")}
	wireHexFog(mapsSvc, drawSvc, newMapEventPublisher(nil, drawSvc), hexSvc)

	toks, err := drawSvc.ListTokens(context.Background(), "m", permissions.RolePlayer)
	if err != nil || len(toks) != 1 || toks[0].ID != "lit" {
		t.Errorf("player tokens = %v (err %v), want only the lit one", toks, err)
	}
	if hidden, err := mapsSvc.IsShadowedMapImage(context.Background(), "c", "media-dark"); err != nil || !hidden {
		t.Errorf("a picture file under the fog: hidden=%v err=%v, want refused", hidden, err)
	}
	if hidden, err := mapsSvc.IsShadowedMapImage(context.Background(), "c", "media-other"); err != nil || hidden {
		t.Errorf("an unrelated file: hidden=%v err=%v, want served", hidden, err)
	}
}

func TestMapEventPublisher_TokenEventsUnderFog(t *testing.T) {
	m := fogMask("")
	dx, dy := pct(m, 4, 4)
	lx, ly := pct(m, 1, 1)
	cases := []struct {
		name       string
		publish    func(a *mapEventPublisherAdapter)
		wantDMOnly bool
	}{
		{"a token created in the dark", func(a *mapEventPublisherAdapter) {
			a.PublishTokenEvent("created", "c", &maps.Token{ID: "t", MapID: "m", X: dx, Y: dy})
		}, true},
		{"a token created on explored land", func(a *mapEventPublisherAdapter) {
			a.PublishTokenEvent("created", "c", &maps.Token{ID: "t", MapID: "m", X: lx, Y: ly})
		}, false},
		{"a hidden token on explored land", func(a *mapEventPublisherAdapter) {
			a.PublishTokenEvent("updated", "c", &maps.Token{ID: "t", MapID: "m", X: lx, Y: ly, IsHidden: true})
		}, true},
		{"a token dragged into the dark", func(a *mapEventPublisherAdapter) {
			a.PublishTokenPositionEvent("c", "m", "t", dx, dy, false)
		}, true},
		{"a token dragged on explored land", func(a *mapEventPublisherAdapter) {
			a.PublishTokenPositionEvent("c", "m", "t", lx, ly, false)
		}, false},
		{"a picture reaching from explored land into the dark carries its file id", func(a *mapEventPublisherAdapter) {
			pts, _ := json.Marshal([]map[string]float64{{"x": lx, "y": ly}, {"x": dx, "y": dy}})
			a.PublishDrawingEvent("updated", "c", &maps.Drawing{ID: "p", MapID: "m", DrawingType: "image", Points: pts, Visibility: "everyone", ImageID: strPtr("media-1")})
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := &captureBus{}
			tc.publish(&mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{m}})
			if bus.last == nil {
				t.Fatal("nothing published")
			}
			if bus.last.RequiresDM != tc.wantDMOnly {
				t.Errorf("RequiresDM = %v, want %v", bus.last.RequiresDM, tc.wantDMOnly)
			}
		})
	}
}
