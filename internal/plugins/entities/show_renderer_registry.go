// show_renderer_registry.go — slug-keyed extension point for entity-show
// rendering. Layered above BlockRegistry: when an entity's entity_type
// slug has a registered renderer, that renderer owns the page contents;
// otherwise layout-block dispatch runs unchanged.
//
// Audience: system package authors (Draw Steel, D&D 5.5e, etc.) who want
// to render character / monster / item entities with system-specific
// layout the generic block system can't express. See
// docs/system-package-rendering.md for the external-facing contract.
package entities

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityShowRenderContext is everything a registered renderer receives
// when handling an entity-show request. The fields mirror the args of
// the EntityShowPage templ exactly, so a registered renderer has the
// same inputs as the block-dispatch fallback; keep it that way when the
// templ signature grows.
type EntityShowRenderContext struct {
	CC             *campaigns.CampaignContext
	Entity         *Entity
	EntityType     *EntityType
	Ancestors      []Entity
	Children       []Entity
	ShowAttributes bool
	ShowCalendar   bool
	CSRFToken      string
	// UserID is the viewing user's id, used to gate owner-only field
	// visibility. Empty for an anonymous viewer.
	UserID string
	// OwnerName is the claimant's display name, empty when unclaimed or unresolved.
	OwnerName string
}

// EntityShowRenderer renders the inner contents of an entity show page
// when the entity_type slug matches a registered system package. The
// returned component is rendered inside the layouts.App wrapper that
// already provides the page chrome (sidebar, breadcrumb, claim banner);
// the renderer's job is the area that the layout-block iteration would
// otherwise fill.
type EntityShowRenderer func(ctx EntityShowRenderContext) templ.Component

// EntityShowRendererRegistry maps entity_type slugs to renderers. One
// renderer per slug; a system package registers each character-shaped
// slug it ships (drawsteel-character, drawsteel-monster, etc.). The
// mutex makes Register / Lookup safe under concurrent access.
type EntityShowRendererRegistry struct {
	mu        sync.RWMutex
	renderers map[string]EntityShowRenderer
	// presetRenderers binds by entity-type preset_category (e.g. "character") —
	// the system-agnostic seam. A system maps its widget to a PRESET so it
	// renders any type carrying that preset (e.g. the addon's Player Characters
	// category), not just one bespoke slug. Slug wins over preset at lookup.
	presetRenderers map[string]EntityShowRenderer
}

// NewEntityShowRendererRegistry returns an empty registry ready for
// startup-time registration calls.
func NewEntityShowRendererRegistry() *EntityShowRendererRegistry {
	return &EntityShowRendererRegistry{
		renderers:       map[string]EntityShowRenderer{},
		presetRenderers: map[string]EntityShowRenderer{},
	}
}

// Register adds a renderer for the given entity_type slug. A second
// Register call for the same slug REPLACES the prior one — the last
// registration wins. This matches the BlockRegistry's behavior and
// gives system packages a way to override each other deterministically
// if their order is wired explicitly in routes.go. Empty slug is a
// no-op (defensive — silently dropping a typo is better than panicking
// on a configuration mistake at startup).
func (r *EntityShowRendererRegistry) Register(slug string, renderer EntityShowRenderer) {
	if slug == "" || renderer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.renderers[slug] = renderer
}

// Lookup returns the renderer registered for slug, or (nil, false) if
// none. The boolean second return mirrors map-lookup convention so
// callers can branch cleanly on the miss case.
func (r *EntityShowRendererRegistry) Lookup(slug string) (EntityShowRenderer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rend, ok := r.renderers[slug]
	return rend, ok
}

// RegisterByPresetCategory adds a renderer bound to an entity-type
// preset_category. Last registration wins (mirrors Register); empty category is
// a no-op. This is how a system fills a Chronicle-owned category (e.g. the
// addon's player-character type) without owning its slug.
func (r *EntityShowRendererRegistry) RegisterByPresetCategory(category string, renderer EntityShowRenderer) {
	if category == "" || renderer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.presetRenderers[category] = renderer
}

// LookupByPresetCategory returns the renderer registered for a preset_category,
// or (nil, false). Consulted only after a slug miss — slug is more specific.
func (r *EntityShowRendererRegistry) LookupByPresetCategory(category string) (EntityShowRenderer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rend, ok := r.presetRenderers[category]
	return rend, ok
}

// globalEntityShowRendererRegistry is the singleton consumed by
// show.templ via lookupEntityShowRenderer. Installed at boot and
// hot-swapped on package install; an atomic.Pointer keeps that
// race-free against request goroutines reading it concurrently. Nil
// before the first install.
var globalEntityShowRendererRegistry atomic.Pointer[EntityShowRendererRegistry]

// SetGlobalEntityShowRendererRegistry installs (or hot-swaps) the registry that
// show.templ consults at request time. Called from internal/app/routes.go at boot
// and from the package-install hook. Atomic — safe to call while requests read.
func SetGlobalEntityShowRendererRegistry(r *EntityShowRendererRegistry) {
	globalEntityShowRendererRegistry.Store(r)
}

// GetGlobalEntityShowRendererRegistry returns the installed registry.
// May be nil if SetGlobalEntityShowRendererRegistry hasn't been called
// yet (only possible during boot, before user requests can reach
// rendering code). All read-time callers must nil-check.
func GetGlobalEntityShowRendererRegistry() *EntityShowRendererRegistry {
	return globalEntityShowRendererRegistry.Load()
}

