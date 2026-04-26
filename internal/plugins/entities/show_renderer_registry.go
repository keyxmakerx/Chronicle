// show_renderer_registry.go — slug-keyed extension point for entity-show
// rendering (CH4). Layered above the existing BlockRegistry: when an
// entity's entity_type slug has a registered renderer, that renderer
// owns the page contents; otherwise the existing layout-block dispatch
// runs unchanged. Block dispatch IS the fallback — there is no new
// fallback path to maintain.
//
// Audience: system package authors (Draw Steel, future D&D 5.5e, etc.)
// who want to render character / monster / item entities with system-
// specific layout that the generic block system can't express. The
// host ships zero character-specific renderers; system packages own
// the full vertical slice. See docs/system-package-rendering.md for
// the external-facing contract.
//
// V1 lifecycle: register at startup (during RegisterRoutes, mirroring
// BlockRegistry), no live mutation, restart-required for system
// install/disable. The mutex is present for race-detector cleanliness
// during startup registration, not for live-reload support.

package entities

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityShowRenderContext is everything a registered renderer receives
// when handling an entity-show request. The fields mirror the args of
// the EntityShowPage templ exactly — anything the block-dispatch
// fallback can read, a registered renderer can read too. Adding a new
// field here when the templ signature grows is the maintenance cost
// of feature parity; the alternative (renderer with fewer inputs than
// fallback) silently degrades capability over time.
type EntityShowRenderContext struct {
	CC             *campaigns.CampaignContext
	Entity         *Entity
	EntityType     *EntityType
	Ancestors      []Entity
	Children       []Entity
	ShowAttributes bool
	ShowCalendar   bool
	CSRFToken      string
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
// slug it ships (drawsteel-character, drawsteel-monster, etc.).
//
// The mutex makes Register / Lookup safe under concurrent access. In
// V1 every Register call happens during startup (single goroutine,
// before HTTP serves) and every Lookup happens at request time, so
// the lock is uncontended in practice — the cost is paid for race-
// detector cleanliness and the option to relax the "restart-required"
// rule later without reworking the registry shape.
type EntityShowRendererRegistry struct {
	mu        sync.RWMutex
	renderers map[string]EntityShowRenderer
}

// NewEntityShowRendererRegistry returns an empty registry ready for
// startup-time registration calls.
func NewEntityShowRendererRegistry() *EntityShowRendererRegistry {
	return &EntityShowRendererRegistry{
		renderers: map[string]EntityShowRenderer{},
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

// globalEntityShowRendererRegistry is the singleton consumed by
// show.templ via lookupEntityShowRenderer. Set during RegisterRoutes
// before HTTP serves; nil before that.
var globalEntityShowRendererRegistry *EntityShowRendererRegistry

// SetGlobalEntityShowRendererRegistry installs the registry that
// show.templ will consult at request time. Called from
// internal/app/routes.go alongside SetGlobalBlockRegistry.
func SetGlobalEntityShowRendererRegistry(r *EntityShowRendererRegistry) {
	globalEntityShowRendererRegistry = r
}

// GetGlobalEntityShowRendererRegistry returns the installed registry.
// May be nil if SetGlobalEntityShowRendererRegistry hasn't been called
// yet (only possible during boot, before user requests can reach
// rendering code). All read-time callers must nil-check.
func GetGlobalEntityShowRendererRegistry() *EntityShowRendererRegistry {
	return globalEntityShowRendererRegistry
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
// shipping any Go code. The host's CH4.5 auto-registration walks the
// manifest at boot, calls this helper for each entry, and registers the
// returned function under the entry's slug.
//
// The widget slug is captured by value at registration time, so changing
// the manifest after install (without a restart) does not affect already-
// registered renderers — matching the registry's V1 restart-required
// lifecycle.
func MakeWidgetMountRenderer(widget string) EntityShowRenderer {
	return func(ctx EntityShowRenderContext) templ.Component {
		entityID := ""
		if ctx.Entity != nil {
			entityID = ctx.Entity.ID
		}
		campaignID := ""
		if ctx.CC != nil && ctx.CC.Campaign != nil {
			campaignID = ctx.CC.Campaign.ID
		}
		return widgetMount{widget: widget, entityID: entityID, campaignID: campaignID}
	}
}

// widgetMount is a tiny templ.Component that renders one boot.js mount
// point. Implemented directly (rather than via a generated templ file)
// because it's three attributes on a single div — generating a .templ
// for it would obscure more than it clarifies, and keeping the helper
// here lets system-package auto-registration avoid a circular import on
// any rendering scaffold.
type widgetMount struct {
	widget     string
	entityID   string
	campaignID string
}

func (w widgetMount) Render(_ context.Context, out io.Writer) error {
	_, err := fmt.Fprintf(
		out,
		`<div data-widget="%s" data-entity-id="%s" data-campaign-id="%s"></div>`,
		templ.EscapeString(w.widget),
		templ.EscapeString(w.entityID),
		templ.EscapeString(w.campaignID),
	)
	return err
}

// lookupEntityShowRenderer is the templ-callable helper that
// EntityShowPage uses to dispatch. Returns the registered renderer
// applied to the supplied context (so the templ caller gets a
// templ.Component to embed), or nil if no renderer is registered for
// the entity type's slug — the templ branches on nil to fall through
// to the existing block-dispatch path.
//
// Centralised here so the dispatch logic (registry nil-check, slug
// extraction, miss handling) lives in Go code rather than templ
// conditionals. Keeps show.templ readable.
func lookupEntityShowRenderer(ctx EntityShowRenderContext) templ.Component {
	if ctx.EntityType == nil {
		return nil
	}
	reg := globalEntityShowRendererRegistry
	if reg == nil {
		return nil
	}
	rend, ok := reg.Lookup(ctx.EntityType.Slug)
	if !ok {
		return nil
	}
	return rend(ctx)
}
