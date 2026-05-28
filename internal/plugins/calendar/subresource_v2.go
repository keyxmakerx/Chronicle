// subresource_v2.go — V2 sub-resource settings handler + helpers.
// Wave 1 PR 2 (C-CAL-V2-SUBRESOURCE-CARDS-A). Implements the
// card-grid editor for months / weekdays / moons / seasons at
// /campaigns/:id/calendar/v2/:calId/settings/:resource. PUT
// endpoints reuse the existing V1 bulk-set routes (per dispatch §A):
//
//   PUT /campaigns/:id/calendars/:calId/months    UpdateMonthsAPI
//   PUT /campaigns/:id/calendars/:calId/weekdays  UpdateWeekdaysAPI
//   PUT /campaigns/:id/calendars/:calId/moons     UpdateMoonsAPI
//   PUT /campaigns/:id/calendars/:calId/seasons   UpdateSeasonsAPI
//
// V1 settings tab stays operational at /campaigns/:id/calendars/...;
// V2 surface is additive per PR #363's coexistence pattern.

package calendar

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SubresourceKind identifies which sub-resource a settings page is
// editing. The four batch-A kinds share the card-grid + drawer shape;
// per-resource form fields branch by kind. Batch B (Wave 1 PR 3) adds
// 5 more list resources (eras, categories, festivals, cycles, zones)
// plus the weather-singular exception — see ShowV2SubresourceSettings
// for the singular render path.
type SubresourceKind string

const (
	SubresourceMonths     SubresourceKind = "months"
	SubresourceWeekdays   SubresourceKind = "weekdays"
	SubresourceMoons      SubresourceKind = "moons"
	SubresourceSeasons    SubresourceKind = "seasons"
	// V2 Wave 1 PR 3 / C-CAL-V2-SUBRESOURCE-CARDS-B additions:
	SubresourceEras       SubresourceKind = "eras"
	SubresourceCategories SubresourceKind = "categories"
	SubresourceFestivals  SubresourceKind = "festivals"
	SubresourceCycles     SubresourceKind = "cycles"
	SubresourceZones      SubresourceKind = "zones"
	SubresourceWeather    SubresourceKind = "weather"
)

// isSingular reports whether the kind renders as a single state card
// rather than a card grid. Weather is the lone singular kind: a
// calendar has one current weather state, not a list. The handler
// + templ + JS all branch on this to skip dnd / add-card affordances.
func (k SubresourceKind) isSingular() bool {
	return k == SubresourceWeather
}

// SubresourceCardData is the uniform shape the shared subresourceCard
// templ component renders. Per-resource lists project into this
// shape so the card markup stays homogeneous.
type SubresourceCardData struct {
	ID          string // stable index identifier (sort_order string or DB id)
	Index       int    // 0-based position in the list (used for dnd)
	Name        string
	Subtitle    string // e.g. "31 days", "rest day", "cycle 28d"
	Color       string // hex; empty when not applicable (months/weekdays)
	IsAccent    bool   // true for special tinting (e.g. weekday IsRestDay)
}

// SubresourceViewData bundles everything the page + grid + drawer need.
// Kept resource-agnostic at the wrapper level; per-resource data lives
// in the embedded slices (only the relevant one is populated).
type SubresourceViewData struct {
	Calendar    *Calendar
	CampaignID  string
	Kind        SubresourceKind
	Cards       []SubresourceCardData
	IsOwner     bool
	IsScribe    bool
	CSRFToken   string
	// Per-resource raw payloads used by the drawer JS to round-trip the
	// full list on Save (bulk-set PUT semantics). Only one is populated
	// per request — by Kind. Weather (singular) populates Weather + the
	// catalog of Zones the picker references.
	Months          []Month
	Weekdays        []Weekday
	Moons           []Moon
	Seasons         []Season
	Eras            []Era
	EventCategories []EventCategory
	Festivals       []Festival
	Cycles          []Cycle
	Zones           []WeatherZone
	Weather         *Weather // populated only when Kind == SubresourceWeather
}

