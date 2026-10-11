package app

// The "Coming up" dashboard block: the next game nights, what this viewer
// still owes the table, and the next calendar events, in one card.
//
// It lives in internal/app because it joins the calendar and sessions plugins,
// which may not import each other. Everything here reads through the services'
// own methods, so the visibility rules are exactly the ones those plugins
// already enforce; nothing is filtered or re-derived locally except wording.
// A source that fails is logged and left out, so one broken plugin never
// takes the dashboard down.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

const (
	// comingUpTimeout bounds every read the block makes, so a slow database
	// delays only this card, never the rest of the dashboard.
	comingUpTimeout = 3 * time.Second
	// comingUpNightsShown, comingUpWorldShown and comingUpWaitingShown keep the
	// card a glance, not a list; the calendar is one click away for the rest.
	comingUpNightsShown  = 3
	comingUpWorldShown   = 3
	comingUpWaitingShown = 4
	// comingUpNightsWindowDays is how far ahead nights are looked for. The
	// sessions service refuses a window wider than 62 days.
	comingUpNightsWindowDays = 60
)

// comingUpCalendar is the slice of calendar.CalendarService the block reads.
type comingUpCalendar interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
	ListUpcomingEvents(ctx context.Context, calendarID, campaignID string, limit int, v permissions.Viewer) ([]calendar.Event, error)
}

// comingUpNights is the slice of sessions.SessionService the block reads:
// nights with the table's answers, open date polls, and the "confirm your
// times" ask that the Who's free planner stores.
type comingUpNights interface {
	ListGameNights(ctx context.Context, campaignID, from, to, today string, members []sessions.NightMember) ([]sessions.GameNight, error)
	ListProposalSummaries(ctx context.Context, campaignID, viewerID string) ([]sessions.ProposalSummary, error)
	GetMyAvailability(ctx context.Context, campaignID, userID string) (*sessions.MyAvailabilityResponse, error)
	LastConfirmAsk(ctx context.Context, campaignID string) (time.Time, error)
}

// comingUpMembers lists the campaign's members, the roster a night's
// "N of M" is counted over.
type comingUpMembers interface {
	ListMembers(ctx context.Context, campaignID string) ([]campaigns.CampaignMember, error)
}

// comingUpSources bundles what the composer reads. A nil member means that
// source is not wired and its section is left out.
type comingUpSources struct {
	Calendar comingUpCalendar
	Nights   comingUpNights
	Members  comingUpMembers
}

// comingUpRequest says who is looking and which sources the caller has
// already cleared (addon on, plugin healthy).
type comingUpRequest struct {
	CampaignID string
	Viewer     permissions.Viewer
	// Member is true only for a real campaign member: game nights, polls and
	// asks are the table's own business, never shown to a public visitor.
	Member   bool
	Calendar bool
	Nights   bool
	Now      time.Time
}

// comingUpRow is one line of the card, already worded.
type comingUpRow struct {
	// TileTop and TileBig fill the date tile: a weekday or month
	// abbreviation over a day number. TileIcon replaces TileBig for items
	// that have no date (a poll, a confirm ask).
	TileTop  string
	TileBig  string
	TileIcon string
	Title    string
	Sub      string
	// Waiting marks a row that needs the viewer.
	Waiting bool
	Href    string
	// Action is the button word on a row that needs the viewer.
	Action string
	// Primary makes the action the filled button; otherwise it is the quiet one.
	Primary bool
	// Answer is the viewer's own answer to a night ("Going", "Maybe",
	// "Can't go"), shown as a pill; AnswerTone picks its colour.
	Answer     string
	AnswerTone string
	// Chip is the event kind, shown as a small pill.
	Chip string
	// SessionID and Date name a game night, so the calendar can open it in
	// place instead of following Href.
	SessionID string
	Date      string
}

// comingUpView is the card's content. Sections with no rows are not drawn.
type comingUpView struct {
	// GameNightsHref is the Game nights page (the real-world calendar); it
	// is the card's link when game nights are on, else CalendarHref is.
	GameNightsHref string
	CalendarHref   string
	Nights         []comingUpRow
	Waiting        []comingUpRow
	World          []comingUpRow
	// Answered counts the sources that read successfully. Zero means nothing
	// could be said, so the block renders nothing instead of claiming the
	// future is empty.
	Answered int
}

