// block_host.go — DOM-id helpers for the re-render seam (C-WIDGET-BINDING-P4b).
// A host's widget block lives inside a stable wrapper element so a bind/unbind
// can target it for an in-place HTMX swap; the picker fragment loads into a
// separate slot inside that wrapper.
package widgetbindings

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