// ShowV2SubresourceSettings renders the V2 card-grid editor for one
// homogeneous bulk-set resource. The handler dispatches on the
// :resource URL param; each resource projects into the uniform
// SubresourceCardData shape for grid rendering.
//
// GET /campaigns/:id/calendar/v2/:calId/settings/:resource
func (h *Handler) ShowV2SubresourceSettings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	calID := c.Param("calId")
	resource := SubresourceKind(c.Param("resource"))

	cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	data := SubresourceViewData{
		Calendar:   cal,
		CampaignID: cc.Campaign.ID,
		Kind:       resource,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:   cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	switch resource {
	case SubresourceMonths:
		data.Months = cal.Months
		data.Cards = monthsToCards(cal.Months)
	case SubresourceWeekdays:
		data.Weekdays = cal.Weekdays
		data.Cards = weekdaysToCards(cal.Weekdays)
	case SubresourceMoons:
		data.Moons = cal.Moons
		data.Cards = moonsToCards(cal.Moons)
	case SubresourceSeasons:
		data.Seasons = cal.Seasons
		data.Cards = seasonsToCards(cal.Seasons)
	case SubresourceEras:
		data.Eras = cal.Eras
		data.Cards = erasToCards(cal.Eras)
	case SubresourceCategories:
		data.EventCategories = cal.EventCategories
		data.Cards = categoriesToCards(cal.EventCategories)
	case SubresourceFestivals:
		// Festivals come from a separate Get call — they aren't in
		// the eager-loaded calendar struct.
		fests, err := h.svc.GetFestivals(c.Request().Context(), cal.ID)
		if err != nil {
			return err
		}
		data.Festivals = fests
		data.Cards = festivalsToCards(fests)
	case SubresourceCycles:
		// Cycles are also from a separate Get.
		cycles, err := h.svc.GetCycles(c.Request().Context(), cal.ID)
		if err != nil {
			return err
		}
		data.Cycles = cycles
		data.Cards = cyclesToCards(cycles)
	case SubresourceZones:
		zonesState, err := h.svc.GetWeatherZones(c.Request().Context(), cal.ID)
		if err != nil {
			return err
		}
		if zonesState != nil {
			data.Zones = zonesState.Zones
			data.Cards = zonesToCards(zonesState.Zones, zonesState.ActiveZone)
		}
	case SubresourceWeather:
		// Singular: load current state + the zones catalog (drawer
		// uses zones for the active-zone picker).
		weather, err := h.svc.GetWeather(c.Request().Context(), cal.ID)
		if err != nil {
			return err
		}
		data.Weather = weather
		zonesState, err := h.svc.GetWeatherZones(c.Request().Context(), cal.ID)
		if err != nil {
			return err
		}
		if zonesState != nil {
			data.Zones = zonesState.Zones
		}
	default:
		return apperror.NewNotFound("unknown sub-resource")
	}

	// Capture user-id for the page context (drawer Save uses
	// /api/.../calendars/:calId/<resource>; auth is via session).
	_ = auth.GetUserID(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, SubresourceGridFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, SubresourceSettingsPage(cc, data))
}

// --- Per-resource → card-data projections ---

func monthsToCards(months []Month) []SubresourceCardData {
	out := make([]SubresourceCardData, len(months))
	for i, m := range months {
		sub := pluralizeDays(m.Days)
		if m.IsIntercalary {
			sub += " · intercalary"
		}
		if m.LeapYearDays > 0 {
			sub += " · +" + itoa(m.LeapYearDays) + " leap"
		}
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     m.Name,
			Subtitle: sub,
		}
	}
	return out
}

func weekdaysToCards(weekdays []Weekday) []SubresourceCardData {
	out := make([]SubresourceCardData, len(weekdays))
	for i, w := range weekdays {
		sub := ""
		if w.IsRestDay {
			sub = "rest day"
		}
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     w.Name,
			Subtitle: sub,
			IsAccent: w.IsRestDay,
		}
	}
	return out
}

func moonsToCards(moons []Moon) []SubresourceCardData {
	out := make([]SubresourceCardData, len(moons))
	for i, m := range moons {
		sub := "cycle " + itoa(int(m.CycleDays)) + "d"
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     m.Name,
			Subtitle: sub,
			Color:    m.Color,
		}
	}
	return out
}

func seasonsToCards(seasons []Season) []SubresourceCardData {
	out := make([]SubresourceCardData, len(seasons))
	for i, s := range seasons {
		sub := "month " + itoa(s.StartMonth) + " · day " + itoa(s.StartDay) +
			" → month " + itoa(s.EndMonth) + " · day " + itoa(s.EndDay)
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     s.Name,
			Subtitle: sub,
			Color:    s.Color,
		}
	}
	return out
}

// --- Batch B projections (PR 3 / C-CAL-V2-SUBRESOURCE-CARDS-B) ---

func erasToCards(eras []Era) []SubresourceCardData {
	out := make([]SubresourceCardData, len(eras))
	for i, e := range eras {
		sub := "from year " + itoa(e.StartYear)
		if e.EndYear != nil {
			sub += " to " + itoa(*e.EndYear)
		} else {
			sub += " · ongoing"
		}
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     e.Name,
			Subtitle: sub,
			Color:    e.Color,
		}
	}
	return out
}

func categoriesToCards(cats []EventCategory) []SubresourceCardData {
	out := make([]SubresourceCardData, len(cats))
	for i, c := range cats {
		// Categories ship slug + icon as identifying chrome; the subtitle
		// shows slug (operator-recognizable) + icon name if set.
		sub := c.Slug
		if c.Icon != "" {
			sub += " · " + c.Icon
		}
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     c.Name,
			Subtitle: sub,
			Color:    c.Color,
		}
	}
	return out
}

