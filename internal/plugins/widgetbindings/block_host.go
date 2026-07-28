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
// WHY AN OPT-IN AND NOT AN UNCONDITIONAL DECLARATION (C-CALV4-HOST-P3 §4).
//
// THE DISPATCH'S ⚠, MEASURED AND RETRACTED FIRST. It warned that
// `container-type: inline-size` implies `contain: layout`, which would make
// this wrapper a containing block for fixed- and absolutely-positioned
// descendants — and the maps block renders two
// `class="hidden fixed inset-0 z-[9999]"` modals INSIDE it
// (internal/plugins/maps/maps.templ:281-282, composed from MapEditorBody), so
// both would be trapped inside the 600px embed. It reads correct from the spec
// text. It is not what browsers do. Measured with getBoundingClientRect in
// Chromium 141.0.7390.37, three identical hosts:
//
//	plain host                    fixed (0,0)     absolute (0,0)
//	container-type: inline-size   fixed (0,0)     absolute (0,0)    ← NOT trapped
//	contain: layout               fixed (82,450)  absolute (82,450) ← trapped
//
// `contain: layout` establishes the containing block; `container-type` does
// not. The reproduction is in the slice's evidence set
// (reports/chronicle/screenshots/2026-07-28-c-calv4-host-p3/06-…png).
//
// WHAT THE OPT-IN IS ACTUALLY FOR, then. Exactly one of the four widget types
// BlockHost wraps is sized by container queries; the other three asked for
// nothing and get today's plain box, so this change cannot regress a surface it
// does not own. Containment semantics are also precisely the class of behaviour
// that differs between engines — the spec sentence above is the one that made
// the ⚠ plausible — and the maps modals really are inside this wrapper, so the
// blast radius of getting it wrong is real even where Chromium tolerates it.
// Verifying the other three RENDER paths needs a live client with a database,
// which the slice that wrote this could not run. Flipping this to unconditional
// is one line once someone has.
//
// Written once at registration (before any request renders a block) and read on
// every render, hence the mutex rather than a bare map.
var (
	inlineSizeMu    sync.RWMutex
	inlineSizeHosts = map[string]bool{}
)

// DeclareInlineSizeHost marks a widget type whose block sizes itself with CSS
// container queries, so its BlockHost wrapper becomes a measured inline-size
// container. Called from the widget type's constructor at registration.
//
// A block that declares this takes on style + inline-size containment for its
// whole subtree — see inlineSizeHosts for what was and was not measured.
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
