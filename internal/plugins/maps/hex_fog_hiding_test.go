package maps

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// fakeFogLookup serves a fixed mask (nil: no fog) or an error.
type fakeFogLookup struct {
	mask *FogMask
	err  error
}

func (f fakeFogLookup) FogMask(context.Context, string) (*FogMask, error) { return f.mask, f.err }

// fogPositions returns percentages for the centre of an unexplored hex and of
// an explored one in fogMaskFixture.
func fogPositions() (darkX, darkY, litX, litY float64) {
	g := fogMaskFixture().Geo
	darkX, darkY = pctOf(g, HexKey{4, 4}, 1000)
	litX, litY = pctOf(g, HexKey{1, 1}, 1000)
	return
}

func TestMapService_ListMarkers_FogHiding(t *testing.T) {
	dx, dy, lx, ly := fogPositions()
	all := []Marker{
		{ID: "dark", MapID: "map-1", X: dx, Y: dy},
		{ID: "lit", MapID: "map-1", X: lx, Y: ly},
	}
	repo := &mockMapRepo{listMarkersFn: func(context.Context, string, int) ([]Marker, error) {
		return append([]Marker(nil), all...), nil
	}}
	build := func(l HexFogLookup) *mapService {
		s := NewMapService(repo).(*mapService)
		s.SetHexFogLookup(l)
		return s
	}
	ids := func(ms []Marker) string {
		var out []string
		for _, m := range ms {
			out = append(out, m.ID)
		}
		return strings.Join(out, ",")
	}
	mask := fogMaskFixture()

	tests := []struct {
		name string
		role int
		want string
	}{
		{"public visitor", permissions.RoleNone, "lit"},
		{"player", permissions.RolePlayer, "lit"},
		{"scribe", permissions.RoleScribe, "lit"},
		{"owner, and a co-DM promoted to owner", permissions.RoleOwner, "dark,lit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := build(fakeFogLookup{mask: mask}).ListMarkers(context.Background(), "camp-1", "map-1", tc.role, "u")
			if err != nil {
				t.Fatal(err)
			}
			if ids(got) != tc.want {
				t.Errorf("sees %q, want %q", ids(got), tc.want)
			}
		})
	}

	t.Run("no fog hides nothing", func(t *testing.T) {
		got, _ := build(fakeFogLookup{}).ListMarkers(context.Background(), "camp-1", "map-1", permissions.RolePlayer, "u")
		if len(got) != 2 {
			t.Errorf("sees %d pins, want 2", len(got))
		}
	})
	t.Run("a failed fog lookup fails the list", func(t *testing.T) {
		if _, err := build(fakeFogLookup{err: errors.New("db down")}).ListMarkers(context.Background(), "camp-1", "map-1", permissions.RolePlayer, "u"); err == nil {
			t.Error("the list must fail closed, not leak")
		}
	})
	t.Run("by id", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			mk   *Marker
			role int
			want bool
		}{
			{"dark pin, player", &all[0], permissions.RolePlayer, true},
			{"dark pin, scribe", &all[0], permissions.RoleScribe, true},
			{"dark pin, owner", &all[0], permissions.RoleOwner, false},
			{"lit pin, player", &all[1], permissions.RolePlayer, false},
		} {
			got, err := build(fakeFogLookup{mask: mask}).IsMarkerShadowed(context.Background(), tc.mk, tc.role)
			if err != nil || got != tc.want {
				t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
			}
		}
		got, err := build(fakeFogLookup{err: errors.New("down")}).IsMarkerShadowed(context.Background(), &all[1], permissions.RolePlayer)
		if err == nil || !got {
			t.Errorf("a failed lookup must answer hidden with an error, got %v, %v", got, err)
		}
	})
}

