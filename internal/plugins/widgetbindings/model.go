// Package widgetbindings is the foundation for the widget-binding framework
// (C-WIDGET-BINDING-P1-SPINE): a generic, host-agnostic mapping of
// host (entity / entity-type / dashboard) ↔ widget-type ↔ data-instance.
//
// It is the dynamic, declarative replacement for the per-widget hardcoding
// that `entity_calendar`/`entity_worldstate`/`map_editor` grew independently
// (the `entities.map_id` column is the same idea, hardcoded for one type).
// Widget types register their behavior in the Registry; the Service resolves,
// for a given host, which data-instance a widget renders, following a
// precedence chain (host's own binding → entity-type template → default).
//
// Design rationale (see ADR 2026-06-07-widget-binding-polymorphic-fk-free in
// .ai/decisions.md): the binding table is POLYMORPHIC and FK-FREE on both
// host_id and instance_id, so it can live in a plugin without tripping the
// core-before-plugin migration-ordering rule. Referential integrity is the
// app layer's job (MariaDB has no RLS backstop) — enforced by an AND of three
// mechanisms: per-plugin delete hooks, an always-on render-time orphan guard,
// and a periodic campaign integrity sweep.
package widgetbindings

import "time"

// Host types. These are PERSISTED polymorphic discriminators — an immutable,
// append-only namespace. Renaming one orphans every stored binding, so they
// are validated in app code (not a DB enum) and only ever added to.
const (
	HostTypeEntity     = "entity"      // a single entity instance hosts the widget
	HostTypeEntityType = "entity_type" // an entity-type template (inherited by its entities)
	HostTypeDashboard  = "dashboard"   // a campaign dashboard surface (host_id e.g. "<campaignID>:player")
)

// validHostTypes is the app-code guard for the host_type namespace (P1 stores
// only `entity` in practice, but all three are representable from day one so
// entity-type + dashboard hosting are later activations, not schema changes).
var validHostTypes = map[string]bool{
	HostTypeEntity:     true,
	HostTypeEntityType: true,
	HostTypeDashboard:  true,
}

// IsValidHostType reports whether ht is a known host-type discriminator.
func IsValidHostType(ht string) bool { return validHostTypes[ht] }

// Resolution source layers — which rung of the precedence chain won. Returned
// alongside the instance id so the cascade is inspectable, not a black box
// (precedent refinement #2).
const (
	SourceOwn        = "own"         // the host's own binding
	SourceEntityType = "entity_type" // inherited from the entity's type template
	SourceDefault    = "default"     // the widget type's default (today's behavior)
	SourceNone       = "none"        // nothing resolved (no binding, no default)
)

// WidgetBinding is one host ↔ widget-type ↔ instance row. host_id and
// instance_id are polymorphic strings with NO foreign keys (see package doc).
type WidgetBinding struct {
	ID         string    `json:"id"`
	CampaignID string    `json:"campaign_id"`
	HostType   string    `json:"host_type"`
	HostID     string    `json:"host_id"`
	WidgetType string    `json:"widget_type"`
	InstanceID string    `json:"instance_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// HostRef identifies the thing a widget is rendered on, for resolution. It is
// plain strings so this foundation plugin imports no other plugin (entity /
// campaign code depends on widgetbindings, never the reverse).
//
// EntityTypeID enables the inheritance path: for an `entity` host it carries
// the entity's type id so Resolve can fall back to the entity-type's template
// binding. Empty when not applicable.
type HostRef struct {
	CampaignID   string
	Type         string // one of HostType*
	ID           string // entity id / entity-type id / dashboard key
	EntityTypeID string // entity hosts only; "" otherwise
}

// Resolution is the outcome of Resolve: the winning instance id + which layer
// it came from (+ the widget type, for convenience).
type Resolution struct {
	InstanceID string
	Source     string // one of Source*
	WidgetType string
}

// Resolved reports whether resolution produced a usable instance.
func (r Resolution) Resolved() bool { return r.Source != SourceNone && r.InstanceID != "" }
