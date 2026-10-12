package tilegrid

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// KindTiles is the Kind of a row that holds tiles instead of columns. An
// empty Kind is an ordinary row, so layouts saved before tile sections
// existed need no migration.
const KindTiles = "tiles"

// Grid limits. The grid is 4 columns wide (2 on narrow screens); height is
// counted in rows of the grid's minimum row height. The height cap keeps a
// hand-written size from asking for a tile taller than any page.
const (
	MaxWidth           = 4
	MaxHeight          = 6
	MaxTilesPerSection = 40
)

// Tile is one widget in a tile section. Placement is order only: the grid
// packs tiles in the order they are stored, so there is no x/y to go stale
// when a neighbour changes size.
type Tile struct {
	ID string `json:"id"`
	// Type is a block-registry type.
	Type string `json:"type"`
	// Size is "WxH", for example "2x1". It must be one of the sizes the type
	// declares (BlockMeta.TileSizes).
	Size string `json:"size"`
	// Config is the same per-type config a block in a column carries,
	// including config.visibility.
	Config map[string]any `json:"config,omitempty"`
}

// ParseSize reads a canonical "WxH" size. Only the canonical spelling is
// accepted (no spaces, no leading zeros, lowercase x), so a stored size can be
// compared with a declared one as plain strings.
func ParseSize(s string) (w, h int, ok bool) {
	ws, hs, found := strings.Cut(s, "x")
	if !found {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(ws)
	h, err2 := strconv.Atoi(hs)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if w < 1 || w > MaxWidth || h < 1 || h > MaxHeight {
		return 0, 0, false
	}
	if strconv.Itoa(w) != ws || strconv.Itoa(h) != hs {
		return 0, 0, false
	}
	return w, h, true
}

// Resolve picks the size a tile is drawn at. A stored size that the type still
// allows wins. Otherwise the type's first allowed size is used, so shrinking a
// widget's declared sizes never breaks a saved page. ok is false when the
// type allows no usable size at all; the caller then shows the same notice a
// missing block gets rather than guessing.
func Resolve(stored string, allowed []string) (size string, w, h int, ok bool) {
	for _, a := range allowed {
		if a == stored {
			if w, h, valid := ParseSize(a); valid {
				return a, w, h, true
			}
		}
	}
	for _, a := range allowed {
		if w, h, valid := ParseSize(a); valid {
			return a, w, h, true
		}
	}
	return "", 0, 0, false
}

// maxDeclaredSizes bounds a widget's declared size list; the grid has only a
// few useful shapes.
const maxDeclaredSizes = 12

// ValidateSizeList checks the tile_sizes a widget declares in its manifest:
// each entry a well-formed size, none repeated. An empty list is valid (the
// widget just cannot be a tile).
func ValidateSizeList(sizes []string) error {
	if len(sizes) > maxDeclaredSizes {
		return fmt.Errorf("too many sizes (%d, max %d)", len(sizes), maxDeclaredSizes)
	}
	seen := make(map[string]bool, len(sizes))
	for _, s := range sizes {
		if _, _, ok := ParseSize(s); !ok {
			return fmt.Errorf("%q is not a size like 2x1 (1 to %d wide, 1 to %d tall)", s, MaxWidth, MaxHeight)
		}
		if seen[s] {
			return fmt.Errorf("%q is listed twice", s)
		}
		seen[s] = true
	}
	return nil
}

// SizesFunc reports the sizes a block type (with the tile's own config, since
// an extension widget's sizes depend on which widget it is) can take as a
// tile. A nil result means the type cannot be a tile.
type SizesFunc func(blockType string, config map[string]any) []string

// ValidateSection checks one tile section on save. rowNum is 1-based, for the
// message. seenIDs is shared with the caller so a tile id cannot repeat a
// block id elsewhere in the layout. sizesFor may be nil (a preset validated
// before a campaign is known); the type and size-membership checks are then
// skipped, as the block-type check is.
//
// Messages are written for the person in the layout editor, which shows them
// verbatim.
func ValidateSection(rowNum int, hasColumns bool, tiles []Tile, seenIDs map[string]bool, sizesFor SizesFunc) error {
	if hasColumns {
		return apperror.NewBadRequest(fmt.Sprintf("Row %d has both columns and tiles. A row is one or the other; split it into two rows.", rowNum))
	}
	if len(tiles) > MaxTilesPerSection {
		return apperror.NewBadRequest(fmt.Sprintf("Row %d has too many tiles (max %d). Move some to another tile section.", rowNum, MaxTilesPerSection))
	}
	for _, t := range tiles {
		if strings.TrimSpace(t.ID) == "" {
			return apperror.NewBadRequest("Internal: a tile is missing its ID. Refresh the page and try again.")
		}
		if seenIDs[t.ID] {
			return apperror.NewBadRequest("This layout contains the same block twice. Refresh the page — the editor may have lost track of a drag.")
		}
		seenIDs[t.ID] = true
		if _, _, ok := ParseSize(t.Size); !ok {
			return apperror.NewBadRequest(fmt.Sprintf("A tile in row %d has an invalid size (%q). Sizes look like 2x1: 1 to %d wide, 1 to %d tall.", rowNum, t.Size, MaxWidth, MaxHeight))
		}
		if sizesFor == nil {
			continue
		}
		allowed := sizesFor(t.Type, t.Config)
		if len(allowed) == 0 {
			return apperror.NewBadRequest(fmt.Sprintf("The %q block can't be shown as a tile. Put it in a column row instead.", t.Type))
		}
		if !contains(allowed, t.Size) {
			return apperror.NewBadRequest(fmt.Sprintf("The %q block can't be %s as a tile. Allowed sizes: %s.", t.Type, t.Size, strings.Join(allowed, ", ")))
		}
	}
	return nil
}

// ValidateKind rejects a row kind this version does not know, so a typo or a
// newer client's kind fails the save instead of being stored as a row nothing
// can draw.
func ValidateKind(rowNum int, kind string) error {
	if kind != "" && kind != KindTiles {
		return apperror.NewBadRequest(fmt.Sprintf("Row %d has an unknown kind (%q).", rowNum, kind))
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
