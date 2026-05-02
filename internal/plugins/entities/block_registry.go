// block_registry.go provides a self-registering system for entity page block types.
// Plugins register their block metadata and renderers at startup so that
// validation, rendering, and the template-editor palette are all driven
// by a single source of truth.
package entities

import (
	"context"
	"log/slog"
	"sync"

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// BlockMeta describes a block type for the layout editor UI and validation.
// Both template (entity page) and dashboard block types are registered here
// so a single source of truth drives palette, validation, and config dialogs.
type BlockMeta struct {
	Type         string            `json:"type"`
	Label        string            `json:"label"`
	Icon         string            `json:"icon"`                      // FontAwesome class (e.g., "fa-heading").
	Description  string            `json:"description"`
	Addon        string            `json:"addon,omitempty"`           // Required addon slug; empty = always available.
	Container    bool              `json:"container,omitempty"`       // True for layout containers (two_column, tabs, etc.).
	WidgetSlug   string            `json:"widget_slug,omitempty"`     // For ext_widget blocks: the extension widget slug.
	Contexts     []string          `json:"contexts,omitempty"`        // Editor contexts: "dashboard", "template". Empty = all.
	ConfigFields []ConfigFieldMeta `json:"config_fields,omitempty"`   // Declarative config schema for the editor dialog.
}

// ConfigFieldMeta describes a single configurable field for a block type.
// The layout editor auto-generates a config dialog from these declarations.
type ConfigFieldMeta struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`              // "number", "text", "textarea", "select", "entity_type", "map"
	Min     *int     `json:"min,omitempty"`      // For "number" type.
	Max     *int     `json:"max,omitempty"`      // For "number" type.
	Default any      `json:"default,omitempty"`
	Options []Option `json:"options,omitempty"`  // For "select" type.
}

// Option is a label+value pair for select-type config fields.
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
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
	mu           sync.RWMutex
	entries      map[string]registeredBlock
	order        []string // insertion order for stable palette ordering
	addonChecker blockAddonChecker
}

// NewBlockRegistry creates an empty registry.
func NewBlockRegistry() *BlockRegistry {
	return &BlockRegistry{
		entries: make(map[string]registeredBlock),
	}
}

// SetAddonChecker sets the addon checker used by Render() to skip blocks
// whose addon is disabled. Must be called after addon service is initialized.
func (r *BlockRegistry) SetAddonChecker(ac blockAddonChecker) {
	r.addonChecker = ac
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

// TypesForCampaignAndContext returns block metadata filtered by both addon
// availability and editor context. Pass an empty editorCtx to skip context
// filtering (returns all blocks for the campaign, same as TypesForCampaign).
func (r *BlockRegistry) TypesForCampaignAndContext(ctx context.Context, campaignID string, checker blockAddonChecker, editorCtx string) []BlockMeta {
	all := r.Types()
	result := make([]BlockMeta, 0, len(all))
	for _, meta := range all {
		// Filter by context if specified.
		if editorCtx != "" && len(meta.Contexts) > 0 {
			found := false
			for _, c := range meta.Contexts {
				if c == editorCtx {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// Filter by addon.
		if meta.Addon != "" && checker != nil {
			enabled, err := checker.IsEnabledForCampaign(ctx, campaignID, meta.Addon)
			if err != nil || !enabled {
				continue
			}
		}
		result = append(result, meta)
	}
	return result
}

// IsValidForContext returns true if blockType is registered and belongs to the
// given editor context (or has no context restriction). Pass empty editorCtx
// to skip the context check.
func (r *BlockRegistry) IsValidForContext(blockType string, editorCtx string) bool {
	r.mu.RLock()
	entry, ok := r.entries[blockType]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	if editorCtx == "" || len(entry.meta.Contexts) == 0 {
		return true
	}
	for _, c := range entry.meta.Contexts {
		if c == editorCtx {
			return true
		}
	}
	return false
}

// Render dispatches to the registered renderer for the block type.
// Returns nil if the block type is not registered or if its addon is disabled.
func (r *BlockRegistry) Render(goCtx context.Context, ctx BlockRenderContext) templ.Component {
	r.mu.RLock()
	entry, ok := r.entries[ctx.Block.Type]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	// Skip blocks whose addon is disabled for this campaign. Diagnostic
	// logging on both branches: a Debug line on the silent-skip path
	// (so an operator can flip to debug level and immediately see why
	// a block isn't rendering) and a Warn line on the checker-error
	// path (previously swallowed via `if err == nil`, which masked any
	// DB outage hitting the addon-gate query).
	if entry.meta.Addon != "" && r.addonChecker != nil && ctx.CC != nil {
		enabled, err := r.addonChecker.IsEnabledForCampaign(goCtx, ctx.CC.Campaign.ID, entry.meta.Addon)
		if err != nil {
			slog.Warn("block_registry: addon-gate check errored; rendering block anyway",
				slog.String("block_type", entry.meta.Type),
				slog.String("addon_slug", entry.meta.Addon),
				slog.String("campaign_id", ctx.CC.Campaign.ID),
				slog.Any("error", err),
			)
		} else if !enabled {
			slog.Debug("block_registry: skipping block, addon disabled for campaign",
				slog.String("block_type", entry.meta.Type),
				slog.String("addon_slug", entry.meta.Addon),
				slog.String("campaign_id", ctx.CC.Campaign.ID),
			)
			return nil
		}
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

// BlockConfigInt extracts an integer value from a block config map.
// Returns defaultVal if the key is missing or not a number.
func BlockConfigInt(config map[string]any, key string, defaultVal int) int {
	if config == nil {
		return defaultVal
	}
	v, ok := config[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return defaultVal
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
// Returns an empty component if the block type is unregistered or its addon
// is disabled. The goCtx is the request context from the templ render call.
func RenderBlock(goCtx context.Context, block TemplateBlock, cc *campaigns.CampaignContext, entity *Entity, entityType *EntityType, csrfToken string) templ.Component {
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
	comp := reg.Render(goCtx, ctx)
	if comp == nil {
		return templ.NopComponent
	}
	return comp
}
