package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// memShadowRepo is an in-memory drawing store; only the shadow-relevant
// methods are implemented, the embedded nil interface covers the rest.
type memShadowRepo struct {
	DrawingRepository
	rows map[string]*Drawing
}

func (r *memShadowRepo) CreateDrawing(_ context.Context, d *Drawing) error {
	cp := *d
	r.rows[d.ID] = &cp
	return nil
}
func (r *memShadowRepo) GetDrawing(_ context.Context, id string) (*Drawing, error) {
	cp := *r.rows[id]
	return &cp, nil
}
func (r *memShadowRepo) UpdateDrawing(_ context.Context, d *Drawing) error {
	cp := *d
	r.rows[d.ID] = &cp
	return nil
}
func (r *memShadowRepo) DeleteDrawing(_ context.Context, id string) error {
	delete(r.rows, id)
	return nil
}
func (r *memShadowRepo) ListShadows(_ context.Context, mapID string) ([]Drawing, error) {
	var out []Drawing
	for _, d := range r.rows {
		if d.MapID == mapID && d.DrawingType == DrawingTypeShadow {
			out = append(out, *d)
		}
	}
	return out, nil
}

// pictureHarness wires a real map service to a real drawing service the way
// production does, counting the map listings the guard causes.
type pictureHarness struct {
	maps     *mapService
	drawings DrawingService
	repo     *mockMapRepo
	listed   int
	image    *string
}

func newPictureHarness(t *testing.T) *pictureHarness {
	t.Helper()
	img := originalMediaID
	h := &pictureHarness{image: &img}
	h.repo = &mockMapRepo{
		listMapsFn: func(context.Context, string) ([]Map, error) {
			h.listed++
			return []Map{{ID: "map-1", CampaignID: "camp-1", ImageID: h.image}}, nil
		},
		getMapFn: func(context.Context, string) (*Map, error) {
			return &Map{ID: "map-1", CampaignID: "camp-1", Name: "n", ImageID: h.image}, nil
		},
		updateMapFn: func(context.Context, *Map) error { return nil },
		deleteMapFn: func(context.Context, string) error { return nil },
	}
	h.maps = NewMapService(h.repo).(*mapService)
	h.drawings = NewDrawingService(&memShadowRepo{rows: map[string]*Drawing{}})
	h.drawings.SetMapLookup(func(context.Context, string) (string, error) { return "camp-1", nil })
	h.maps.SetShadowLookup(h.drawings)
	return h
}

