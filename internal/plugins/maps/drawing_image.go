package maps

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DrawingTypeImage marks a drawing that is a picture from the campaign's
// media placed on the map: two opposite corners, rotation, opacity in
// fill_alpha, and a media file plus crop that no other drawing type carries.
const DrawingTypeImage = "image"

// Picture limits. Opacity never reaches zero so a picture cannot be made
// invisible yet still sit on the map; a crop never trims more than 45% from
// a side so opposite sides cannot meet and the visible part always has area.
const (
	imageMinOpacity  = 0.2
	imageMaxOpacity  = 1.0
	imageMaxCropSide = 45.0
	imageMaxTurn     = 45.0
)

// MediaVerifier answers whether a media file may be used as a picture on a
// campaign's map. It is a seam so this plugin never reads the media plugin's
// repository; the app wires an adapter over the media service.
type MediaVerifier interface {
	// ImageInCampaign is true only when the file exists, is an image and
	// belongs to the campaign. A missing file is a clean false, not an error.
	ImageInCampaign(ctx context.Context, mediaID, campaignID string) (bool, error)
}

// imageCrop is the stored crop: percent trimmed from each edge.
type imageCrop struct {
	T float64 `json:"t"`
	R float64 `json:"r"`
	B float64 `json:"b"`
	L float64 `json:"l"`
}

// dropNullJSON turns a literal JSON null into "absent", because a body key sent
// as null binds to the four bytes "null" rather than to an empty message.
func dropNullJSON(raw json.RawMessage) json.RawMessage {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	return raw
}

// validateCrop requires finite sides within 0-45 so the visible part never
// collapses, and returns the crop re-serialised from the four parsed sides so
// any other key a caller sent is never stored. An empty crop (no trimming) is
// valid and stays empty.
func validateCrop(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var c imageCrop
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, apperror.NewBadRequest("crop must be an object with t, r, b and l")
	}
	for _, v := range []float64{c.T, c.R, c.B, c.L} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > imageMaxCropSide {
			return nil, apperror.NewBadRequest("each crop side must be between 0 and 45")
		}
	}
	clean, err := json.Marshal(c)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return clean, nil
}

// clampImageOpacity keeps a picture's opacity inside 0.2-1.0. An unset
// opacity (zero) means fully opaque, since a new picture is sent without one.
func clampImageOpacity(a float64) float64 {
	if a == 0 || math.IsNaN(a) {
		return imageMaxOpacity
	}
	return math.Min(imageMaxOpacity, math.Max(imageMinOpacity, a))
}

// validateImageGeometry checks the shape rules shared by create and update:
// two finite corners and a sane turn.
func validateImageGeometry(points json.RawMessage, rotation float64) error {
	pts, ok := parsePoints(points)
	if !ok || len(pts) != 2 {
		return apperror.NewBadRequest("a picture needs exactly two corner points")
	}
	if math.IsNaN(rotation) || math.IsInf(rotation, 0) || math.Abs(rotation) > imageMaxTurn {
		return apperror.NewBadRequest("rotation must be between -45 and 45 degrees")
	}
	return nil
}

// verifyImageMedia refuses a media id that is not an image of the map's
// campaign. It fails closed when the lookups are not wired, so a missing
// wire can never let a cross-campaign file through.
func (s *drawingService) verifyImageMedia(ctx context.Context, mapID, mediaID string) error {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return apperror.NewBadRequest("a picture needs an image")
	}
	if s.media == nil || s.mapLookup == nil {
		return apperror.NewInternal(errImageWiring)
	}
	campaignID, err := s.mapLookup(ctx, mapID)
	if err != nil {
		return err
	}
	ok, err := s.media.ImageInCampaign(ctx, mediaID, campaignID)
	if err != nil {
		return err
	}
	if !ok {
		// Same answer for "no such file" and "another campaign's file" so the
		// response cannot be used to probe for media ids.
		return apperror.NewBadRequest("image must be a picture from this campaign's media")
	}
	return nil
}

// mediaIDPattern finds anything shaped like a media id. The media route
// matches ids case-insensitively, so the pattern does too.
var mediaIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// tokenImageMediaIDs lists every media id a token picture path could make a
// viewer's browser fetch from Chronicle, in its raw and percent-decoded forms,
// so no spelling of "/media/<id>" escapes the campaign check. A data: picture
// fetches nothing.
func tokenImageMediaIDs(p string) []string {
	p = trimURLControlChars(p)
	if len(p) >= 5 && strings.EqualFold(p[:5], "data:") {
		return nil
	}
	ids := mediaIDPattern.FindAllString(p, -1)
	if dec, err := url.PathUnescape(p); err == nil && dec != p {
		ids = append(ids, mediaIDPattern.FindAllString(dec, -1)...)
	}
	return ids
}

// verifyTokenImageMedia refuses a token picture that names a media file that
// is not an image of the map's campaign. A path naming no media file (a
// bundled icon, a Foundry asset path) needs no lookup.
func (s *drawingService) verifyTokenImageMedia(ctx context.Context, mapID string, path *string) error {
	if path == nil {
		return nil
	}
	ids := tokenImageMediaIDs(*path)
	if len(ids) == 0 {
		return nil
	}
	if s.media == nil || s.mapLookup == nil {
		return apperror.NewInternal(errImageWiring)
	}
	campaignID, err := s.mapLookup(ctx, mapID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		ok, err := s.media.ImageInCampaign(ctx, id, campaignID)
		if err != nil {
			return err
		}
		if !ok {
			return apperror.NewBadRequest("image_path must be a picture from this campaign's media")
		}
	}
	return nil
}

// rejectImageFields keeps image-only columns off every other drawing type.
func rejectImageFields(imageID *string, crop json.RawMessage) error {
	if (imageID != nil && *imageID != "") || len(crop) > 0 {
		return apperror.NewBadRequest("image_id and crop are only for pictures")
	}
	return nil
}