// Empty reports that sources answered but none had anything to show.
func (v comingUpView) Empty() bool {
	return len(v.Nights) == 0 && len(v.Waiting) == 0 && len(v.World) == 0
}

// buildComingUp reads each enabled source independently and assembles the
// card. It never returns an error: a failed source is logged and skipped.
func buildComingUp(ctx context.Context, src comingUpSources, req comingUpRequest) comingUpView {
	now := req.Now.UTC()
	view := comingUpView{}
	userID := req.Viewer.UserID()
	base := "/campaigns/" + url.PathEscape(req.CampaignID)

	if req.Calendar && src.Calendar != nil {
		cal, rows, ok := comingUpWorld(ctx, src.Calendar, req)
		if ok {
			view.Answered++
			view.World = rows
		}
		if cal != nil {
			view.CalendarHref = base + "/calendars/" + url.PathEscape(cal.ID) + "/view"
		}
		if view.CalendarHref == "" && req.Member {
			view.CalendarHref = base + "/calendars"
		}
	}

	if req.Nights && req.Member && userID != "" && src.Nights != nil {
		view.GameNightsHref = base + "/game-nights"
		var waiting []comingUpRow
		if src.Members != nil {
			nights, ok := comingUpNightList(ctx, src, req, now)
			if ok {
				view.Answered++
				view.Nights, waiting = comingUpNightRows(nights, base, userID)
			}
		}
		if polls, ok := comingUpPolls(ctx, src.Nights, req, base); ok {
			view.Answered++
			waiting = append(waiting, polls...)
		}
		if ask, ok := comingUpConfirmAsk(ctx, src.Nights, req, base); ok {
			view.Answered++
			waiting = append(waiting, ask...)
		}
		if len(waiting) > comingUpWaitingShown {
			waiting = waiting[:comingUpWaitingShown]
		}
		view.Waiting = waiting
	}
	return view
}

