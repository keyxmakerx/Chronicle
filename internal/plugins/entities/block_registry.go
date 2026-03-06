// block_registry.go provides a self-registering system for entity page block types.
// Plugins register their block metadata and renderers at startup so that
// validation, rendering, and the template-editor palette are all driven
// by a single source of truth.
package entities

import (
	"context"
	"sync"

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// BlockMeta describes a block type for the template editor UI and validation.
type BlockMeta struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Icon        string `json:"icon"`                    // FontAwesome class (e.g., "fa-heading").
	Description string `json:"description"`
	Addon       string `json:"addon,omitempty"`          // Required addon slug; empty = always available.
	Container   bool   `json:"container,omitempty"`      // True for layout containers (two_column, tabs, etc.).
	WidgetSlug  string `json:"widget_slug,omitempty"`    // For ext_widget blocks: the extension widget slug.
}

// BlockRenderContext holds the data available to every block renderer.
type BlockRenderContext struct {
	Block      TemplateBlock
	CC         *campaigns.CampaignContext
	Entity     *Entity
	EntityType *EntityType
	CSRFToken  string
}

// BlockRenderer returns a templ.Component for the given block context.
type BlockRenderer func(ctx BlockRenderContext) templ.Component

// registeredBlock pairs metadata with the renderer.
type registeredBlock struct {
	meta     BlockMeta
	renderer BlockRenderer
}

// BlockRegistry maps block type names to metadata and renderers.
// Safe for concurrent reads after startup (writes happen only during init).
type BlockRegistry struct {
	mu      sync.RWMutex
	entries map[string]registeredBlock
	order   []string // insertion order for stable palette ordering
}

// NewBlockRegistry creates an empty registry.
func NewBlockRegistry() *BlockRegistry {
	return &BlockRegistry{
		entries: make(map[string]registeredBlock),
	}
}

// Register adds a block type to the registry. If a type with the same name
// already exists it is silently overwritten (allows plugin overrides).
func (r *BlockRegistry) Register(meta BlockMeta, renderer BlockRenderer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[meta.Type]; !exists {
		r.order = append(r.order, meta.Type)
	}
	r.entries[meta.Type] = registeredBlock{meta: meta, renderer: renderer}
}

// IsValid returns true if blockType is a registered block type.
func (r *BlockRegistry) IsValid(blockType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[blockType]
	return ok
}

// Types returns all registered block metadata in registration order.
func (r *BlockRegistry) Types() []BlockMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]BlockMeta, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.entries[name].meta)
	}
	return result
}

// AddonChecker tests whether an addon slug is enabled for a campaign.
// Matches the existing AddonChecker interface in handler.go.
type blockAddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
}

// TypesForCampaign returns block metadata filtered by which addons are
// enabled for the given campaign. Blocks with no addon requirement are
// always included.
func (r *BlockRegistry) TypesForCampaign(ctx context.Context, campaignID string, checker blockAddonChecker) []BlockMeta {
	all := r.Types()
	if checker == nil {
		return all
	}

	result := make([]BlockMeta, 0, len(all))
	for _, meta := range all {
		if meta.Addon == "" {
			result = append(result, meta)
			continue
		}
		enabled, err := checker.IsEnabledForCampaign(ctx, campaignID, meta.Addon)
		if err == nil && enabled {
			result = append(result, meta)
		}
	}
	return result
}

// Render dispatches to the registered renderer for the block type.
// Returns nil if the block type is not registered.
func (r *BlockRegistry) Render(ctx BlockRenderContext) templ.Component {
	r.mu.RLock()
	entry, ok := r.entries[ctx.Block.Type]
	r.mu.RUnlock()

	if !ok {
		return nil
	}
	return entry.renderer(ctx)
}

// --- Block config helpers (used by plugin renderers) ---

// BlockConfigString extracts a string value from a block config map.
// Returns empty string if the key is missing or not a string.
func BlockConfigString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	v, ok := config[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// BlockConfigLimit extracts a numeric limit from a block config map.
// Returns defaultLimit if the key is missing or not a positive number.
func BlockConfigLimit(config map[string]any, key string, defaultLimit int) int {
	if config == nil {
		return defaultLimit
	}
	v, ok := config[key]
	if !ok {
		return defaultLimit
	}
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int(n)
		}
	case int:
		if n > 0 {
			return n
		}
	}
	return defaultLimit
}

// --- Package-level registry (set during app startup) ---

var (
	globalRegistryMu sync.RWMutex
	globalRegistry   *BlockRegistry
)

// SetGlobalBlockRegistry sets the package-level block registry.
// Called once during app startup after all plugins have registered.
func SetGlobalBlockRegistry(reg *BlockRegistry) {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	globalRegistry = reg
}

// GetGlobalBlockRegistry returns the package-level block registry.
func GetGlobalBlockRegistry() *BlockRegistry {
	globalRegistryMu.RLock()
	defer globalRegistryMu.RUnlock()
	return globalRegistry
}

// RenderBlock dispatches to the global registry. Called by templ components.
// Returns an empty component if the block type is unregistered.
func RenderBlock(block TemplateBlock, cc *campaigns.CampaignContext, entity *Entity, entityType *EntityType, csrfToken string) templ.Component {
	reg := GetGlobalBlockRegistry()
	if reg == nil {
		return templ.NopComponent
	}
	ctx := BlockRenderContext{
		Block:      block,
		CC:         cc,
		Entity:     entity,
		EntityType: entityType,
		CSRFToken:  csrfToken,
	}
	comp := reg.Render(ctx)
	if comp == nil {
		return templ.NopComponent
	}
	return comp
}
