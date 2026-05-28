// subresource_v2_helpers.go — pure-Go helpers consumed by
// subresource_v2.templ. Kept separate so the helpers are unit-testable
// and the templ file stays focused on rendering.

package calendar

import (
	"encoding/json"
	"log/slog"

	"github.com/a-h/templ"
)

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
	}
	return "No items configured."
}
