// schedule.go — THE SCHEDULE: GET /campaigns/:id/schedule, the calendar-v4
// W-G Part B surface (C-CALV4-RSVP-P8 §10).
//
// ONE PAGE, FIVE SURFACES, in two role-ordered DOM orders:
//
//	Director   Verdict → Matrix → (Roster · Painter) → Answer
//	Player     (Answer · Painter) → Verdict → Matrix
//
// THE DESIGN CONTRACT IS THE SEALED MOCKUP, mockups/calendar-v4-schedule.html,
// signed by the operator on 2026-07-29 (decisions/2026-07-29-schedule-mockup-
// signed.md) against the wg-schedule-* stills. Where the W-G design spec
// (plans/2026-07-26-calendar-v4-wg-schedule-design-spec.md) and the mockup
// disagree, THE MOCKUP WINS — it is the signed artefact and the spec is not.
// Every divergence the drawing pass recorded is carried here rather than
// re-litigated; the load-bearing one is the `.sc-` RENAMES, and it is a CASCADE
// fact rather than a taste one:
//
//	.sc-why   not `.calrow .dt`   — the WHY sentence must be --text-secondary;
//	                                the signed `.dt` is --text-muted.
//	.sc-foot  not `.foot`         — the compose rule is two sentences; the
//	                                signed `.foot` is a 36px single-line strip.
//	.sc-body  not padding on .lhead — the panel's header band is the signed
//	                                `.lhead` verbatim; the body carries padding.
//
// The mockup declares `@layer tokens, base, marks, schedule, motion, block,
// bench, sheets, responsive` — `schedule` sorts BEFORE `bench`, so a schedule
// rule can never override a signed Bench rule at any specificity. This surface
// is therefore built so it never WANTS to: where a signed class does not fit a
// new context the answer is a NEW `sc-` class beside it, never a redefinition.
//
// ── WHAT THIS FILE READS, AND WHAT IT REFUSES TO READ ──────────────────────
//
// Everything comes from surfaces that already shipped:
//
//	the roster + week overlay  → BenchScheduleReader (P8A's seam, widened
//	                             ADDITIVELY here with minute-accurate lanes and
//	                             per-hour prefer counts — no new method, so no
//	                             existing implementation moved)
//	the session + its answers  → the viewer's OWN UpcomingAcrossCalendars index
//	                             (§4's W5a rule: no second calendar read, no
//	                             role branch that resolves calendars itself)
//	the derived window         → benchRsvpPeakRun, THE SAME arithmetic the Bench
//	                             panel prints, so the two surfaces cannot
//	                             disagree about when to play
//	the ask control            → benchRsvpAsk (P8B's LIVE Nudge)
//	availability WRITES        → the scheduler's own shipped PUT routes. Part B
//	                             adds NO write route and NO migration.
//
// ── PERMISSION IS ABSENCE, AND IT IS IN THE PAYLOAD ────────────────────────
//
// IsGM is the ONLY permission input and it decides what is BUILT, never what a
// template hides. A player's ScheduleData carries no member lane, no other
// member's name, no Director control and no `needs backend` chip — the builder
// never emits one. The single ruled exception is the addon fault box, which
// names the roles that can repair it (§2.4, kept knowingly).
package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"

	"github.com/labstack/echo/v4"
)

// scheduleBands are the three hour bands the `.phead` segment offers, in the
// mockup's order. A band NEVER silently hides someone: a member whose only
// windows fall outside it gets a printed row saying so, with the count and the
// repair ("widen it above"), which is why the control can be this coarse.
var scheduleBands = []struct {
	Key, Label string
	From, To   int
}{
	{"evening", "Evening 16–24", 16, 24},
	{"afternoon", "Afternoon 12–20", 12, 20},
	{"all", "All 24", 0, 24},
}

// scheduleDefaultBand is what an unrecognised (or absent) ?band resolves to.
const scheduleDefaultBand = "evening"

