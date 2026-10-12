package campaigns

import (
	"context"
	"sync"

	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// TileSizeResolver reports the sizes a block type can take in a tile section.
// The block registry lives in the entities plugin, which imports this one, so
// the dashboard reaches it through this interface, wired at startup.
type TileSizeResolver interface {
	TileSizesFor(ctx context.Context, campaignID, blockType string, config map[string]any) []string
}

var (
	tileResolverMu sync.RWMutex
	tileResolver   TileSizeResolver
)

// SetTileSizeResolver wires the registry that answers tile sizes. Until it is
// set (unit tests), no block type can be a tile and a dashboard tile section
// draws its not-available notice.
func SetTileSizeResolver(r TileSizeResolver) {
	tileResolverMu.Lock()
	defer tileResolverMu.Unlock()
	tileResolver = r
}

// tileSizes returns the sizes blockType can take as a tile in a campaign.
func tileSizes(ctx context.Context, campaignID, blockType string, config map[string]any) []string {
	tileResolverMu.RLock()
	r := tileResolver
	tileResolverMu.RUnlock()
	if r == nil {
		return nil
	}
	return r.TileSizesFor(ctx, campaignID, blockType, config)
}

// tileSizesFunc adapts tileSizes to tilegrid's lookup for one campaign.
func tileSizesFunc(ctx context.Context, campaignID string) tilegrid.SizesFunc {
	return func(blockType string, config map[string]any) []string {
		return tileSizes(ctx, campaignID, blockType, config)
	}
}

// DashboardTileBlock turns a tile into the block the dashboard block switch
// draws, so a tile and a column block share one renderer.
func DashboardTileBlock(t tilegrid.Tile) DashboardBlock {
	return DashboardBlock{ID: t.ID, Type: t.Type, Config: t.Config}
}

// ResolveDashboardTile gives the size a dashboard tile is drawn at, falling
// back to its type's first allowed size when the stored one is no longer
// allowed. ok is false when the type allows no size.
func ResolveDashboardTile(ctx context.Context, campaignID string, t tilegrid.Tile) (w, h int, ok bool) {
	_, w, h, ok = tilegrid.Resolve(t.Size, tileSizes(ctx, campaignID, t.Type, t.Config))
	return w, h, ok
}

// TileVisibleTo reports whether a tile's config.visibility lets the viewer see
// it. It is the same rule entity pages apply to a block ("dm_only" shows only
// to those who can read DM-only content).
func TileVisibleTo(config map[string]any, cc *CampaignContext) bool {
	v, _ := config["visibility"].(string)
	return v != "dm_only" || cc.VisibilityRole() >= int(RoleOwner)
}
