package calendar

import "encoding/json"

// calendar_v2_editor_helpers.go — server-side helpers for the full event
// editor DRAWER (C-CAL-LARGE-EDITOR, design slice 5 of 6). The drawer's
// tier segment, type chips, and the "fantasy-equivalence" hint line all read
// their vocabulary from the active calendar / campaign here, so the templ
// stays declarative and the JS (event_grid.js) only handles interaction.
//
// Cites: cordinator/decisions/2026-05-21-core-tenets §T-B3 (production UI —
// data-driven controls, no hard-coded vocabulary) · §T-B2 (plugin isolation —
// the campaigns tier defaults are mirrored locally, never imported).

// platformDefaultTierOptions mirrors the campaigns plugin's platform-default
// event tiers (campaigns/service.go GetEventTierDefinitions). Kept LOCAL so the
// calendar plugin does not import campaigns (T-B2). Used only as the render
// fallback when a campaign has no configured tier vocabulary AND the handler's
// loadTierDefinitions returned nil (tierLister unwired / lookup miss). The
// prominence values match mapProminenceToTier's bands so the segment order and
// the grid's tier rendering agree.
func platformDefaultTierOptions() []TierDefinitionAlias {
	return []TierDefinitionAlias{
		{Slug: "major", Name: "Major", Color: "#ef4444", Prominence: 100},
		{Slug: "standard", Name: "Standard", Color: "#6366f1", Prominence: 50},
		{Slug: "detail", Name: "Detail", Color: "#94a3b8", Prominence: 10},
	}
}

// eventTierOptions returns the tier vocabulary the drawer's WHO-section tier
// segment renders. Prefers the campaign-configured definitions (data.
// TierDefinitions); falls back to the platform defaults so the control is never
// empty. The drawer persists the chosen slug through the existing (now bound)
// events endpoint's `tier` field.
func eventTierOptions(data CalendarV2ViewData) []TierDefinitionAlias {
	if len(data.TierDefinitions) > 0 {
		return data.TierDefinitions
	}
	return platformDefaultTierOptions()
}

// typeChipPipStyle is the inline style for a type chip's color pip. Mirrors
// monthLinePipStyle (calendar_v2_helpers.go) — templ sanitizes the style
// attribute, and the category color is operator-defined the same way the month
// grid already trusts it.
func typeChipPipStyle(color string) string {
	if color == "" {
		return "background: var(--color-text-muted);"
	}
	return "background: " + color + ";"
}

// eventDrawerEpoch returns the active calendar's epoch suffix (e.g. "DR"), or
// "" when none is set. The drawer's JS appends it to a bare year when no named
// era covers the year, so the fantasy hint reads "1492 DR" rather than "1492".
func eventDrawerEpoch(data CalendarV2ViewData) string {
	if data.ActiveCalendar != nil && data.ActiveCalendar.EpochName != nil {
		return *data.ActiveCalendar.EpochName
	}
	return ""
}

// drawerEra is the minimal era shape the drawer's fantasy-hint JS needs: a
// named year-range lookup (start..end, end nil = ongoing).
type drawerEra struct {
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   *int   `json:"end"`
}

// eventDrawerErasJSON serializes the active calendar's eras into the compact
// lookup array the drawer JS uses to resolve a year → era name for the
// fantasy-equivalence hint ("= Harvestwane 16 · Year of the Broken Lantern").
// Returns "[]" when there are no eras so the JS parse is always safe.
func eventDrawerErasJSON(data CalendarV2ViewData) string {
	if data.ActiveCalendar == nil || len(data.ActiveCalendar.Eras) == 0 {
		return "[]"
	}
	out := make([]drawerEra, 0, len(data.ActiveCalendar.Eras))
	for _, e := range data.ActiveCalendar.Eras {
		out = append(out, drawerEra{Name: e.Name, Start: e.StartYear, End: e.EndYear})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}