func festivalsToCards(fests []Festival) []SubresourceCardData {
	out := make([]SubresourceCardData, len(fests))
	for i, f := range fests {
		sub := ""
		switch {
		case f.AfterMonth != nil:
			sub = "intercalary after month " + itoa(*f.AfterMonth)
		case f.Month != nil && f.Day != nil:
			sub = "month " + itoa(*f.Month) + " · day " + itoa(*f.Day)
		case f.Month != nil:
			sub = "month " + itoa(*f.Month)
		default:
			sub = "no date set"
		}
		color := ""
		if f.Color != nil {
			color = *f.Color
		}
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     f.Name,
			Subtitle: sub,
			Color:    color,
		}
	}
	return out
}

func cyclesToCards(cycles []Cycle) []SubresourceCardData {
	out := make([]SubresourceCardData, len(cycles))
	for i, c := range cycles {
		sub := c.Type
		if c.CycleLength > 0 {
			sub += " · length " + itoa(c.CycleLength)
		}
		if n := len(c.Entries); n > 0 {
			sub += " · " + itoa(n) + " entries"
		}
		out[i] = SubresourceCardData{
			ID:       itoa(i),
			Index:    i,
			Name:     c.Name,
			Subtitle: sub,
		}
	}
	return out
}

// zonesToCards projects WeatherZone definitions. The active zone gets
// IsAccent=true so the card surfaces visually distinct from inactive
// zones (matches the V1 read-only zones panel's "active" badge style).
func zonesToCards(zones []WeatherZone, activeZoneID string) []SubresourceCardData {
	out := make([]SubresourceCardData, len(zones))
	for i, z := range zones {
		sub := z.ZoneID
		// Surface a hint about payload richness so operators know if
		// a zone has presets vs. is empty. Payload shape is opaque
		// per PR #360, but presence of "presets" is a common signal.
		if z.Payload != nil {
			if presets, ok := z.Payload["presets"].([]any); ok && len(presets) > 0 {
				sub += " · " + itoa(len(presets)) + " presets"
			}
		}
		out[i] = SubresourceCardData{
			ID:       z.ZoneID,
			Index:    i,
			Name:     z.Name,
			Subtitle: sub,
			IsAccent: z.ZoneID == activeZoneID,
		}
	}
	return out
}

// pluralizeDays returns "N day" or "N days" so the card subtitle
// reads naturally for intercalary single-day months.
func pluralizeDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return itoa(n) + " days"
}

// itoa is a tiny strconv.Itoa indirection so the templ-helper file
// doesn't import strconv just for one call — keeps the import block
// tidy. (Internal use only.)
func itoa(n int) string {
	// Negative + zero + positive handled by strconv.Itoa semantics;
	// inline to avoid importing strconv into the helper file scope.
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// subresourcePUTPath returns the endpoint the drawer's Save action
// targets for a given resource. Reuses V1's existing bulk-set
// endpoints — no new wire surface in this PR. Two resources have
// distinct path shapes:
//   - categories →  /event-categories  (the V1 path)
//   - zones      →  /weather/zones     (PR #360 path)
func subresourcePUTPath(campaignID, calendarID string, kind SubresourceKind) string {
	suffix := string(kind)
	switch kind {
	case SubresourceCategories:
		suffix = "event-categories"
	case SubresourceZones:
		suffix = "weather/zones"
	}
	return "/campaigns/" + campaignID + "/calendars/" + calendarID + "/" + suffix
}

// subresourceTitle returns the human-readable heading for the page.
func subresourceTitle(kind SubresourceKind) string {
	switch kind {
	case SubresourceMonths:
		return "Months"
	case SubresourceWeekdays:
		return "Weekdays"
	case SubresourceMoons:
		return "Moons"
	case SubresourceSeasons:
		return "Seasons"
	case SubresourceEras:
		return "Eras"
	case SubresourceCategories:
		return "Event Categories"
	case SubresourceFestivals:
		return "Festivals"
	case SubresourceCycles:
		return "Cycles"
	case SubresourceZones:
		return "Weather Zones"
	case SubresourceWeather:
		return "Weather"
	}
	return "Sub-resource"
}

// subresourceSingular returns the singular form for "Add X" + empty-state.
func subresourceSingular(kind SubresourceKind) string {
	switch kind {
	case SubresourceMonths:
		return "month"
	case SubresourceWeekdays:
		return "weekday"
	case SubresourceMoons:
		return "moon"
	case SubresourceSeasons:
		return "season"
	case SubresourceEras:
		return "era"
	case SubresourceCategories:
		return "category"
	case SubresourceFestivals:
		return "festival"
	case SubresourceCycles:
		return "cycle"
	case SubresourceZones:
		return "zone"
	case SubresourceWeather:
		return "weather state"
	}
	return "item"
}