// MakeWidgetMountRenderer returns an EntityShowRenderer that emits a single
// <div data-widget="…" data-entity-id="…" data-campaign-id="…"> element.
// The standard boot.js auto-mounter (static/js/boot.js) picks the element
// up at page load (and after htmx:afterSettle) and runs the registered
// widget's init(el, config) against it; the widget owns the rest of the
// page from there.
//
// This is the bridge that lets system-package authors declare a renderer
// in their manifest's `renderers` field — `{slug, widget}` — without
// shipping any Go code. Auto-registration walks the manifest at boot,
// calls this helper for each entry, and registers the returned function
// under the entry's slug.
func MakeWidgetMountRenderer(widget string) EntityShowRenderer {
	return func(ctx EntityShowRenderContext) templ.Component {
		m := widgetMount{widget: widget}
		if ctx.Entity != nil {
			m.entityID = ctx.Entity.ID
			m.visibility = string(ctx.Entity.Visibility)
			// Gates owner-only widget UI (e.g. the Draw Steel sheet's Background
			// box) safely server-side — mirrors the isGM gate below, but for
			// "this viewer is the entity's claimed owner" instead of GM-tier.
			m.isOwner = ctx.Entity.IsOwnedBy(ctx.UserID)
			m.claimed = ctx.Entity.OwnerUserID != nil
			m.claimedName = ctx.OwnerName
		}
		if ctx.CC != nil {
			if ctx.CC.Campaign != nil {
				m.campaignID = ctx.CC.Campaign.ID
			}
			// GM == can see dm_only content (Owner or a DM-grantee). Gates GM-only
			// widget UI (e.g. the Draw Steel sheet's GM Lore box) safely server-side.
			m.isGM = ctx.CC.VisibilityRole() >= int(campaigns.RoleOwner)
			// The same rule the server applies to the identity writes, so a
			// widget never offers an edit the server would refuse.
			m.canEditIdentity = ctx.Entity != nil && CanEditIdentity(ctx.CC.MemberRole, ctx.Entity, ctx.UserID)
			// The image route is Scribe-only; a claimed owner can rename and set
			// identity fields but not replace the picture.
			m.canChangeImage = ctx.CC.MemberRole >= campaigns.RoleScribe
		}
		return m
	}
}

// widgetMount is a tiny templ.Component that renders one boot.js mount
// point, implemented directly rather than via a generated templ file
// since it's a handful of attributes on a single div.
type widgetMount struct {
	widget          string
	entityID        string
	campaignID      string
	isGM            bool   // viewer can see dm_only content (gates GM-only widget UI)
	isOwner         bool   // viewer is this entity's claimed owner (gates owner-only widget UI)
	visibility      string // entity visibility mode (drives a header pill)
	claimed         bool   // someone has claimed this entity
	claimedName     string // the claimant's display name, when known
	canEditIdentity bool   // viewer may edit identity fields and rename (Scribe+ or the claimed owner)
	canChangeImage  bool   // viewer may replace the picture (Scribe+)
}

// Render writes the mount div. data-armory-items is true when the
// items-and-money panel draws on this same page, so the widget can leave its
// own item list out instead of showing two inventories.
func (w widgetMount) Render(ctx context.Context, out io.Writer) error {
	claimedName := w.claimedName
	if w.claimed && claimedName == "" {
		claimedName = "a player"
	}
	_, err := fmt.Fprintf(
		out,
		`<div data-widget="%s" data-entity-id="%s" data-campaign-id="%s" data-is-gm="%t" data-is-owner="%t" data-visibility="%s"`+
			` data-can-edit-identity="%t" data-can-change-image="%t" data-armory-items="%t"`+
			` data-claimed="%t" data-claimed-by-me="%t" data-claimed-name="%s"></div>`,
		templ.EscapeString(w.widget),
		templ.EscapeString(w.entityID),
		templ.EscapeString(w.campaignID),
		w.isGM,
		w.isOwner,
		templ.EscapeString(w.visibility),
		w.canEditIdentity,
		w.canChangeImage,
		autoPagePanel(ctx),
		w.claimed,
		w.claimed && w.isOwner,
		templ.EscapeString(claimedName),
	)
	return err
}

// lookupEntityShowRenderer is the templ-callable helper that
// EntityShowPage uses to dispatch. Returns the registered renderer
// applied to the supplied context, or nil if none is registered for the
// entity type's slug — the templ branches on nil to fall through to
// block-dispatch.
func lookupEntityShowRenderer(ctx EntityShowRenderContext) templ.Component {
	if ctx.EntityType == nil {
		return nil
	}
	reg := globalEntityShowRendererRegistry.Load()
	if reg == nil {
		return nil
	}
	// Slug binding wins (most specific — a system's own bespoke type); fall back
	// to a preset_category binding (the system-agnostic seam) so a system's
	// widget can render the addon's Player Characters category.
	rend, ok := reg.Lookup(ctx.EntityType.Slug)
	if !ok && ctx.EntityType.PresetCategory != nil {
		rend, ok = reg.LookupByPresetCategory(*ctx.EntityType.PresetCategory)
	}
	if !ok {
		return nil
	}
	return rend(ctx)
}

// hasEntityShowRenderer reports whether a game-system renderer owns this
// type's pages in place of the layout, by the same slug-then-category lookup
// lookupEntityShowRenderer uses.
func hasEntityShowRenderer(et *EntityType) bool {
	if et == nil {
		return false
	}
	reg := globalEntityShowRendererRegistry.Load()
	if reg == nil {
		return false
	}
	if _, ok := reg.Lookup(et.Slug); ok {
		return true
	}
	if et.PresetCategory != nil {
		_, ok := reg.LookupByPresetCategory(*et.PresetCategory)
		return ok
	}
	return false
}
