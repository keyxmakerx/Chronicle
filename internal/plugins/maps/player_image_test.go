package maps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const originalMediaID = "media-original-0001"

var (
	hintArea   = ShadowArea{MinX: 10, MinY: 10, MaxX: 40, MaxY: 40, Strength: ShadowAlphaHint}
	hiddenArea = ShadowArea{MinX: 50, MinY: 50, MaxX: 90, MaxY: 90, Strength: ShadowAlphaHidden}
)

// fakeImageSource serves one generated picture and counts reads.
type fakeImageSource struct {
	data  []byte
	err   error
	reads int
}

func (f *fakeImageSource) ReadImage(context.Context, string, string) ([]byte, error) {
	f.reads++
	return f.data, f.err
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := checkerboard(120, 80, 4)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// shadowedMapService builds a map service whose one map has the given picture
// and shadows.
func shadowedMapService(areas []ShadowArea, lookupErr error) *mapService {
	img := originalMediaID
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			return &Map{ID: id, CampaignID: "camp-1", Name: "Vellmoor", ImageID: &img, ImageWidth: 120, ImageHeight: 80}, nil
		},
		listMapsFn: func(context.Context, string) ([]Map, error) {
			other := "media-other"
			return []Map{
				{ID: "map-1", CampaignID: "camp-1", ImageID: &img},
				{ID: "map-2", CampaignID: "camp-1", ImageID: &other},
				{ID: "map-3", CampaignID: "camp-1"},
			}, nil
		},
	}
	s := NewMapService(repo).(*mapService)
	s.SetShadowLookup(fakeShadowLookup{areas: areas, err: lookupErr})
	return s
}

func TestForViewer_WhichRoleGetsWhichPicture(t *testing.T) {
	shadows := []ShadowArea{hintArea}
	cases := []struct {
		name         string
		role         int
		areas        []ShadowArea
		wantOriginal bool
	}{
		{"anonymous visitor gets the player copy", permissions.RoleNone, shadows, false},
		{"player gets the player copy", permissions.RolePlayer, shadows, false},
		{"scribe gets the player copy", permissions.RoleScribe, shadows, false},
		{"owner gets the original", permissions.RoleOwner, shadows, true},
		{"co-DM (promoted to owner) gets the original", int(campaigns.RoleOwner), shadows, true},
		{"player gets the original when there are no shadows", permissions.RolePlayer, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := shadowedMapService(tc.areas, nil)
			stored, _ := s.GetMap(context.Background(), "map-1")
			got, err := s.ForViewer(context.Background(), stored, tc.role)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantOriginal {
				if got.ImageID == nil || *got.ImageID != originalMediaID || got.PlayerImageURL != "" {
					t.Errorf("want the original id and no player URL, got %v / %q", got.ImageID, got.PlayerImageURL)
				}
				return
			}
			if got.ImageID != nil {
				t.Errorf("the original id must not be sent, got %q", *got.ImageID)
			}
			if !strings.HasPrefix(got.PlayerImageURL, "/campaigns/camp-1/maps/map-1/player-image?v=") {
				t.Errorf("player URL = %q", got.PlayerImageURL)
			}
			if !got.HasImage() {
				t.Error("the player copy must still count as the map's picture")
			}
			// The stored map is untouched, so the next viewer is judged afresh.
			if stored.ImageID == nil || stored.PlayerImageURL != "" {
				t.Error("ForViewer must not modify the map it was given")
			}
			// Nothing a viewer receives as JSON may carry the original id.
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), originalMediaID) {
				t.Errorf("serialised map still names the original picture: %s", raw)
			}
		})
	}
}