func (h *pictureHarness) shadowed(t *testing.T) bool {
	t.Helper()
	got, err := h.maps.IsShadowedMapImage(context.Background(), "camp-1", originalMediaID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (h *pictureHarness) addShadow(t *testing.T) *Drawing {
	t.Helper()
	d, err := h.drawings.CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-1", DrawingType: DrawingTypeShadow, CallerRole: permissions.RoleOwner, CallerIsDM: true,
		Points: json.RawMessage(`[{"x":10,"y":10},{"x":30,"y":30}]`), CreatedBy: "dm",
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// A shadow written through the service takes effect on the very next check,
// even right after a "not shadowed" answer was cached; deleting it lifts it.
func TestPictureStatus_ShadowWritesInvalidate(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, h *pictureHarness)
		want  bool
	}{
		{"create shadow", func(t *testing.T, h *pictureHarness) { h.addShadow(t) }, true},
		{"delete shadow", func(t *testing.T, h *pictureHarness) {
			d := h.addShadow(t)
			h.shadowed(t) // cache the shadowed answer
			if err := h.drawings.DeleteDrawing(context.Background(), d.ID, "map-1", nil, "", 3, true); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"update shadow keeps it shadowed", func(t *testing.T, h *pictureHarness) {
			d := h.addShadow(t)
			h.shadowed(t)
			if err := h.drawings.UpdateDrawing(context.Background(), d.ID, "map-1", permissions.RoleOwner, true, UpdateDrawingInput{}); err != nil {
				t.Fatal(err)
			}
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPictureHarness(t)
			if h.shadowed(t) { // primes "not shadowed"
				t.Fatal("no shadow yet")
			}
			tc.write(t, h)
			if got := h.shadowed(t); got != tc.want {
				t.Errorf("shadowed after write = %v, want %v", got, tc.want)
			}
		})
	}
}

// A map's picture changing or the map going away also drops the answer.
func TestPictureStatus_MapWritesInvalidate(t *testing.T) {
	cases := []struct {
		name  string
		write func(h *pictureHarness) error
	}{
		{"map update", func(h *pictureHarness) error {
			return h.maps.UpdateMap(context.Background(), "map-1", UpdateMapInput{Name: "n"})
		}},
		{"map delete", func(h *pictureHarness) error {
			return h.maps.DeleteMap(context.Background(), "map-1", nil)
		}},
		{"map create", func(h *pictureHarness) error {
			_, err := h.maps.CreateMap(context.Background(), CreateMapInput{CampaignID: "camp-1", Name: "n"})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPictureHarness(t)
			h.shadowed(t)
			before := h.listed
			if err := tc.write(h); err != nil {
				t.Fatal(err)
			}
			h.shadowed(t)
			if h.listed != before+1 {
				t.Errorf("maps listed %d times after the write, want a fresh lookup", h.listed-before)
			}
		})
	}
}

func TestPictureStatus_CacheBehaviour(t *testing.T) {
	cases := []struct {
		name      string
		run       func(t *testing.T, h *pictureHarness, now *time.Time)
		wantLists int
	}{
		{"repeat within the TTL is cached", func(t *testing.T, h *pictureHarness, _ *time.Time) { h.shadowed(t); h.shadowed(t) }, 1},
		{"is-picture and is-shadowed share one entry", func(t *testing.T, h *pictureHarness, _ *time.Time) {
			h.shadowed(t)
			_, _ = h.maps.IsMapPicture(context.Background(), "camp-1", originalMediaID)
		}, 1},
		{"entry expires after the TTL", func(t *testing.T, h *pictureHarness, now *time.Time) {
			h.shadowed(t)
			*now = now.Add(pictureStatusTTL + time.Second)
			h.shadowed(t)
		}, 2},
		{"another campaign's write leaves the entry", func(t *testing.T, h *pictureHarness, _ *time.Time) {
			h.shadowed(t)
			h.maps.InvalidateMapPictures("camp-2")
			h.shadowed(t)
		}, 1},
		{"unknown campaign flushes everything", func(t *testing.T, h *pictureHarness, _ *time.Time) {
			h.shadowed(t)
			h.maps.InvalidateMapPictures("")
			h.shadowed(t)
		}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPictureHarness(t)
			now := time.Unix(1000, 0)
			h.maps.pictureCache.now = func() time.Time { return now }
			tc.run(t, h, &now)
			if h.listed != tc.wantLists {
				t.Errorf("ListMaps called %d times, want %d", h.listed, tc.wantLists)
			}
		})
	}
}

// A result computed before an invalidation is never stored, so a slow check
// that raced a shadow write cannot leave "not shadowed" behind.
func TestPictureStatus_InFlightResultDroppedAfterInvalidate(t *testing.T) {
	c := newPictureStatusCache()
	_, gen, _ := c.get("camp-1", "m")
	c.invalidate("camp-1")
	c.put("camp-1", "m", pictureStatus{}, gen)
	if _, _, ok := c.get("camp-1", "m"); ok {
		t.Error("a stale result was cached")
	}
}

func TestPictureStatus_BoundedAndErrorsNotCached(t *testing.T) {
	c := newPictureStatusCache()
	for i := 0; i < pictureStatusMaxEntries+10; i++ {
		_, gen, _ := c.get("c", fmt.Sprint(i))
		c.put("c", fmt.Sprint(i), pictureStatus{}, gen)
	}
	if len(c.entries) > pictureStatusMaxEntries {
		t.Errorf("cache holds %d entries, cap is %d", len(c.entries), pictureStatusMaxEntries)
	}

	h := newPictureHarness(t)
	h.repo.listMapsFn = func(context.Context, string) ([]Map, error) { h.listed++; return nil, errors.New("down") }
	for i := 0; i < 2; i++ {
		if got, err := h.maps.IsShadowedMapImage(context.Background(), "camp-1", originalMediaID); err == nil || !got {
			t.Fatalf("want true with an error, got %v, %v", got, err)
		}
	}
	if h.listed != 2 {
		t.Errorf("a failure was cached (%d lookups)", h.listed)
	}
}

func TestIsMapPicture(t *testing.T) {
	cases := []struct {
		name    string
		areas   []ShadowArea
		lookup  error
		media   string
		want    bool
		wantErr bool
	}{
		{"shadowed picture", []ShadowArea{hintArea}, nil, originalMediaID, true, false},
		{"unshadowed picture is still a map picture", nil, nil, originalMediaID, true, false},
		{"file no map uses", []ShadowArea{hintArea}, nil, "media-unused", false, false},
		{"shadow lookup fails: treated as a picture", nil, errors.New("db"), originalMediaID, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shadowedMapService(tc.areas, tc.lookup).IsMapPicture(context.Background(), "camp-1", tc.media)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got %v, %v; want %v, err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestCheckPlayerImageSize(t *testing.T) {
	cases := []struct {
		name    string
		w, h    int
		wantErr bool
	}{
		{"ordinary", 4000, 3000, false},
		{"exactly 50 megapixels", 10000, 5000, false},
		{"over the pixel cap within the side cap", 10000, 5001, true},
		{"square at the upload cap", 10000, 10000, true},
		{"one side over the cap", 10001, 10, true},
		{"old limit no longer allowed", 11000, 100, true},
		{"empty", 0, 100, true},
		{"negative", -1, 100, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkPlayerImageSize(tc.w, tc.h); (err != nil) != tc.wantErr {
				t.Errorf("checkPlayerImageSize(%d,%d) err=%v, wantErr=%v", tc.w, tc.h, err, tc.wantErr)
			}
		})
	}
}