// comingUpWorld reads the default calendar's next events through the same
// service call the dashboard's Upcoming Events card uses, so GM-only events
// and per-user rules are hidden by the calendar, not here. It returns the
// calendar (when one was found) for the card's link.
func comingUpWorld(ctx context.Context, svc comingUpCalendar, req comingUpRequest) (*calendar.Calendar, []comingUpRow, bool) {
	cal, err := svc.GetDefaultCalendarForViewer(ctx, req.CampaignID, req.Viewer)
	if err != nil {
		// A campaign with no calendar yet is ordinary: the section is simply
		// absent, and it still counts as answered.
		if isComingUpNotFound(err) {
			return nil, nil, true
		}
		logComingUpErr("world", req.CampaignID, err)
		return nil, nil, false
	}
	if cal == nil {
		return nil, nil, true
	}
	events, err := svc.ListUpcomingEvents(ctx, cal.ID, req.CampaignID, comingUpWorldShown, req.Viewer)
	if err != nil {
		logComingUpErr("calendar events", req.CampaignID, err)
		return cal, nil, false
	}
	today := cal.AbsoluteDay(cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
	rows := make([]comingUpRow, 0, len(events))
	for i := range events {
		e := &events[i]
		days := cal.AbsoluteDay(e.Year, e.Month, e.Day) - today
		rows = append(rows, comingUpRow{
			TileTop: monthAbbrev(cal.MonthName(e.Month)),
			TileBig: fmt.Sprintf("%d", e.Day),
			Title:   e.Name,
			Sub:     inDaysLabel(days),
			Chip:    strings.TrimSpace(e.KindName),
		})
	}
	return cal, rows, true
}

// comingUpNightList reads the table's nights for the next stretch, each with
// every member's answer.
func comingUpNightList(ctx context.Context, src comingUpSources, req comingUpRequest, now time.Time) ([]sessions.GameNight, bool) {
	list, err := src.Members.ListMembers(ctx, req.CampaignID)
	if err != nil {
		logComingUpErr("members", req.CampaignID, err)
		return nil, false
	}
	members := make([]sessions.NightMember, 0, len(list))
	for _, m := range list {
		members = append(members, sessions.NightMember{UserID: m.UserID, Name: m.DisplayName})
	}
	// A night stays "today" until its date has ended in every zone, the
	// sessions plugin's own rule, so one in progress still shows.
	t := now.Add(-14 * time.Hour)
	today := t.Format("2006-01-02")
	to := t.AddDate(0, 0, comingUpNightsWindowDays).Format("2006-01-02")
	nights, err := src.Nights.ListGameNights(ctx, req.CampaignID, today, to, today, members)
	if err != nil {
		logComingUpErr("game nights", req.CampaignID, err)
		return nil, false
	}
	return nights, true
}

// comingUpNightRows turns nights into the "Game nights" rows and the
// "Waiting on you" rows for this viewer. A night the viewer has not answered,
// or that moved after they answered, is waiting on them and goes only there.
// Past nights are skipped.
func comingUpNightRows(nights []sessions.GameNight, base, userID string) (shown, waiting []comingUpRow) {
	live := make([]sessions.GameNight, 0, len(nights))
	for _, n := range nights {
		if n.Past {
			continue
		}
		n.ForViewer(userID, false)
		live = append(live, n)
	}
	// ListGameNights groups by session; the card wants one timeline.
	sort.SliceStable(live, func(i, j int) bool {
		if live[i].Date != live[j].Date {
			return live[i].Date < live[j].Date
		}
		return live[i].Time < live[j].Time
	})
	for _, n := range live {
		href := fmt.Sprintf("%s/game-nights?session=%s&date=%s", base, url.QueryEscape(n.SessionID), url.QueryEscape(n.Date))
		tileTop, tileBig := nightTile(n.Date)
		title := n.Name
		if c := nightClock(n.Time); c != "" {
			title += " · " + c
		}
		mine := n.Mine
		owes := mine != nil && (mine.Answer == sessions.NightNoAnswer || mine.Recheck)

		// A night the viewer owes an answer on is listed once, under
		// "Waiting on you", so the card never shows the same night twice.
		if owes {
			sub := nightTallyLabel(n.Tally) + " · you haven't answered"
			if mine.Answer != sessions.NightNoAnswer {
				sub = nightTallyLabel(n.Tally) + " · moved since you answered"
			}
			waiting = append(waiting, comingUpRow{
				TileTop: tileTop, TileBig: tileBig, Title: title, Sub: sub,
				Waiting: true, Href: href, Action: "Answer", Primary: true,
				SessionID: n.SessionID, Date: n.Date,
			})
			continue
		}
		if len(shown) < comingUpNightsShown {
			answer, tone := nightAnswerPill(mine)
			shown = append(shown, comingUpRow{
				TileTop: tileTop, TileBig: tileBig, Title: title,
				Sub: nightTallyLabel(n.Tally), Href: href,
				Answer: answer, AnswerTone: tone,
				SessionID: n.SessionID, Date: n.Date,
			})
		}
	}
	return shown, waiting
}

// comingUpPolls lists open date polls the viewer has not answered yet.
func comingUpPolls(ctx context.Context, svc comingUpNights, req comingUpRequest, base string) ([]comingUpRow, bool) {
	list, err := svc.ListProposalSummaries(ctx, req.CampaignID, req.Viewer.UserID())
	if err != nil {
		logComingUpErr("date polls", req.CampaignID, err)
		return nil, false
	}
	var rows []comingUpRow
	for _, p := range list {
		if p.Proposal.Status != sessions.ProposalOpen || p.MyResponded {
			continue
		}
		rows = append(rows, comingUpRow{
			TileTop: "Poll", TileIcon: "fa-solid fa-chart-simple",
			Title:   p.Proposal.Title,
			Sub:     "Date poll · " + answeredCount(p.ResponderN),
			Waiting: true, Href: base + "/proposals/" + url.PathEscape(p.Proposal.ID),
			Action: "Vote", Primary: true,
		})
	}
	return rows, true
}

// comingUpConfirmAsk adds the "confirm your times" ask when the Director has
// asked everyone and this viewer's saved hours predate the ask. The test is
// the planner's own: a member who never saved hours is handled by its nudge,
// not by this ask.
func comingUpConfirmAsk(ctx context.Context, svc comingUpNights, req comingUpRequest, base string) ([]comingUpRow, bool) {
	asked, err := svc.LastConfirmAsk(ctx, req.CampaignID)
	if err != nil {
		logComingUpErr("confirm ask", req.CampaignID, err)
		return nil, false
	}
	if asked.IsZero() {
		return nil, true
	}
	mine, err := svc.GetMyAvailability(ctx, req.CampaignID, req.Viewer.UserID())
	if err != nil {
		logComingUpErr("my availability", req.CampaignID, err)
		return nil, false
	}
	answeredAt, perr := time.Parse(time.RFC3339, mine.AnsweredAt)
	if !mine.Answered || perr != nil || !answeredAt.Before(asked) {
		return nil, true
	}
	return []comingUpRow{{
		TileTop: "Times", TileIcon: "fa-regular fa-clock",
		Title:   "Confirm your times",
		Sub:     "Asked " + asked.UTC().Format("Jan 2") + " · are your hours still right?",
		Waiting: true, Href: base + "/availability", Action: "Answer", Primary: true,
	}}, true
}

// --- wording ---

// nightTile is the weekday abbreviation and day number of a YYYY-MM-DD date.
func nightTile(date string) (top, big string) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", ""
	}
	return t.Format("Mon"), fmt.Sprintf("%d", t.Day())
}

