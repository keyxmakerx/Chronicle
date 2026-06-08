// registry.go — the dynamic widget-type registry (C-WIDGET-BINDING-P1-SPINE).
// Widget types register their behavior here DECLARATIVELY instead of being
// hardcoded into each block's renderer. The registry is the app-code guard for
// the widget_type namespace (a registered slug == a valid widget_type value),
// preserving modularity: a new widget type plugs in by registering, with no
// schema change.
package widgetbindings

import (
	"context"
	"errors"
	"sync"
)

// ErrNotImplemented is returned by P1 stubs (ListInstances/CreateInstance) that
// P4 (the create-or-pick UI) fills in.
var ErrNotImplemented = errors.New("widgetbindings: not implemented in this wave")

// InstanceRef is a lightweight instance summary for the picker.
type InstanceRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

// CreateInput is the generic payload for WidgetType.CreateInstance
// (C-WIDGET-BINDING-P4a). The binding HTTP handler collects form fields into
// it (so the handler stays widget-type-agnostic) and each WidgetType
// type-asserts it and reads what it needs — calendar uses Name; Raw carries
// any extra fields a future type wants without changing this boundary.
type CreateInput struct {
	Name string
	Raw  map[string]string
}

// WidgetType declares one widget type's behavior to the framework. Owning
// plugins (calendar in P1; maps/timeline later) implement it and register an
// instance at startup; the binding Service drives resolution through it without
// importing the plugin.
//
// P1 uses InstanceExists + DefaultInstance (+ Slug). ListInstances and
// CreateInstance are part of the contract for the P4 picker and may return
// ErrNotImplemented until then.
type WidgetType interface {
	// Slug is the persisted widget_type discriminator (e.g. "calendar").
	Slug() string

	// InstanceExists reports whether instanceID is a real instance of this
	// widget type that belongs to campaignID. It is BOTH the orphan guard and
	// the campaign-scope security check (the instance fetch is the #1 leakage
	// vector) — it must return false for a missing OR cross-campaign instance.
	InstanceExists(ctx context.Context, campaignID, instanceID string) (bool, error)

	// DefaultInstance returns the unbound default instance for a host — i.e.
	// today's behavior (calendar → the campaign's default calendar). ok=false
	// when there is no default. MUST equal pre-framework behavior so unbound
	// hosts render identically (zero churn for #411–#420).
	DefaultInstance(ctx context.Context, host HostRef) (instanceID string, ok bool, err error)

	// ListInstances powers the P4 picker (campaign's instances of this type).
	ListInstances(ctx context.Context, campaignID string, role int) ([]InstanceRef, error)
	// CreateInstance powers the P4 "create new" flow.
	CreateInstance(ctx context.Context, campaignID string, input any) (instanceID string, err error)
}

// Registry holds the registered widget types. Safe for concurrent reads after
// startup (writes happen during app wiring).
type Registry struct {
	mu    sync.RWMutex
	types map[string]WidgetType
}

// NewRegistry creates an empty widget-type registry.
func NewRegistry() *Registry {
	return &Registry{types: make(map[string]WidgetType)}
}

// Register adds (or replaces) a widget type by its slug.
func (r *Registry) Register(wt WidgetType) {
	if wt == nil || wt.Slug() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types[wt.Slug()] = wt
}

// Get returns the widget type for a slug.
func (r *Registry) Get(slug string) (WidgetType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	wt, ok := r.types[slug]
	return wt, ok
}

// IsValidWidgetType reports whether slug names a registered widget type. This
// is the app-code namespace guard (no DB enum).
func (r *Registry) IsValidWidgetType(slug string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[slug]
	return ok
}

// Slugs returns the registered widget-type slugs (stable-ish; for diagnostics).
func (r *Registry) Slugs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.types))
	for s := range r.types {
		out = append(out, s)
	}
	return out
}
