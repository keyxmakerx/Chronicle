// block_host.go — DOM-id helpers for the re-render seam (C-WIDGET-BINDING-P4b).
// A host's widget block lives inside a stable wrapper element so a bind/unbind
// can target it for an in-place HTMX swap; the picker fragment loads into a
// separate slot inside that wrapper.
package widgetbindings

import "sync"

// BlockHostID is the stable DOM id of a host's widget block — the swap target a
// binding mutation replaces (outerHTML), so the block re-renders in place with
// no full reload. The id is derived from the widget type + host id, which is
// unique per rendered block (one widget type per host).
func BlockHostID(widgetType, hostID string) string {
	return "widget-block-" + widgetType + "-" + hostID
}

// pickerSlotID is the DOM id of the inline panel the "Change…" affordance loads
// the picker fragment into (innerHTML). Nested inside the block host, so a
// successful mutation's outerHTML swap of the host also clears the open picker.
func pickerSlotID(widgetType, hostID string) string {
	return "widget-picker-" + widgetType + "-" + hostID
}

// pickerHostType is the affordance's host_type fallback. An unknown or empty
// host type resolves to the entity host the affordance hardcoded before
// C-CALV4-HOST-P3 §5, so a caller that forgets to name one gets the previous
// behaviour rather than a picker query that reads no binding at all.
func pickerHostType(hostType string) string {
	if IsValidHostType(hostType) {
		return hostType
	}
	return HostTypeEntity
}

// --- the container-query opt-in -------------------------------------------

// inlineSizeHosts is the set of widget types whose BlockHost wrapper carries
// `container-type: inline-size`.
//
// WHY AN OPT-IN AND NOT AN UNCONDITIONAL DECLARATION (C-CALV4-HOST-P3 §4, and
// the ⚠ that dispatch attached to it — MEASURED, not speculative).
// `container-type: inline-size` implies `contain: layout style inline-size`,
// and layout containment makes the element a CONTAINING BLOCK FOR FIXED- AND
// ABSOLUTELY-POSITIONED DESCENDANTS. BlockHost wraps all four bound widget
// types, and the maps block renders two `class="hidden fixed inset-0 z-[9999]"`
// modals INSIDE it (internal/plugins/maps/maps.templ:281-282, composed from
// MapEditorBody) — under an unconditional declaration both would be trapped
// inside the 600px map embed instead of covering the viewport. The timeline and
// worldstate blocks are unaffected today, but nothing stops either from growing
// an overlay later.
//
// So the containment is declared BY THE WIDGET TYPE THAT WANTS IT, in that
// plugin's own package, and every other host keeps today's plain box. Written
// once at registration (before any request renders a block) and read on every
// render, hence the mutex rather than a bare map.
var (
	inlineSizeMu    sync.RWMutex
	inlineSizeHosts = map[string]bool{}
)

// DeclareInlineSizeHost marks a widget type whose block sizes itself with CSS
// container queries, so its BlockHost wrapper becomes a measured inline-size
// container. Called from the widget type's constructor at registration.
//
// A block that declares this MUST tolerate being a containing block for its own
// fixed/absolute descendants — see inlineSizeHosts for what that costs.
func DeclareInlineSizeHost(widgetType string) {
	inlineSizeMu.Lock()
	defer inlineSizeMu.Unlock()
	inlineSizeHosts[widgetType] = true
}

// blockHostStyle is the inline style BlockHost carries for widgetType: the
// container declaration for a declared host, empty for everything else.
//
// Inline rather than a stylesheet rule because widgetbindings owns no
// stylesheet, and a rule keyed on a plugin slug (`[data-widget-host="calendar"]`)
// would put a quoted plugin slug outside the owning plugin — the exact line
// tools/check-plugin-isolation.sh exists to catch.
func blockHostStyle(widgetType string) string {
	inlineSizeMu.RLock()
	defer inlineSizeMu.RUnlock()
	if inlineSizeHosts[widgetType] {
		return "container-type:inline-size"
	}
	return ""
}
