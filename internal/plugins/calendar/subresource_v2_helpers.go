// subresource_v2_helpers.go — pure-Go helpers consumed by
// subresource_v2.templ. Kept separate so the helpers are unit-testable
// and the templ file stays focused on rendering.

package calendar

import (
	"encoding/json"
	"log/slog"

	"github.com/a-h/templ"
)

// subresourceSiblingHref returns a URL to a sibling sub-resource
// settings page (e.g. "configure zones first →" from weather). Used
// by the singular weather state card + drawer when the zones catalog
// is empty.
func subresourceSiblingHref(data SubresourceViewData, sibling SubresourceKind) templ.SafeURL {
	if data.Calendar == nil {
		return templ.SafeURL("/campaigns/" + data.CampaignID + "/calendar/v2")
	}
	return templ.SafeURL("/campaigns/" + data.CampaignID + "/calendar/v2/" + data.Calendar.ID + "/settings/" + string(sibling))
}

// singularHeading returns the weather-state card's title. Falls back
// to "No weather state" for calendars that never set weather.
func singularHeading(data SubresourceViewData) string {
	if data.Weather == nil {
		return "No weather state"
	}
	if data.Weather.PresetLabel != nil && *data.Weather.PresetLabel != "" {
		return *data.Weather.PresetLabel
	}
	if data.Weather.PresetID != nil && *data.Weather.PresetID != "" {
		return *data.Weather.PresetID
	}
	if data.Weather.ZoneName != nil && *data.Weather.ZoneName != "" {
		return *data.Weather.ZoneName + " zone"
	}
	return "Weather state (no preset)"
}

// singularSwatch returns the color hex to render in the weather card's
// color swatch. Prefers Color override, falls back to "" (no swatch).
func singularSwatch(data SubresourceViewData) string {
	if data.Weather == nil || data.Weather.Color == nil {
		return ""
	}
	return *data.Weather.Color
}

// singularDetailLines returns the secondary text rows shown under the
// weather card heading. One per non-nil field so empty values stay
// hidden; surface lines only when there's something to say.
func singularDetailLines(data SubresourceViewData) []string {
	if data.Weather == nil {
		return []string{"Click to set the calendar's current weather."}
	}
	var lines []string
	if data.Weather.TemperatureCelsius != nil {
		lines = append(lines, "Temperature · "+formatFloat(*data.Weather.TemperatureCelsius)+"°C")
	}
	if data.Weather.ZoneName != nil && *data.Weather.ZoneName != "" {
		lines = append(lines, "Active zone · "+*data.Weather.ZoneName)
	} else if data.Weather.ZoneID != nil && *data.Weather.ZoneID != "" {
		lines = append(lines, "Active zone · "+*data.Weather.ZoneID)
	}
	if data.Weather.Description != nil && *data.Weather.Description != "" {
		lines = append(lines, *data.Weather.Description)
	}
	if len(lines) == 0 {
		lines = []string{"No additional details set."}
	}
	return lines
}

// formatFloat renders a float64 with no trailing zeros and at most
// one decimal place — keeps "15.0" as "15", "15.5" as "15.5".
func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return itoa(int(f))
	}
	// Single-decimal precision keeps the card heading scannable.
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	if frac < 0 {
		frac = -frac
	}
	return itoa(whole) + "." + itoa(frac)
}

// subresourceBackHref returns the URL for the "back" arrow that
// returns to the V2 calendar shell (Month view by default).
func subresourceBackHref(data SubresourceViewData) templ.SafeURL {
	if data.Calendar == nil {
		return templ.SafeURL("/campaigns/" + data.CampaignID + "/calendar/v2")
	}
	return templ.SafeURL("/campaigns/" + data.CampaignID + "/calendar/v2/" + data.Calendar.ID + "/month")
}

// subresourcePayloadJSON serializes the per-resource list as JSON
// for the subresource_grid.js widget to round-trip. Drawer Save
// reconstructs the full list (bulk-set PUT semantics) by patching
// this payload with edited / created / deleted entries.
//
// JSON encoding failure here is a programming error (the model
// types are all serializable) but we slog rather than panic — the
// page still renders without dnd / drawer if the payload is malformed.
func subresourcePayloadJSON(data SubresourceViewData) string {
	var payload any
	switch data.Kind {
	case SubresourceMonths:
		payload = data.Months
	case SubresourceWeekdays:
		payload = data.Weekdays
	case SubresourceMoons:
		payload = data.Moons
	case SubresourceSeasons:
		payload = data.Seasons
	case SubresourceEras:
		payload = data.Eras
	case SubresourceCategories:
		payload = data.EventCategories
	case SubresourceFestivals:
		payload = data.Festivals
	case SubresourceCycles:
		payload = data.Cycles
	case SubresourceZones:
		payload = data.Zones
	case SubresourceWeather:
		// Singular: marshal the Weather struct directly. Drawer JS
		// reads it as an object, not an array. nil means "no state
		// yet" which the drawer renders as create mode.
		if data.Weather == nil {
			return "{}"
		}
		payload = data.Weather
	default:
		payload = []any{}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("subresource payload marshal failed",
			slog.String("kind", string(data.Kind)),
			slog.Any("error", err),
		)
		return "[]"
	}
	return string(b)
}

// boolToStr returns "true"/"false" for data-attribute use. Templ's
// expr-attributes only accept string-typed values for direct attrs.
func boolToStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// subresourceEmptyExplainer returns a contextual hint for the empty
// state, helping operators understand what each resource is for.
func subresourceEmptyExplainer(kind SubresourceKind) string {
	switch kind {
	case SubresourceMonths:
		return "Define the months that make up the calendar year. Each month has a name and day count."
	case SubresourceWeekdays:
		return "Define the days of the week. Rest days get a tinted background in Month view."
	case SubresourceMoons:
		return "Track lunar bodies. Each moon has a cycle length and color used in Day/Week visuals."
	case SubresourceSeasons:
		return "Mark seasonal date ranges. Each season has a color and optional weather effect."
	case SubresourceEras:
		return "Define historical eras (Age of Magic, Third Age, etc.). Each era spans a range of years and can be ongoing."
	case SubresourceCategories:
		return "Group events by category (Combat, Travel, Festival, etc.). Each category has a slug, icon, and color used throughout the calendar."
	case SubresourceFestivals:
		return "Festivals are fixed calendar entries — holidays that don't recur as events. Each lives on a specific month + day, or intercalary between months."
	case SubresourceCycles:
		return "Cycles rotate through entries over time (zodiac, elemental, seasonal). Each cycle has a length in years and an ordered list of entries."
	case SubresourceZones:
		return "Weather zones define climate regions (temperate, tropical, arctic). The active zone drives the weather generator. Foundry sync may also edit zones."
	case SubresourceWeather:
		return "Set the current in-world weather: preset, temperature, and the active climate zone. Updates fire the calendar.weather.changed event for connected Foundry clients."
	}
	return "No items configured."
}
