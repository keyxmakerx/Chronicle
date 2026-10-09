package entities

import (
	"sync"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CategoryBlockRenderer draws one extra block on a category dashboard. It gets
// the viewer's campaign context and the category, and must decide what the
// viewer may see itself.
type CategoryBlockRenderer func(cc *campaigns.CampaignContext, et *EntityType) templ.Component

// extraCategoryBlocks holds category dashboard blocks that other plugins own.
// The built-in blocks stay in CategoryBlockSwitch; a plugin cannot be imported
// from here (plugin isolation), so it registers its block at startup instead.
var extraCategoryBlocks = struct {
	sync.RWMutex
	byType map[string]CategoryBlockRenderer
}{byType: map[string]CategoryBlockRenderer{}}

// RegisterCategoryBlock adds a category dashboard block owned by another
// plugin. Registering the same type again replaces it.
func RegisterCategoryBlock(blockType string, r CategoryBlockRenderer) {
	extraCategoryBlocks.Lock()
	defer extraCategoryBlocks.Unlock()
	extraCategoryBlocks.byType[blockType] = r
}

// IsExtraCategoryBlock reports whether blockType was registered through
// RegisterCategoryBlock. Saving a category layout accepts these in addition to
// the dashboard block set, so a registered block survives a save.
func IsExtraCategoryBlock(blockType string) bool {
	extraCategoryBlocks.RLock()
	defer extraCategoryBlocks.RUnlock()
	_, ok := extraCategoryBlocks.byType[blockType]
	return ok
}

// renderExtraCategoryBlock draws a registered block, or nothing for an
// unknown type (an old layout naming a block that no longer exists).
func renderExtraCategoryBlock(cc *campaigns.CampaignContext, et *EntityType, blockType string) templ.Component {
	extraCategoryBlocks.RLock()
	r := extraCategoryBlocks.byType[blockType]
	extraCategoryBlocks.RUnlock()
	if r == nil {
		return templ.NopComponent
	}
	return r(cc, et)
}