// scheduleMotionLine is printed VERBATIM on the surface, exactly as the sealed
// mockup prints it (spec §2.0). It is the page's own statement of its motion
// budget, in the operator's language rather than in CSS — and it is the sentence
// static/css/calendar-schedule.css is graded against.
const scheduleMotionLine = "Nothing on this page ever moves to a new place. Things change " +
	"colour when you point at them, one panel folds open over the rows beneath it, and menus " +
	"appear where you clicked — that is the whole list."

// scheduleProportionLine is the mockup's printed proportion rule. It is on the
// surface for the same reason the Bench's is: it is what stops the page decaying
// into four identical panels the first time somebody "tidies" the grid.
const scheduleProportionLine = "The answer and the grid it came from are the two big surfaces, " +
	"and they stack — they can never sit side by side, at any width. Your own week and the " +
	"roster are the two small ones, and they can never grow into the big ones. There is no " +
	"screen size at which these four become peers."

// ScheduleToggle is one button in a `.seg` segmented control. Href is a LINK in
// production — every control on this page is a GET against the same route, which
// is what makes the whole surface state-addressable and JS-free.
type ScheduleToggle struct {
	Key, Label string
	Href       string
	Pressed    bool
	// Disabled + Title carry the one control the page ever refuses: DAY zoom
	// below the width where a day column can still clear the 24px target floor.
	Disabled bool
	Title    string
}

// ScheduleFault is the `.sc-faultbox` — a NAMED, REPAIRABLE refusal drawn where
// a surface's body would be.
//
// It is NOT a dashed reserve. Dashed means "not built yet" and only that
// (ledger #21); an unavailable feature is a fault, and a fault says who can fix
// it. The one place this page names another member to a player is here, and it
// is kept knowingly (§2.4): a repair the reader cannot perform is only
// actionable if it names who can, and "ask an administrator" is the sentence
// that makes a product feel broken.
type ScheduleFault struct {
	Headline string
	Detail   string
}

// ScheduleData is the page's complete render input. Assembled by buildSchedule
// and rendered by schedule.templ; nothing in schedule.templ queries.
type ScheduleData struct {
	CampaignID   string
	CampaignName string

	// IsGM is the dm-sight role (owner or co-DM) and the ONLY permission input.
	IsGM      bool
	CSRFToken string
	// LoadError degrades the whole surface to a friendly "couldn't load" card.
	// It is the one state that costs the page its body, and it never says which
	// read failed.
	LoadError bool

	// --- the week, and the stepper that walks it -----------------------------

	// WeekStart is the Monday the overlay snapped to, YYYY-MM-DD. ONE
	// Monday-snapped week per request — no multi-week payload exists (ledger
	// #15), which is why the control is a STEPPER and never a range picker.
	WeekStart string
	// WeekLabel is the human Monday ("Mon 20 Jul 2026"); WeekRange is the
	// stepper's pill ("20–26 Jul").
	WeekLabel string
	WeekRange string
	PrevHref  string
	NextHref  string

	// --- the zone this page states its times in ------------------------------

	// Zone is the full IANA identifier, EMPTY when neither the viewer nor the
	// calendar has one. ZoneLeaf is the last path segment ("Chicago") — the
	// only zone name Chronicle can honestly print, because it has no
	// abbreviation helper and a fabricated "CDT" is worse than a real
	// "Chicago" (ledger #5).
	Zone     string
	ZoneLeaf string
	// ZoneSource is "member" | "calendar" | "none", so the frame can say WHOSE
	// zone it is rather than implying it belongs to the reader.
	ZoneSource string
	// ZoneFrame is the printed sentence built from the three above.
	ZoneFrame string

	// --- the controls --------------------------------------------------------

	Band        string
	BandLabel   string
	BandFrom    int
	BandTo      int
	BandOptions []ScheduleToggle
	Zoom        string
	ZoomOptions []ScheduleToggle
	// Day is the ISO date DAY zoom is pointed at; empty in WEEK zoom.
	Day string

	// --- the five surfaces ---------------------------------------------------

	Verdict ScheduleVerdict
	Matrix  ScheduleMatrix
	Roster  ScheduleRoster
	Painter SchedulePainter
	Answer  ScheduleAnswer

	// MotionLine and Proportion are printed on the surface, verbatim.
	MotionLine string
	Proportion string
}