// nightClock words a stored "HH:MM" as "7 pm" or "7:30 pm"; "" for none.
func nightClock(hhmm string) string {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return ""
	}
	if t.Minute() == 0 {
		return strings.ToLower(t.Format("3 pm"))
	}
	return strings.ToLower(t.Format("3:04 pm"))
}

// nightTallyLabel is "4 of 5 coming" over everyone counted. Members the
// organizer left out of the count are already excluded from the tally.
func nightTallyLabel(t sessions.NightTally) string {
	return fmt.Sprintf("%d of %d coming", t.Going, t.Going+t.Maybe+t.Cant+t.NoAnswer)
}

// nightAnswerPill words the viewer's own answer to a night they have
// answered, with the pill's tone; nothing for a night with no answer.
func nightAnswerPill(mine *sessions.NightAnswer) (label, tone string) {
	switch {
	case mine == nil || mine.Answer == sessions.NightNoAnswer:
		return "", ""
	case mine.Answer == sessions.NightYes:
		return "Going", "go"
	case mine.Answer == sessions.NightMaybe:
		return "Maybe", "maybe"
	default:
		return "Can't go", "cant"
	}
}

// answeredCount words how many people have answered a poll.
func answeredCount(n int) string {
	if n == 1 {
		return "1 answer so far"
	}
	return fmt.Sprintf("%d answers so far", n)
}

// monthAbbrev is the first three letters of a month name, for the tile.
func monthAbbrev(name string) string {
	r := []rune(strings.TrimSpace(name))
	if len(r) > 3 {
		r = r[:3]
	}
	return string(r)
}

// inDaysLabel words a distance in in-world days.
func inDaysLabel(days int) string {
	switch {
	case days <= 0:
		return "Today"
	case days == 1:
		return "Tomorrow"
	default:
		return fmt.Sprintf("In %d days", days)
	}
}

// isComingUpNotFound reports the normal "no calendar yet / not visible to
// you" answer.
func isComingUpNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

func logComingUpErr(what, campaignID string, err error) {
	slog.Warn("coming up: source unavailable",
		slog.String("source", what), slog.String("campaign_id", campaignID), slog.Any("error", err))
}

// --- handler ---

// comingUpAddons is the slice of addons.AddonService the handler needs.
type comingUpAddons interface {
	IsEnabledForCampaign(ctx context.Context, campaignID, slug string) (bool, error)
}