func TestDrawingService_FogHiding(t *testing.T) {
	dx, dy, lx, ly := fogPositions()
	line := func(a, b float64, c, d float64) []byte {
		return []byte(`[{"x":` + fl(a) + `,"y":` + fl(b) + `},{"x":` + fl(c) + `,"y":` + fl(d) + `}]`)
	}
	dark := Drawing{ID: "dark", MapID: "map-1", DrawingType: "freehand", Points: line(dx, dy, dx+1, dy)}
	mixed := Drawing{ID: "mixed", MapID: "map-1", DrawingType: "freehand", Points: line(dx, dy, lx, ly)}
	lit := Drawing{ID: "lit", MapID: "map-1", DrawingType: "freehand", Points: line(lx, ly, lx, ly)}
	repo := &shadowRepo{drawings: []Drawing{dark, mixed, lit}}
	build := func(l HexFogLookup) DrawingService {
		svc := NewDrawingService(repo)
		svc.SetHexFogLookup(l)
		return svc
	}
	ids := func(ds []Drawing) string {
		var out []string
		for _, d := range ds {
			out = append(out, d.ID)
		}
		return strings.Join(out, ",")
	}
	mask := fogMaskFixture()

	for _, tc := range []struct {
		name string
		role int
		want string
	}{
		{"public visitor", permissions.RoleNone, "mixed,lit"},
		{"player", permissions.RolePlayer, "mixed,lit"},
		{"scribe", permissions.RoleScribe, "mixed,lit"},
		{"owner / co-DM", permissions.RoleOwner, "dark,mixed,lit"},
	} {
		t.Run("list: "+tc.name, func(t *testing.T) {
			got, err := build(fakeFogLookup{mask: mask}).ListDrawings(context.Background(), "map-1", tc.role, "u")
			if err != nil || ids(got) != tc.want {
				t.Errorf("sees %q (err %v), want %q", ids(got), err, tc.want)
			}
		})
	}
	t.Run("list: a failed fog lookup fails the list", func(t *testing.T) {
		if _, err := build(fakeFogLookup{err: errors.New("db down")}).ListDrawings(context.Background(), "map-1", permissions.RolePlayer, "u"); err == nil {
			t.Error("must fail closed")
		}
	})
	for _, tc := range []struct {
		name string
		d    Drawing
		role int
		want bool
	}{
		{"by id: dark, player", dark, permissions.RolePlayer, true},
		{"by id: dark, scribe", dark, permissions.RoleScribe, true},
		{"by id: dark, owner", dark, permissions.RoleOwner, false},
		{"by id: mixed, player", mixed, permissions.RolePlayer, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.d
			got, err := build(fakeFogLookup{mask: mask}).IsDrawingShadowed(context.Background(), &d, tc.role)
			if err != nil || got != tc.want {
				t.Errorf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	t.Run("by id: a failed lookup answers hidden", func(t *testing.T) {
		d := lit
		got, err := build(fakeFogLookup{err: errors.New("down")}).IsDrawingShadowed(context.Background(), &d, permissions.RolePlayer)
		if err == nil || !got {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

// An anchored layer's picture file is the secret: its image id goes only to
// viewers who may see DM-only content.
func TestDrawingService_WithholdImages(t *testing.T) {
	img := "media-secret"
	pic := Drawing{ID: "pic", MapID: "map-1", DrawingType: DrawingTypeImage, ImageID: &img}
	other := Drawing{ID: "other", MapID: "map-1", DrawingType: DrawingTypeImage, ImageID: &img}
	anchored := fogMaskFixture()
	anchored.AnchorID = "pic"
	build := func(l HexFogLookup) DrawingService {
		svc := NewDrawingService(&shadowRepo{})
		svc.SetHexFogLookup(l)
		return svc
	}
	tests := []struct {
		name     string
		lookup   fakeFogLookup
		role     int
		wantPic  bool // the anchor picture keeps its image
		wantOthr bool
		wantErr  bool
	}{
		{"player, anchored fog", fakeFogLookup{mask: anchored}, permissions.RolePlayer, false, true, false},
		{"scribe, anchored fog", fakeFogLookup{mask: anchored}, permissions.RoleScribe, false, true, false},
		{"public, anchored fog", fakeFogLookup{mask: anchored}, permissions.RoleNone, false, true, false},
		{"owner, anchored fog", fakeFogLookup{mask: anchored}, permissions.RoleOwner, true, true, false},
		{"player, whole-map fog", fakeFogLookup{mask: fogMaskFixture()}, permissions.RolePlayer, true, true, false},
		{"player, no fog", fakeFogLookup{}, permissions.RolePlayer, true, true, false},
		{"player, lookup fails", fakeFogLookup{err: errors.New("down")}, permissions.RolePlayer, false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := []Drawing{pic, other}
			got, err := build(tc.lookup).WithholdImages(context.Background(), "map-1", tc.role, in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if (got[0].ImageID != nil) != tc.wantPic || (got[1].ImageID != nil) != tc.wantOthr {
				t.Errorf("anchor image kept = %v, other kept = %v", got[0].ImageID != nil, got[1].ImageID != nil)
			}
			// The caller's slice is never edited: a cached or shared list stays whole.
			if in[0].ImageID == nil {
				t.Error("WithholdImages edited its input")
			}
		})
	}
}

// fogMapService is a map service whose one map has a picture and the given fog.
func fogMapService(fog *FogMask, err error) *mapService {
	s := shadowedMapService(nil, nil)
	s.SetHexFogLookup(fakeFogLookup{mask: fog, err: err})
	return s
}

func TestForViewer_FogSwapsInThePlayerCopy(t *testing.T) {
	anchored := fogMaskFixture()
	anchored.AnchorID = "pic"
	tests := []struct {
		name         string
		fog          *FogMask
		role         int
		wantOriginal bool
	}{
		{"player, whole-map fog", fogMaskFixture(), permissions.RolePlayer, false},
		{"scribe, whole-map fog", fogMaskFixture(), permissions.RoleScribe, false},
		{"public, whole-map fog", fogMaskFixture(), permissions.RoleNone, false},
		{"owner, whole-map fog", fogMaskFixture(), permissions.RoleOwner, true},
		{"player, anchored fog leaves the background alone", anchored, permissions.RolePlayer, true},
		{"player, no fog", nil, permissions.RolePlayer, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := fogMapService(tc.fog, nil)
			stored, _ := s.GetMap(context.Background(), "map-1")
			got, err := s.ForViewer(context.Background(), stored, tc.role)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantOriginal != (got.ImageID != nil) {
				t.Fatalf("original id sent = %v, want %v", got.ImageID != nil, tc.wantOriginal)
			}
			if !tc.wantOriginal && !strings.Contains(got.PlayerImageURL, "/player-image?v=") {
				t.Errorf("player URL = %q", got.PlayerImageURL)
			}
		})
	}
	t.Run("a failed fog lookup is an error with no map", func(t *testing.T) {
		s := fogMapService(nil, errors.New("down"))
		stored, _ := s.GetMap(context.Background(), "map-1")
		if got, err := s.ForViewer(context.Background(), stored, permissions.RolePlayer); err == nil || got != nil {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

func TestPlayerImageKeyFog_ChangesOnAReveal(t *testing.T) {
	base := playerImageKeyFog(originalMediaID, "map-1", nil, fogMaskFixture())
	if again := playerImageKeyFog(originalMediaID, "map-1", nil, fogMaskFixture()); again != base {
		t.Error("the same state must give the same key")
	}
	revealed := fogMaskFixture()
	revealed.Version++
	if playerImageKeyFog(originalMediaID, "map-1", nil, revealed) == base {
		t.Error("a new layer version (a reveal) must give a new key")
	}
	resized := fogMaskFixture()
	resized.Geo = NewHexGeometry(80, 1000, 1000)
	if playerImageKeyFog(originalMediaID, "map-1", nil, resized) == base {
		t.Error("a new hex size must give a new key")
	}
	if playerImageKeyFog(originalMediaID, "map-1", nil, nil) == base {
		t.Error("fog must be part of the key")
	}
	if playerImageKeyFog(originalMediaID, "map-1", []ShadowArea{hintArea}, nil) != playerImageKey(originalMediaID, "map-1", []ShadowArea{hintArea}) {
		t.Error("without fog the key is the plain shadow key")
	}
}

func TestPlayerImageVersion_FogChangesWithTheLayerVersion(t *testing.T) {
	m := &Map{ID: "map-1", CampaignID: "camp-1", ImageID: strPtr(originalMediaID)}
	v := func(version uint64) string {
		f := fogMaskFixture()
		f.Version = version
		got, err := fogMapService(f, nil).PlayerImageVersion(context.Background(), m)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if v(1) == "" || v(1) == v(2) {
		t.Errorf("versions = %q, %q; want a value that changes on a reveal", v(1), v(2))
	}
	if got, _ := fogMapService(nil, nil).PlayerImageVersion(context.Background(), m); got != "" {
		t.Errorf("no fog and no shadow means the original: %q", got)
	}
}

func TestIsShadowedMapImage_Fog(t *testing.T) {
	anchored := fogMaskFixture()
	anchored.AnchorID = "pic"
	tests := []struct {
		name    string
		fog     *FogMask
		err     error
		media   string
		want    bool
		wantErr bool
	}{
		{"whole-map fog refuses the original", fogMaskFixture(), nil, originalMediaID, true, false},
		{"anchored fog leaves the background alone", anchored, nil, originalMediaID, false, false},
		{"no fog", nil, nil, originalMediaID, false, false},
		{"fog on another file", fogMaskFixture(), nil, "media-unused", false, false},
		{"a failed lookup refuses", nil, errors.New("down"), originalMediaID, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := fogMapService(tc.fog, tc.err).IsShadowedMapImage(context.Background(), "camp-1", tc.media)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got %v, %v; want %v, err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
	// The cached "not shadowed" answer must not survive turning fog on.
	s := fogMapService(nil, nil)
	if got, _ := s.IsShadowedMapImage(context.Background(), "camp-1", originalMediaID); got {
		t.Fatal("no fog yet")
	}
	s.SetHexFogLookup(fakeFogLookup{mask: fogMaskFixture()})
	if got, _ := s.IsShadowedMapImage(context.Background(), "camp-1", originalMediaID); !got {
		t.Error("rewiring the fog must drop the cached answer")
	}
	s.InvalidateMapPictures("camp-1")
}

// A small picture and a field of 50-wide hexes: hex (2,2) is explored, hex
// (1,4) is not.
func smallFog() *FogMask {
	return &FogMask{
		Version: 1, Geo: NewHexGeometry(250, 200, 200), MapW: 200, MapH: 200,
		Explored: map[HexKey]bool{{2, 2}: true},
	}
}

func TestRenderPlayerImageFog_SmudgesUnexploredHexesOnly(t *testing.T) {
	src := checkerboard(200, 200, 4)
	fog := smallFog()
	out := renderPlayerImageFog(src, nil, fog, 4096)

	cx, cy := fog.Geo.Center(1, 4)
	dark := image.Rect(int(cx)-4, int(cy)-4, int(cx)+4, int(cy)+4)
	if mean, _, std := stats(out, src, dark); mean > 20 || std > 12 {
		t.Errorf("unexplored hex: mean %.1f std %.1f, want a dark formless blur", mean, std)
	}
	ex, ey := fog.Geo.Center(2, 2)
	lit := image.Rect(int(ex)-4, int(ey)-4, int(ex)+4, int(ey)+4)
	if _, diff, _ := stats(out, src, lit); diff > 4 {
		t.Errorf("explored hex differs from the original by %.1f, want it untouched", diff)
	}
	// Nothing recoverable: the checker pattern is gone from the unexplored hex.
	if _, diff, _ := stats(out, src, dark); diff < 60 {
		t.Errorf("unexplored hex still within %.1f of the original", diff)
	}
}

func TestRenderPlayerImageFog_NoFogIsTheShadowRender(t *testing.T) {
	src := checkerboard(80, 60, 4)
	a := renderPlayerImageFog(src, []ShadowArea{hintArea}, nil, 4096)
	b := renderPlayerImage(src, []ShadowArea{hintArea}, 4096)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("without fog the render must be unchanged")
	}
}

func TestRenderPlayerImageFog_LandOutsideTheFieldIsUntouched(t *testing.T) {
	src := checkerboard(200, 200, 4)
	// A field laid inside the left half of the picture only.
	fog := &FogMask{
		Version: 1, Geo: NewAnchoredHexGeometry(250, 0, 0, 100, 200), MapW: 200, MapH: 200,
		Explored: map[HexKey]bool{}, AnchorID: "pic",
	}
	out := renderPlayerImageFog(src, nil, fog, 4096)
	if _, diff, _ := stats(out, src, image.Rect(150, 20, 190, 180)); diff > 1 {
		t.Errorf("land outside the field changed by %.1f", diff)
	}
	if _, diff, _ := stats(out, src, image.Rect(30, 30, 60, 60)); diff < 40 {
		t.Errorf("land inside the field barely changed (%.1f)", diff)
	}
}

// With fog on, the cached player copy is replaced the moment the layer version
// moves, and both files never coexist.
func TestPlayerImage_FogRendersAndRegeneratesOnAReveal(t *testing.T) {
	dir := t.TempDir()
	src := &fakeImageSource{data: testPNG(t)}
	fog := &FogMask{
		Version: 1, Geo: NewHexGeometry(250, 120, 80), MapW: 120, MapH: 80,
		Explored: map[HexKey]bool{{1, 1}: true},
	}
	s := shadowedMapService(nil, nil)
	s.SetPlayerImageSource(src, dir)
	s.SetHexFogLookup(fakeFogLookup{mask: fog})
	stored, _ := s.GetMap(context.Background(), "map-1")

	first, err := s.PlayerImage(context.Background(), stored)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	cx, cy := fog.Geo.Center(3, 2)
	if mean, _, _ := stats(decoded, decoded, image.Rect(int(cx)-3, int(cy)-3, int(cx)+3, int(cy)+3)); mean > 25 {
		t.Errorf("an unexplored hex has brightness %.1f, want near black", mean)
	}
	if _, err := s.PlayerImage(context.Background(), stored); err != nil || src.reads != 1 {
		t.Errorf("second call: err=%v reads=%d, want the cached copy", err, src.reads)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "map-1-*.jpg"))

	revealed := *fog
	revealed.Version = 2
	revealed.Explored = map[HexKey]bool{{1, 1}: true, {3, 2}: true}
	s.SetHexFogLookup(fakeFogLookup{mask: &revealed})
	if _, err := s.PlayerImage(context.Background(), stored); err != nil || src.reads != 2 {
		t.Errorf("after a reveal: err=%v reads=%d, want a fresh render", err, src.reads)
	}
	after, _ := filepath.Glob(filepath.Join(dir, "map-1-*.jpg"))
	if len(files) != 1 || len(after) != 1 || after[0] == files[0] {
		t.Errorf("cache files %v then %v, want the old copy replaced", files, after)
	}
}

func TestPlayerImage_FogLookupFailureIsAnError(t *testing.T) {
	s := fogMapService(nil, errors.New("down"))
	s.SetPlayerImageSource(&fakeImageSource{data: testPNG(t)}, t.TempDir())
	stored, _ := s.GetMap(context.Background(), "map-1")
	if data, err := s.PlayerImage(context.Background(), stored); err == nil || data != nil {
		t.Errorf("got %d bytes, err %v; want an error and no image", len(data), err)
	}
}