// scheduleInput is everything buildSchedule needs from the request, resolved by
// the handler. Keeping it a struct rather than eight parameters means the
// handler stays thin and the builder stays testable.
type scheduleInput struct {
	Campaign *campaigns.Campaign
	UserID   string
	// Role is the VISIBILITY role (cc.VisibilityRole()), which is what every
	// filter in this plugin takes.
	Role      int
	CSRFToken string

	// The four display-only query params. NONE of them names a subject: this
	// route accepts no identity parameter at all, which is what makes the IDOR
	// probe in schedule_route_test.go vacuous by construction rather than by
	// validation.
	Week string
	Band string
	Zoom string
	Day  string
	// Cand is the RANK (1|2|3) of the selected candidate window, never a
	// position — cards sit in date order and a selection may not move one.
	Cand string
	// Scope is the Painter's [This week only | Every week] segment.
	Scope string
	// Pref / Sug are the two disclosure states the page carries in the URL so
	// every render is reproducible.
	Pref string
	Sug  string
}

// SchedulePage renders THE SCHEDULE at GET /campaigns/:id/schedule.
//
// THE HANDLER IS THIN, as house rule 1 requires: it resolves the request
// context and delegates the whole assembly to buildSchedule and the whole
// markup to schedule.templ.
func (h *Handler) SchedulePage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	data := h.buildSchedule(c.Request().Context(), scheduleInput{
		Campaign:  cc.Campaign,
		UserID:    auth.GetUserID(c),
		Role:      cc.VisibilityRole(),
		CSRFToken: middleware.GetCSRFToken(c),
		Week:      c.QueryParam("week"),
		Band:      c.QueryParam("band"),
		Zoom:      c.QueryParam("zoom"),
		Day:       c.QueryParam("day"),
		Cand:      c.QueryParam("cand"),
		Scope:     c.QueryParam("scope"),
		Pref:      c.QueryParam("pref"),
		Sug:       c.QueryParam("sug"),
	})
	return middleware.Render(c, 200, SchedulePageView(cc, data))
}

// --- the assembly -----------------------------------------------------------