// comingUpHandler serves the block's fragment. It holds no logic beyond
// gating and rendering; buildComingUp does the work.
type comingUpHandler struct {
	src     comingUpSources
	addons  comingUpAddons
	healthy func(slug string) bool
	now     func() time.Time
}

// Show renders the card for the campaign in the URL. An empty body (no source
// available or answering) makes the dashboard drop the block's placeholder.
// GET /campaigns/:id/coming-up
func (h *comingUpHandler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx, cancel := context.WithTimeout(c.Request().Context(), comingUpTimeout)
	defer cancel()

	// An addon-check failure counts as off: the card is an extra, never worth
	// showing data from a feature the owner may have switched off.
	on := func(slug string) bool {
		ok, err := h.addons.IsEnabledForCampaign(ctx, cc.Campaign.ID, slug)
		return err == nil && ok && (h.healthy == nil || h.healthy(slug))
	}
	calendarOn := on(calendar.PluginSlug)
	req := comingUpRequest{
		CampaignID: cc.Campaign.ID,
		Viewer:     permissions.RequestViewer(cc.VisibilityRole(), auth.GetUserID(c)),
		Member:     cc.IsMember,
		Calendar:   calendarOn,
		// Game nights need both switches, as the sessions routes do.
		Nights: calendarOn && on(sessions.SessionsAddonSlug),
		Now:    h.now(),
	}
	if !req.Calendar && !req.Nights {
		return c.HTML(http.StatusOK, "")
	}
	view := buildComingUp(ctx, h.src, req)
	if view.Answered == 0 {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, comingUpFragment(cc.Campaign.ID, view))
}

// comingUpWaitingItem is one "waiting on you" row as the calendar's button
// reads it. A night carries its session and date so the calendar opens it in
// place; anything else is followed by its href.
type comingUpWaitingItem struct {
	Title     string `json:"title"`
	Sub       string `json:"sub"`
	Action    string `json:"action"`
	Href      string `json:"href"`
	TileTop   string `json:"tileTop"`
	TileBig   string `json:"tileBig,omitempty"`
	TileIcon  string `json:"tileIcon,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Date      string `json:"date,omitempty"`
}

// Waiting lists what the viewer owes the table, for the "waiting on you"
// button on the Game nights calendar. Members only; anyone else, or a
// campaign without game nights, gets an empty list.
// GET /campaigns/:id/coming-up/waiting
func (h *comingUpHandler) Waiting(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx, cancel := context.WithTimeout(c.Request().Context(), comingUpTimeout)
	defer cancel()
	items := []comingUpWaitingItem{}
	userID := auth.GetUserID(c)
	on := func(slug string) bool {
		ok, err := h.addons.IsEnabledForCampaign(ctx, cc.Campaign.ID, slug)
		return err == nil && ok && (h.healthy == nil || h.healthy(slug))
	}
	if !cc.IsMember || userID == "" || !on(calendar.PluginSlug) || !on(sessions.SessionsAddonSlug) {
		return c.JSON(http.StatusOK, items)
	}
	view := buildComingUp(ctx, comingUpSources{Nights: h.src.Nights, Members: h.src.Members}, comingUpRequest{
		CampaignID: cc.Campaign.ID,
		Viewer:     permissions.RequestViewer(cc.VisibilityRole(), userID),
		Member:     true,
		Nights:     true,
		Now:        h.now(),
	})
	for _, r := range view.Waiting {
		items = append(items, comingUpWaitingItem{
			Title: r.Title, Sub: r.Sub, Action: r.Action, Href: r.Href,
			TileTop: r.TileTop, TileBig: r.TileBig, TileIcon: r.TileIcon,
			SessionID: r.SessionID, Date: r.Date,
		})
	}
	return c.JSON(http.StatusOK, items)
}

// registerComingUpRoutes mounts the block's fragment. Public-capable like the
// other dashboard fragments; what a visitor may see is decided per source.
func registerComingUpRoutes(e *echo.Echo, h *comingUpHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	g := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	g.GET("/coming-up", h.Show, campaigns.RequireViewAccess())
	g.GET("/coming-up/waiting", h.Waiting, campaigns.RequireViewAccess())
}
