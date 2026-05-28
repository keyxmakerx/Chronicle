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

// SubresourceKind identifies which homogeneous bulk-set resource a
// sub-resource grid page is editing. The four batch-A kinds share the
// same card-grid + drawer shape; per-resource form fields branch by
// kind. Batch B (PR 3) will add: eras, categories, festivals, cycles,
// zones — all with the same shell but heterogeneous forms.
type SubresourceKind string

const (
	SubresourceMonths   SubresourceKind = "months"
	SubresourceWeekdays SubresourceKind = "weekdays"
	SubresourceMoons    SubresourceKind = "moons"
	SubresourceSeasons  SubresourceKind = "seasons"
)

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
	// per request — by Kind.
	Months   []Month
	Weekdays []Weekday
	Moons    []Moon
	Seasons  []Season
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
// endpoints — no new wire surface in this PR.
func subresourcePUTPath(campaignID, calendarID string, kind SubresourceKind) string {
	return "/campaigns/" + campaignID + "/calendars/" + calendarID + "/" + string(kind)
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
	}
	return "item"
}