// buildSchedule assembles the page.
//
// THE DEGRADE LADDER, and every rung keeps the surface's identity:
//
//	no schedule seam / empty roster → the panels print their empty states; the
//	                                  Painter prints a NAMED fault, not a blank
//	overlay read fails              → no lanes, no ranking, the honesty states
//	                                  the mockup draws for exactly that
//	upcoming index fails            → no session, so no RSVP slot and no clocks;
//	                                  the frame says so rather than guessing
//
// Nothing here reads a calendar directly. The session comes from the viewer's
// own UpcomingAcrossCalendars index — viewer-filtered AT THE SOURCE — which is
// the cheapest way to honour §4's W5a rule: add no path at all.
func (h *Handler) buildSchedule(ctx context.Context, in scheduleInput) ScheduleData {
	isGM := permissions.CanSeeDmOnly(in.Role)
	data := ScheduleData{
		CampaignID:   in.Campaign.ID,
		CampaignName: in.Campaign.Name,
		IsGM:         isGM,
		CSRFToken:    in.CSRFToken,
		MotionLine:   scheduleMotionLine,
		Proportion:   scheduleProportionLine,
	}

	band := scheduleResolveBand(in.Band)
	data.Band, data.BandLabel = band.Key, band.Label
	data.BandFrom, data.BandTo = band.From, band.To
	data.Zoom = scheduleResolveZoom(in.Zoom)

	// THE SESSION FIRST, because the week defaults to the week it falls in. The
	// index is the VIEWER'S OWN — one read, one filter, so this page and the
	// Bench can never disagree about which events exist.
	upcoming := benchUpcoming(ctx, BlockSpine(), in.Campaign.ID, BlockViewer{
		UserID: in.UserID, Role: in.Role,
	})
	session, row, anchorZone := benchRsvpPickSession(upcoming)

	weekStart := scheduleResolveWeek(in.Week, session)
	data.WeekStart = weekStart.Format("2006-01-02")
	data.WeekLabel = weekStart.Format("Mon 2 Jan 2006")
	data.WeekRange = scheduleWeekRange(weekStart)
	data.Day = scheduleResolveDay(in.Day, weekStart)

	// The stepper's two hrefs preserve every other control's state, so stepping
	// a week never silently resets the band, the zoom or an open disclosure.
	base := scheduleQuery(in, data)
	data.PrevHref = scheduleHref(in.Campaign.ID, base, "week", weekStart.AddDate(0, 0, -7).Format("2006-01-02"))
	data.NextHref = scheduleHref(in.Campaign.ID, base, "week", weekStart.AddDate(0, 0, 7).Format("2006-01-02"))
	data.BandOptions = scheduleBandOptions(in.Campaign.ID, base, band.Key)
	data.ZoomOptions = scheduleZoomOptions(in.Campaign.ID, base, data.Zoom)

	roster, rerr := h.scheduleRoster(ctx, in.Campaign.ID)
	if rerr != nil {
		slog.Warn("schedule: roster read failed",
			slog.String("campaign_id", in.Campaign.ID), slog.Any("error", rerr))
	}

	// The zone the page states its own times in: the viewer's own stored zone
	// first, then the calendar's anchor, then nothing. A viewer with no stored
	// zone is TOLD so rather than shown a UTC guess wearing their name (§5).
	for _, m := range roster {
		if m.UserID == in.UserID && strings.TrimSpace(m.TZ) != "" {
			data.Zone, data.ZoneSource = m.TZ, "member"
			break
		}
	}
	if data.Zone == "" && anchorZone != "" {
		data.Zone, data.ZoneSource = anchorZone, "calendar"
	}
	if data.Zone == "" {
		data.ZoneSource = "none"
	}
	data.ZoneLeaf = scheduleZoneLeaf(data.Zone)
	data.ZoneFrame = scheduleZoneFrame(data.Zone, data.ZoneLeaf, data.ZoneSource)

	// The overlay is projected into SOME zone — the grid has to be drawn in one
	// — and the frame above says which, rather than the grid implying it.
	projectZone := data.Zone
	if projectZone == "" {
		projectZone = "UTC"
	}
	var avail *BenchAvailability
	if h.schedule != nil {
		a, aerr := h.schedule.BenchAvailability(ctx, in.Campaign.ID, data.WeekStart, projectZone, isGM)
		if aerr != nil {
			slog.Warn("schedule: availability read failed",
				slog.String("campaign_id", in.Campaign.ID), slog.Any("error", aerr))
		}
		avail = a
	}

	var answers map[string]string
	eventID, calendarID := "", ""
	if row != nil {
		eventID, calendarID = row.Event.ID, row.Calendar.ID
		if h.rsvpRead != nil {
			ans, aerr := h.rsvpRead.AnswersByUser(ctx, &row.Event, in.UserID, in.Role)
			if aerr != nil {
				slog.Warn("schedule: rsvp answers read failed",
					slog.String("event_id", row.Event.ID), slog.Any("error", aerr))
			}
			answers = ans
		}
	}

	sin := scheduleBuildInput{
		IsGM:       isGM,
		ViewerID:   in.UserID,
		CampaignID: in.Campaign.ID,
		CSRFToken:  in.CSRFToken,
		Roster:     roster,
		Avail:      avail,
		Answers:    answers,
		Session:    session,
		EventID:    eventID,
		CalendarID: calendarID,
		Zone:       data.Zone,
		ZoneLeaf:   data.ZoneLeaf,
		WeekStart:  weekStart,
		BandFrom:   band.From,
		BandTo:     band.To,
		Cand:       in.Cand,
		Scope:      scheduleResolveScope(in.Scope),
		PrefOpen:   in.Pref == "open",
		SugOpen:    in.Sug == "open",
		Base:       base,
	}

	// The ask control's two reads are GM-ONLY, exactly as the Bench's are, and
	// for the same reason: smtp.IsConfigured is a database read on every call,
	// a player has no ask control to explain, and the endpoint's own refusal is
	// the honest backstop either way.
	sin.MailConfigured = true
	sin.AskState = ScheduleAskState{Ready: true}
	if isGM {
		if h.mailStatus != nil {
			sin.MailConfigured = h.mailStatus.IsConfigured(ctx)
		}
		if h.rsvpRead != nil {
			if st, serr := h.rsvpRead.ScheduleAskState(ctx, in.Campaign.ID); serr == nil {
				sin.AskState = st
			} else {
				slog.Warn("schedule: schedule-ask cooldown read failed",
					slog.String("campaign_id", in.Campaign.ID), slog.Any("error", serr))
			}
		}
	}

	data.Verdict = scheduleBuildVerdict(sin)
	data.Matrix = scheduleBuildMatrix(sin)
	data.Roster = scheduleBuildRoster(sin)
	data.Painter = scheduleBuildPainter(sin)
	data.Answer = scheduleBuildAnswer(sin)
	return data
}