func TestForViewer_FailsClosedAndPassesThroughWhenNothingToHide(t *testing.T) {
	s := shadowedMapService([]ShadowArea{hintArea}, errors.New("db down"))
	stored, _ := s.GetMap(context.Background(), "map-1")
	if got, err := s.ForViewer(context.Background(), stored, permissions.RolePlayer); err == nil || got != nil {
		t.Errorf("a failed shadow lookup must be an error with no map, got %v, %v", got, err)
	}
	// Owners are never subject to the lookup.
	if got, err := s.ForViewer(context.Background(), stored, permissions.RoleOwner); err != nil || got != stored {
		t.Errorf("owner: %v, %v", got, err)
	}
	noPic := &Map{ID: "map-3", CampaignID: "camp-1"}
	if got, err := s.ForViewer(context.Background(), noPic, permissions.RolePlayer); err != nil || got != noPic {
		t.Errorf("a map with no picture passes through: %v, %v", got, err)
	}
	if got, err := s.ForViewer(context.Background(), nil, permissions.RolePlayer); err != nil || got != nil {
		t.Errorf("nil map: %v, %v", got, err)
	}
}

func TestForViewerList(t *testing.T) {
	s := shadowedMapService([]ShadowArea{hintArea}, nil)
	list, _ := s.ListMaps(context.Background(), "camp-1")
	got, err := s.ForViewerList(context.Background(), list, permissions.RolePlayer)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if m.ImageID != nil {
			t.Errorf("map %s still carries an original picture id", m.ID)
		}
	}
	if got[2].PlayerImageURL != "" || got[2].HasImage() {
		t.Error("a map with no picture gets no player URL")
	}
	owner, _ := s.ForViewerList(context.Background(), list, permissions.RoleOwner)
	if owner[0].ImageID == nil {
		t.Error("owner keeps the originals")
	}
	if _, err := shadowedMapService([]ShadowArea{hintArea}, errors.New("x")).ForViewerList(context.Background(), list, permissions.RolePlayer); err == nil {
		t.Error("a failed lookup must fail the list")
	}
}

func TestIsShadowedMapImage(t *testing.T) {
	cases := []struct {
		name    string
		areas   []ShadowArea
		lookup  error
		media   string
		want    bool
		wantErr bool
	}{
		{"picture of a map with a shadow", []ShadowArea{hintArea}, nil, originalMediaID, true, false},
		{"picture of a map without shadows", nil, nil, originalMediaID, false, false},
		{"a picture no map uses", []ShadowArea{hintArea}, nil, "media-unused", false, false},
		{"shadow lookup fails: refuse", nil, errors.New("db down"), originalMediaID, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shadowedMapService(tc.areas, tc.lookup).IsShadowedMapImage(context.Background(), "camp-1", tc.media)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got %v, %v; want %v, err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
	// A map list that cannot be read also refuses.
	s := shadowedMapService([]ShadowArea{hintArea}, nil)
	s.repo.(*mockMapRepo).listMapsFn = func(context.Context, string) ([]Map, error) { return nil, errors.New("down") }
	if got, err := s.IsShadowedMapImage(context.Background(), "camp-1", originalMediaID); err == nil || !got {
		t.Errorf("list failure: got %v, %v; want true with an error", got, err)
	}
}

func TestPlayerImage_RendersCachesAndRegenerates(t *testing.T) {
	dir := t.TempDir()
	src := &fakeImageSource{data: testPNG(t)}
	s := shadowedMapService([]ShadowArea{hiddenArea}, nil)
	s.SetPlayerImageSource(src, dir)
	stored, _ := s.GetMap(context.Background(), "map-1")

	first, err := s.PlayerImage(context.Background(), stored)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("player image is not a JPEG: %v", err)
	}
	original, _ := png.Decode(bytes.NewReader(src.data))
	// The shadowed centre of the box is nearly black; the unshadowed corner is
	// still the checkerboard (JPEG blurs it a little, so only check it is not dark).
	if mean, _, _ := stats(decoded, original, image.Rect(66, 44, 78, 52)); mean > 20 {
		t.Errorf("shadowed area brightness %.1f, want near black", mean)
	}
	if mean, diff, _ := stats(decoded, original, image.Rect(0, 0, 24, 16)); mean < 60 || diff > 60 {
		t.Errorf("unshadowed area mean %.1f diff %.1f, want it close to the original", mean, diff)
	}

	// Second request is served from disk, not re-rendered.
	if _, err := s.PlayerImage(context.Background(), stored); err != nil || src.reads != 1 {
		t.Errorf("second call: err=%v reads=%d, want one read in total", err, src.reads)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "map-1-*.jpg"))
	if len(files) != 1 {
		t.Fatalf("cache files = %v, want one", files)
	}

	// A shadow change gives a new copy and removes the old file.
	s.SetShadowLookup(fakeShadowLookup{areas: []ShadowArea{hiddenArea, hintArea}})
	if _, err := s.PlayerImage(context.Background(), stored); err != nil || src.reads != 2 {
		t.Errorf("after a shadow change: err=%v reads=%d, want a fresh render", err, src.reads)
	}
	after, _ := filepath.Glob(filepath.Join(dir, "map-1-*.jpg"))
	if len(after) != 1 || after[0] == files[0] {
		t.Errorf("cache files after change = %v, want only the new one", after)
	}
}

