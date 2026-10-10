package maps

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// linkThumbSize is the media thumbnail edge used for the small map pictures in
// the "Opens map" chooser and the tree of linked maps.
const linkThumbSize = "300"

// mapThumbSrc is mapImageSrc for a small picture: the player copy when the
// viewer's version of the map carries one, else a signed thumbnail of the
// original, or "" when there is no picture. Like mapImageSrc it only ever reads
// the viewer's version, so it cannot build an address for a withheld original.
func mapThumbSrc(ctx context.Context, m *Map) string {
	if m == nil {
		return ""
	}
	return thumbSrc(ctx, m.ImageID, m.PlayerImageURL)
}

func thumbSrc(ctx context.Context, imageID *string, playerURL string) string {
	if playerURL != "" {
		return playerURL
	}
	if imageID == nil || *imageID == "" {
		return ""
	}
	return layouts.MediaThumbURL(ctx, *imageID, linkThumbSize)
}

// fillLinkThumbs sets each tree node's ThumbURL from the viewer's version of
// its picture, which the service carried on the node.
func fillLinkThumbs(ctx context.Context, nodes []LinkNode) {
	for i := range nodes {
		id, player := nodes[i].ThumbSource()
		var idp *string
		if id != "" {
			idp = &id
		}
		nodes[i].ThumbURL = thumbSrc(ctx, idp, player)
		fillLinkThumbs(ctx, nodes[i].Children)
	}
}