// scheduleRoster reads the party-visible roster through P8A's seam. A nil seam
// is NOT an error — it is a host that has not wired the sessions plugin, and the
// page degrades to its empty states rather than to a stack trace.
func (h *Handler) scheduleRoster(ctx context.Context, campaignID string) ([]BenchRosterMember, error) {
	if h.schedule == nil {
		return nil, nil
	}
	return h.schedule.BenchRoster(ctx, campaignID)
}

// --- the request's display state --------------------------------------------

// scheduleResolveBand clamps an arbitrary ?band to a known one.
func scheduleResolveBand(key string) struct {
	Key, Label string
	From, To   int
} {
	for _, b := range scheduleBands {
		if b.Key == key {
			return b
		}
	}
	for _, b := range scheduleBands {
		if b.Key == scheduleDefaultBand {
			return b
		}
	}
	return scheduleBands[0]
}

// scheduleResolveZoom clamps ?zoom. WEEK is the default and the only value a
// narrow viewport ever gets — but the SERVER cannot see a viewport, so the
// narrow rule is expressed in CSS (the day columns collapse) and the control
// carries its own explanation rather than the server guessing a width.
func scheduleResolveZoom(z string) string {
	if z == "day" {
		return "day"
	}
	return "week"
}

// scheduleResolveScope clamps the Painter's ?scope. "week" writes a DATE
// EXCEPTION; "recurring" writes the member's NORMAL HOURS. Two different tables,
// two different sentences, and the segment says which is which.
func scheduleResolveScope(s string) string {
	if s == "recurring" {
		return "recurring"
	}
	return "week"
}

// scheduleResolveWeek snaps ?week to a Monday, defaulting to the week the
// resolved session falls in (and to this week when there is none).
//
// SNAPPED, NEVER TRUSTED: an arbitrary date in the query would otherwise
// produce a seven-day window starting on a Thursday, and every column head, the
// overlay's own week key and the Painter's day rows would silently disagree
// about what "this week" means.
func scheduleResolveWeek(raw string, s *BenchRsvpSession) time.Time {
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err == nil {
		offset := (int(t.Weekday()) + 6) % 7
		return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, time.UTC)
	}
	return benchRsvpWeekStart(s)
}

// scheduleResolveDay clamps ?day into the resolved week, defaulting to its
// Saturday — the day a table most often plays, and the one the sealed mockup
// points DAY zoom at.
func scheduleResolveDay(raw string, weekStart time.Time) string {
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err == nil {
		diff := int(t.Sub(weekStart).Hours() / 24)
		if diff >= 0 && diff < 7 {
			return t.Format("2006-01-02")
		}
	}
	return weekStart.AddDate(0, 0, 5).Format("2006-01-02")
}

// scheduleWeekRange renders the stepper's pill: "20–26 Jul", or "29 Jun – 5 Jul"
// when the week straddles a month.
func scheduleWeekRange(weekStart time.Time) string {
	end := weekStart.AddDate(0, 0, 6)
	if weekStart.Month() == end.Month() {
		return fmt.Sprintf("%d–%d %s", weekStart.Day(), end.Day(), end.Format("Jan"))
	}
	return fmt.Sprintf("%d %s – %d %s",
		weekStart.Day(), weekStart.Format("Jan"), end.Day(), end.Format("Jan"))
}