// Whatever goes wrong, the answer is an error and never the original bytes.
func TestPlayerImage_NeverFallsBackToTheOriginal(t *testing.T) {
	good := testPNG(t)
	cases := []struct {
		name  string
		build func(t *testing.T) (*mapService, *Map)
	}{
		{"source read fails", func(t *testing.T) (*mapService, *Map) {
			s := shadowedMapService([]ShadowArea{hintArea}, nil)
			s.SetPlayerImageSource(&fakeImageSource{err: errors.New("disk")}, t.TempDir())
			return s, nil
		}},
		{"source is not an image", func(t *testing.T) (*mapService, *Map) {
			s := shadowedMapService([]ShadowArea{hintArea}, nil)
			s.SetPlayerImageSource(&fakeImageSource{data: []byte("<svg onload=alert(1)>")}, t.TempDir())
			return s, nil
		}},
		{"shadow lookup fails", func(t *testing.T) (*mapService, *Map) {
			s := shadowedMapService([]ShadowArea{hintArea}, errors.New("db"))
			s.SetPlayerImageSource(&fakeImageSource{data: good}, t.TempDir())
			return s, nil
		}},
		{"image source not wired", func(t *testing.T) (*mapService, *Map) {
			return shadowedMapService([]ShadowArea{hintArea}, nil), nil
		}},
		{"map has no picture", func(t *testing.T) (*mapService, *Map) {
			s := shadowedMapService([]ShadowArea{hintArea}, nil)
			s.SetPlayerImageSource(&fakeImageSource{data: good}, t.TempDir())
			return s, &Map{ID: "map-9", CampaignID: "camp-1"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, m := tc.build(t)
			if m == nil {
				m, _ = s.GetMap(context.Background(), "map-1")
			}
			data, err := s.PlayerImage(context.Background(), m)
			if err == nil || data != nil {
				t.Errorf("want an error and no bytes, got %d bytes, err=%v", len(data), err)
			}
		})
	}
}

// An unwritable cache directory still serves the (smudged) copy.
func TestPlayerImage_UnwritableCacheStillServes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := shadowedMapService([]ShadowArea{hintArea}, nil)
	s.SetPlayerImageSource(&fakeImageSource{data: testPNG(t)}, filepath.Join(file, "sub"))
	stored, _ := s.GetMap(context.Background(), "map-1")
	data, err := s.PlayerImage(context.Background(), stored)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("not a JPEG: %v", err)
	}
}

