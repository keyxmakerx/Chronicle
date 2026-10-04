package maps

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif" // decoders for every picture type the media plugin accepts
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	_ "golang.org/x/image/webp"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MediaImageSource reads the original bytes of a campaign's picture. Implemented
// in app over the media plugin, so maps never touches media's storage or
// repository.
type MediaImageSource interface {
	ReadImage(ctx context.Context, campaignID, mediaID string) ([]byte, error)
}

// Limits on what is decoded to render a player copy. The side limit equals the
// media plugin's upload cap (kept separately because maps never imports media),
// so it only refuses rows that predate that cap. The pixel cap is the real
// memory guard: a decoded 10000x10000 picture is hundreds of MB, and the side
// limit alone allows exactly that.
const (
	maxPlayerImageSourceSide   = 10000
	maxPlayerImageSourcePixels = 50_000_000
)

// checkPlayerImageSize refuses a picture too big to decode safely. It runs on
// the header-only DecodeConfig result, before any pixels are allocated.
func checkPlayerImageSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("map picture has no size (%dx%d)", width, height)
	}
	if width > maxPlayerImageSourceSide || height > maxPlayerImageSourceSide ||
		int64(width)*int64(height) > maxPlayerImageSourcePixels {
		return fmt.Errorf("map picture is too large to prepare (%dx%d)", width, height)
	}
	return nil
}

// renderSlots bounds how many player copies are rendered at once. A full-size
// render holds a few hundred MB, and a shadow edit invalidates the copy for
// every player at the same moment.
var renderSlots = make(chan struct{}, 2)

// safeFileStem keeps a cache file name to characters a database id can contain.
var safeFileStem = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// SetPlayerImageSource wires where originals are read and where player copies
// are cached. The cache directory is created on first write; if it cannot be
// written the copy is still served, just regenerated next time. Unwired, the
// player image route refuses rather than falling back to the original.
func (s *mapService) SetPlayerImageSource(src MediaImageSource, cacheDir string) {
	s.images = src
	s.imageCacheDir = cacheDir
}

// playerImageURL is where a viewer subject to hiding fetches the picture. The
// version is the cache key, so the address changes whenever a shadow does and a
// browser never shows a stale copy.
func playerImageURL(campaignID, mapID, key string) string {
	return fmt.Sprintf("/campaigns/%s/maps/%s/player-image?v=%s", campaignID, mapID, key[:16])
}

// shadowedAreas returns the shadows of a map for a viewer subject to hiding.
// An unwired lookup means no shadows are known (tests and installs without
// drawings); the production wiring is pinned by a test in app.
func (s *mapService) shadowedAreas(ctx context.Context, mapID string) ([]ShadowArea, error) {
	if s.shadows == nil {
		return nil, nil
	}
	return s.shadows.ShadowAreas(ctx, mapID)
}

// ForViewer implements MapService. For a viewer who is not owner/DM-equivalent
// and a map that has a shadow, it returns a copy with the original image id
// removed and the player copy's address in its place; otherwise m itself. A
// failed shadow lookup is an error, never the original.
func (s *mapService) ForViewer(ctx context.Context, m *Map, role int) (*Map, error) {
	if m == nil || !shadowHidingApplies(role) || m.ImageID == nil || *m.ImageID == "" {
		return m, nil
	}
	areas, err := s.shadowedAreas(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("list shadow areas: %w", err)
	}
	if len(areas) == 0 {
		return m, nil
	}
	cp := *m
	cp.ImageID = nil
	cp.PlayerImageURL = playerImageURL(m.CampaignID, m.ID, playerImageKey(*m.ImageID, m.ID, areas))
	return &cp, nil
}