// scheduleZoneLeaf prints the LAST IANA path segment and nothing else.
//
// Chronicle has no abbreviation or offset helper on this path (timeutil.Zone's
// Label == Value by construction), so the page prints "Chicago" with the full
// identifier in `title` and NEVER fabricates a "CDT" (ledger #5). A real leaf is
// better than an invented abbreviation.
func scheduleZoneLeaf(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return ""
	}
	if i := strings.LastIndex(tz, "/"); i >= 0 && i+1 < len(tz) {
		return tz[i+1:]
	}
	return tz
}

// scheduleZoneFrame states what the page's times mean, ONCE, and says whose zone
// it is.
func scheduleZoneFrame(zone, leaf, source string) string {
	switch source {
	case "member":
		return "times in " + leaf + " · your zone"
	case "calendar":
		return "times in the calendar's zone, " + leaf + " — you have not set yours"
	default:
		_ = zone
		return "no time zone is set for you or this calendar"
	}
}

// --- the controls' hrefs ----------------------------------------------------

// scheduleQuery is the page's full display state as query values. EVERY control
// is a LINK carrying the whole state, which is what makes the surface
// state-addressable, reproducible in a render harness, and JS-free.
func scheduleQuery(in scheduleInput, data ScheduleData) url.Values {
	v := url.Values{}
	v.Set("week", data.WeekStart)
	v.Set("band", data.Band)
	v.Set("zoom", data.Zoom)
	v.Set("day", data.Day)
	if c := strings.TrimSpace(in.Cand); c != "" {
		v.Set("cand", c)
	}
	v.Set("scope", scheduleResolveScope(in.Scope))
	if in.Pref == "open" {
		v.Set("pref", "open")
	}
	if in.Sug == "open" {
		v.Set("sug", "open")
	}
	return v
}

// scheduleHref renders one control's target: the page's own path with a single
// key overridden.
func scheduleHref(campaignID string, base url.Values, key, value string) string {
	next := url.Values{}
	for k, vals := range base {
		next[k] = append([]string(nil), vals...)
	}
	next.Set(key, value)
	return "/campaigns/" + campaignID + "/schedule?" + next.Encode()
}

func scheduleBandOptions(campaignID string, base url.Values, active string) []ScheduleToggle {
	out := make([]ScheduleToggle, 0, len(scheduleBands))
	for _, b := range scheduleBands {
		out = append(out, ScheduleToggle{
			Key: b.Key, Label: b.Label, Pressed: b.Key == active,
			Href: scheduleHref(campaignID, base, "band", b.Key),
		})
	}
	return out
}

func scheduleZoomOptions(campaignID string, base url.Values, active string) []ScheduleToggle {
	return []ScheduleToggle{
		{Key: "week", Label: "Week", Pressed: active == "week",
			Href: scheduleHref(campaignID, base, "zoom", "week")},
		{Key: "day", Label: "Day", Pressed: active == "day",
			Href:  scheduleHref(campaignID, base, "zoom", "day"),
			Title: "one day at full width; the week view is forced on a narrow screen"},
	}
}

// scheduleHour renders an hour-of-day as a clock time. The end hour of a window
// is EXCLUSIVE and printed as the time the window closes at, so a one-hour peak
// reads 19:00–20:00 rather than 19:00–19:00.
func scheduleHour(h int) string {
	return fmt.Sprintf("%02d:00", ((h%24)+24)%24)
}

// scheduleMinute renders a minute-of-day as a clock time, for the popovers where
// the minute-accurate truth is the whole point.
func scheduleMinute(m int) string {
	m = ((m % 1440) + 1440) % 1440
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// schedulePlural is the page's one pluraliser. Every count on this surface
// prints its denominator, so "1 member shows" and "3 members show" both have to
// read as English.
func schedulePlural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// scheduleItoa keeps the templ side free of strconv.
func scheduleItoa(n int) string { return strconv.Itoa(n) }