// Every surface that sends a map to a viewer: the page, the focus-view
// fragment, the entity-page preview, the list and the meta JSON. None may carry
// the original picture id or a /media/ address for a viewer subject to hiding,
// and the owner must still get the original.
func TestMapSurfaces_NeverCarryTheOriginalToAHiddenViewer(t *testing.T) {
	campaignCtx := func(role campaigns.Role, granted bool) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: granted}
	}
	viewers := []struct {
		name         string
		cc           *campaigns.CampaignContext
		wantOriginal bool
	}{
		{"player", campaignCtx(campaigns.RolePlayer, false), false},
		{"scribe", campaignCtx(campaigns.RoleScribe, false), false},
		{"owner", campaignCtx(campaigns.RoleOwner, false), true},
		{"co-DM", campaignCtx(campaigns.RoleScribe, true), true},
	}
	for _, v := range viewers {
		t.Run(v.name, func(t *testing.T) {
			h := NewHandler(shadowedMapService([]ShadowArea{hintArea}, nil))
			surfaces := map[string]string{}

			for name, fn := range map[string]echo.HandlerFunc{"page": h.Show, "viewer": h.Viewer, "meta": h.GetMapMetaAPI, "list": h.Index} {
				rec, err := serveHandler(t, fn, "/x", v.cc, "map-1")
				if err != nil || rec.Code != http.StatusOK {
					t.Fatalf("%s: code=%d err=%v", name, rec.Code, err)
				}
				surfaces[name] = rec.Body.String()
			}
			// The entity-page preview, built the way the widget type builds it.
			mapSvc := h.svc
			stored, _ := mapSvc.GetMap(context.Background(), "map-1")
			role := v.cc.VisibilityRole()
			vm, err := mapSvc.ForViewer(context.Background(), stored, role)
			if err != nil {
				t.Fatal(err)
			}
			var sb strings.Builder
			if err := BlockEntityMapEmbed(v.cc, "ent-1", MapViewData{CampaignID: "camp-1", Map: vm}, false, "default").Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			surfaces["entity preview"] = sb.String()

			for name, body := range surfaces {
				has := strings.Contains(body, originalMediaID)
				if has != v.wantOriginal {
					t.Errorf("%s: names the original picture = %v, want %v", name, has, v.wantOriginal)
				}
				if !v.wantOriginal && !strings.Contains(body, "player-image") {
					t.Errorf("%s: no player copy address", name)
				}
			}
		})
	}
}

func TestHandler_PlayerImage(t *testing.T) {
	playerCtx := func(campaignID string) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: campaignID}, MemberRole: campaigns.RolePlayer}
	}
	build := func(src MediaImageSource, lookupErr error) *Handler {
		s := shadowedMapService([]ShadowArea{hiddenArea}, lookupErr)
		if src != nil {
			s.SetPlayerImageSource(src, t.TempDir())
		}
		return NewHandler(s)
	}

	t.Run("serves a JPEG with safe headers", func(t *testing.T) {
		rec, err := serveHandler(t, build(&fakeImageSource{data: testPNG(t)}, nil).PlayerImage, "/x", playerCtx("camp-1"), "map-1")
		if err != nil || rec.Code != http.StatusOK {
			t.Fatalf("code=%d err=%v", rec.Code, err)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
			t.Errorf("Content-Type = %q", ct)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(rec.Header().Get("Cache-Control"), "private") {
			t.Errorf("headers = %v", rec.Header())
		}
		if _, err := jpeg.Decode(rec.Body); err != nil {
			t.Errorf("body is not a JPEG: %v", err)
		}
	})
	t.Run("a map of another campaign is not found", func(t *testing.T) {
		_, err := serveHandler(t, build(&fakeImageSource{data: testPNG(t)}, nil).PlayerImage, "/x", playerCtx("camp-2"), "map-1")
		if err == nil {
			t.Error("a map outside the campaign must not be served")
		}
	})
	for name, h := range map[string]*Handler{
		"unreadable original gives an error, not the original": build(&fakeImageSource{err: errors.New("disk")}, nil),
		"unwired source gives an error":                        build(nil, nil),
		"failed shadow lookup gives an error":                  build(&fakeImageSource{data: testPNG(t)}, errors.New("db")),
	} {
		t.Run(name, func(t *testing.T) {
			rec, err := serveHandler(t, h.PlayerImage, "/x", playerCtx("camp-1"), "map-1")
			if err == nil || rec.Body.Len() != 0 {
				t.Errorf("want an error and no body, got err=%v body=%d bytes", err, rec.Body.Len())
			}
		})
	}
}
