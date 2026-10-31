package entities

import (
	"sort"
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

// extraCategoryBlockTypes lists the types registered through
// RegisterCategoryBlock, sorted.
func extraCategoryBlockTypes() []string {
	extraCategoryBlocks.RLock()
	defer extraCategoryBlocks.RUnlock()
	out := make([]string, 0, len(extraCategoryBlocks.byType))
	for t := range extraCategoryBlocks.byType {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// categoryLeads maps a category's type slug to a registered block drawn under
// the heading of that category's built-in landing page, so a plugin can give
// its own kind of page a better front door without the GM arranging it.
var categoryLeads = struct {
	sync.RWMutex
	bySlug map[string]string
}{bySlug: map[string]string{}}

// RegisterCategoryLead draws blockType (registered through
// RegisterCategoryBlock) under the heading of every category whose type slug
// is typeSlug, while that category has no arranged home of its own.
func RegisterCategoryLead(typeSlug, blockType string) {
	categoryLeads.Lock()
	defer categoryLeads.Unlock()
	categoryLeads.bySlug[typeSlug] = blockType
}

// renderCategoryLead draws the lead block for et, or nothing.
func renderCategoryLead(cc *campaigns.CampaignContext, et *EntityType) templ.Component {
	categoryLeads.RLock()
	bt := categoryLeads.bySlug[et.Slug]
	categoryLeads.RUnlock()
	if bt == "" {
		return templ.NopComponent
	}
	return renderExtraCategoryBlock(cc, et, bt)
}