// PlayerImageVersion implements MapService. The version is the start of the
// cache key, so it changes whenever the picture or any shadow does.
func (s *mapService) PlayerImageVersion(ctx context.Context, m *Map) (string, error) {
	if m == nil || m.ImageID == nil || *m.ImageID == "" {
		return "", nil
	}
	areas, err := s.shadowedAreas(ctx, m.ID)
	if err != nil {
		return "", fmt.Errorf("list shadow areas: %w", err)
	}
	if len(areas) == 0 {
		return "", nil
	}
	return playerImageKey(*m.ImageID, m.ID, areas)[:16], nil
}

// ForViewerList is ForViewer over a list.
func (s *mapService) ForViewerList(ctx context.Context, ms []Map, role int) ([]Map, error) {
	if !shadowHidingApplies(role) {
		return ms, nil
	}
	out := make([]Map, len(ms))
	for i := range ms {
		vm, err := s.ForViewer(ctx, &ms[i], role)
		if err != nil {
			return nil, err
		}
		out[i] = *vm
	}
	return out, nil
}

// PlayerImage implements MapService: the JPEG of the map's picture with every
// shadow smudged in. m must be the stored map (it carries the original id). It
// returns an error, never the original, when the copy cannot be produced.
func (s *mapService) PlayerImage(ctx context.Context, m *Map) ([]byte, error) {
	if m == nil || m.ImageID == nil || *m.ImageID == "" {
		return nil, apperror.NewNotFound("this map has no picture")
	}
	if s.images == nil || s.shadows == nil {
		return nil, apperror.NewInternal(fmt.Errorf("player image is not wired"))
	}
	areas, err := s.shadows.ShadowAreas(ctx, m.ID)
	if err != nil {
		return nil, fmt.Errorf("list shadow areas: %w", err)
	}
	key := playerImageKey(*m.ImageID, m.ID, areas)
	path := s.playerImagePath(m.ID, key)
	if data := readCached(path); data != nil {
		return data, nil
	}

	select {
	case renderSlots <- struct{}{}:
		defer func() { <-renderSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// Another request may have finished the same copy while this one waited.
	if data := readCached(path); data != nil {
		return data, nil
	}

	original, err := s.images.ReadImage(ctx, m.CampaignID, *m.ImageID)
	if err != nil {
		return nil, fmt.Errorf("read map picture: %w", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("map picture is not a readable image: %w", err))
	}
	if err := checkPlayerImageSize(cfg.Width, cfg.Height); err != nil {
		return nil, apperror.NewInternal(err)
	}
	src, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("decode map picture: %w", err))
	}
	data, err := encodePlayerImage(renderPlayerImage(src, areas, playerImageMaxSide))
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("encode player image: %w", err))
	}
	s.storeCached(m.ID, path, data)
	return data, nil
}

// playerImagePath is the cache file for a map and key, or "" when no cache
// directory is configured or the map id is not a plain id.
func (s *mapService) playerImagePath(mapID, key string) string {
	if s.imageCacheDir == "" || !safeFileStem.MatchString(mapID) {
		return ""
	}
	return filepath.Join(s.imageCacheDir, mapID+"-"+key[:32]+".jpg")
}

// readCached returns the cached copy, or nil on a miss or an empty file.
func readCached(path string) []byte {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// storeCached writes the copy atomically (temp file, then rename) so a reader
// never sees a half-written file, and removes the map's older copies. Failures
// are logged only: the copy is still served, just not kept.
func (s *mapService) storeCached(mapID, path string, data []byte) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		slog.Warn("maps: player image cache directory unavailable", slog.Any("error", err))
		return
	}
	tmp, err := os.CreateTemp(dir, mapID+"-*.tmp")
	if err != nil {
		slog.Warn("maps: cannot cache player image", slog.Any("error", err))
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
		slog.Warn("maps: cannot cache player image", slog.String("map_id", mapID))
		return
	}
	stale, _ := filepath.Glob(filepath.Join(dir, mapID+"-*.jpg"))
	for _, f := range stale {
		if f != path {
			_ = os.Remove(f)
		}
	}
}
