package app

// The campaign home's "Since you were last here" block: what changed in the
// campaign since this member last pressed "Mark as seen". It lives in
// internal/app because the news comes from the entities, quests and sessions
// plugins, which the campaign home may not import.
//
// Rules the block keeps:
//   - Members only. A guest, or anyone not signed in, is answered empty.
//   - Every count and name comes from a read that already filters for this
//     viewer (pages by the entity visibility predicate, quests by the page
//     gate and the DM's hidden notices, recaps with the DM-only parts taken
//     out, nights and polls which any member may see). Nothing is counted
//     first and trimmed later, so a hidden thing adds to no number.
//   - The marker moves only when the member presses the button. Loading the
//     home never moves it, or the list would be gone before it was read.
//   - A source that fails leaves out its own row. If every row is missing
//     because a source failed, the block is left out too: it never says "all
//     caught up" on a picture it could not finish.
//   - The view holds finished strings only.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

const (
	// sinceLookBack caps how far back the block reads, so a member who has
	// been away for months is not shown a wall of old news.
	sinceLookBack = 30 * 24 * time.Hour
	// sinceRowsShown is how many rows the block lists, newest first.
	sinceRowsShown = 5
	// sinceNamesShown is how many things a row names before "and N more".
	sinceNamesShown = 3
)

// sincePages reads new pages for one viewer.
type sincePages interface {
	Since(ctx context.Context, campaignID string, since time.Time, role int, userID string, nameLimit int) (entities.PagesSince, error)
}

// sinceNights reads game-night news.
type sinceNights interface {
	RecapsSince(ctx context.Context, campaignID string, since time.Time, v sessions.SinceViewer) ([]sessions.SinceItem, error)
	NightsSince(ctx context.Context, campaignID string, since time.Time, today string) ([]sessions.SinceItem, error)
	PollsSince(ctx context.Context, campaignID string, since time.Time) ([]sessions.SinceItem, error)
}

// sinceQuests reads quest news for one viewer.
type sinceQuests interface {
	ChangesSince(ctx context.Context, campaignID string, v quests.Viewer, since time.Time) (*quests.QuestChanges, error)
}

// sinceSources bundles what the block reads. A nil source is left out.
type sinceSources struct {
	Pages  sincePages
	Nights sinceNights
	Quests sinceQuests
	// NightsOn says game nights are switched on and healthy for the
	// campaign: recaps, nights and polls all need it.
	NightsOn func(ctx context.Context, campaignID string) bool
}

// sinceRequest says who is looking and from when.
type sinceRequest struct {
	CampaignID string
	UserID     string
	// Role is the visibility role (a player's while the owner views as one).
	Role int
	// DMTeam is the owner or a Co-DM not previewing as a player.
	DMTeam bool
	Quests quests.Viewer
	// From is the effective "seen up to" time; Now the request's clock.
	From time.Time
	Now  time.Time
	Loc  *time.Location
}

// sinceRow is one group, already worded.
type sinceRow struct {
	Bold string
	Sub  string
	Day  string
	Href string
	// at orders the rows, newest first; it is never drawn.
	at time.Time
}

// sinceView is what the template draws. Rows is empty for "caught up".
type sinceView struct {
	Rows     []sinceRow
	CaughtUp string
	SeenHref string
	// SeenVals is the JSON body the button sends: the moment this list was
	// drawn, so news that arrives while it is open is not marked seen.
	SeenVals string
}

// sinceEffectiveFrom is where the block reads from: the member's marker or
// join time, no more than thirty days back and never in the future.
func sinceEffectiveFrom(seenUpTo, now time.Time) time.Time {
	from := seenUpTo.UTC()
	if floor := now.Add(-sinceLookBack); from.Before(floor) {
		from = floor
	}
	if from.After(now) {
		from = now
	}
	return from
}

// sinceDay words a moment as a short weekday, or a date once it is a week or
// more old, in the viewer's own zone.
func sinceDay(at, now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	a, n := at.In(loc), now.In(loc)
	if n.Sub(a) >= 7*24*time.Hour {
		if a.Year() != n.Year() {
			return a.Format("2 Jan 2006")
		}
		return a.Format("2 Jan")
	}
	return a.Format("Mon")
}

// sinceCaughtUp is the line shown with nothing new: the weekday of the last
// visit, a date once that is a week old, and just "You're all caught up" when
// the visit was today.
func sinceCaughtUp(from, now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	f, n := from.In(loc), now.In(loc)
	switch {
	case f.Format("2006-01-02") == n.Format("2006-01-02"):
		return "You're all caught up"
	case n.Sub(f) >= 7*24*time.Hour:
		if f.Year() != n.Year() {
			return "You're all caught up since " + f.Format("2 January 2006")
		}
		return "You're all caught up since " + f.Format("2 January")
	}
	return "You're all caught up since " + f.Format("Monday")
}

// sinceCount words a group by its count: one reads as the singular phrase.
func sinceCount(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}

// sinceNames lists up to three names, then "and N more" for the rest of total.
func sinceNames(names []string, total int) string {
	var shown []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && len(shown) < sinceNamesShown {
			shown = append(shown, n)
		}
	}
	out := strings.Join(shown, ", ")
	if more := total - len(shown); more > 0 && len(shown) > 0 {
		out += fmt.Sprintf(" and %d more", more)
	}
	return out
}

// sinceItemNames is the names of a night or poll list.
func sinceItemNames(items []sessions.SinceItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

// sinceBuild reads every source and words the rows, newest group first, at
// most five. failed is true when a source could not be read, so the caller
// does not call an empty result "caught up".
func sinceBuild(ctx context.Context, src sinceSources, req sinceRequest) (rows []sinceRow, failed bool) {
	base := "/campaigns/" + url.PathEscape(req.CampaignID)
	loc := req.Loc
	if loc == nil {
		loc = time.UTC
	}
	add := func(r sinceRow) {
		r.Day = sinceDay(r.at, req.Now, loc)
		rows = append(rows, r)
	}
	fail := func(what string, err error) {
		failed = true
		logComingUpErr("since last here: "+what, req.CampaignID, err)
	}

	if src.Pages != nil {
		if p, err := src.Pages.Since(ctx, req.CampaignID, req.From, req.Role, req.UserID, sinceNamesShown); err != nil {
			fail("pages", err)
		} else if p.Count > 0 && len(p.Newest) > 0 {
			names := make([]string, len(p.Newest))
			for i, n := range p.Newest {
				names[i] = n.Name
			}
			href := base + "/entities"
			if p.Count == 1 {
				href = base + "/entities/" + url.PathEscape(p.Newest[0].ID)
			}
			add(sinceRow{Bold: sinceCount(p.Count, "1 new page", "%d new pages"), Sub: sinceNames(names, p.Count), Href: href, at: p.Newest[0].CreatedAt})
		}
	}

	if src.Quests != nil {
		if q, err := src.Quests.ChangesSince(ctx, req.CampaignID, req.Quests, req.From); err != nil {
			fail("quests", err)
		} else {
			board := ""
			if q.TypeSlug != "" {
				board = base + "/" + url.PathEscape(q.TypeSlug)
			}
			group := func(list []quests.QuestChange, one, many string) {
				if len(list) == 0 {
					return
				}
				names := make([]string, len(list))
				for i, c := range list {
					names[i] = c.Title
				}
				href := board
				if len(list) == 1 {
					href = base + "/entities/" + url.PathEscape(list[0].EntityID)
				}
				add(sinceRow{Bold: sinceCount(len(list), one, many), Sub: sinceNames(names, len(list)), Href: href, at: list[0].Saved})
			}
			group(q.New, "New quest", "%d new quests")
			group(q.Finished, "Quest finished", "%d quests finished")
		}
	}

	if src.Nights != nil && src.NightsOn != nil && src.NightsOn(ctx, req.CampaignID) {
		if list, err := src.Nights.RecapsSince(ctx, req.CampaignID, req.From, sessions.SinceViewer{DMTeam: req.DMTeam}); err != nil {
			fail("recaps", err)
		} else if len(list) > 0 {
			href := base + "/sessions"
			if len(list) == 1 {
				href = base + "/sessions/" + url.PathEscape(list[0].ID)
			}
			add(sinceRow{Bold: sinceCount(len(list), "Recap posted", "%d recaps posted"), Sub: sinceNames(sinceItemNames(list), len(list)), Href: href, at: list[0].At})
		}
		today := req.Now.In(loc).Format("2006-01-02")
		if list, err := src.Nights.NightsSince(ctx, req.CampaignID, req.From, today); err != nil {
			fail("nights", err)
		} else if len(list) > 0 {
			href := base + "/sessions"
			sub := sinceNames(sinceItemNames(list), len(list))
			if len(list) == 1 {
				href = base + "/sessions/" + url.PathEscape(list[0].ID)
				if d := glanceDateLabel(&list[0].Date, req.Now); d != "" {
					sub = strings.TrimSpace(list[0].Name) + ", " + d
				}
			}
			add(sinceRow{Bold: sinceCount(len(list), "New game night set", "%d new game nights set"), Sub: sub, Href: href, at: list[0].At})
		}
		if list, err := src.Nights.PollsSince(ctx, req.CampaignID, req.From); err != nil {
			fail("polls", err)
		} else if len(list) > 0 {
			href := base + "/proposals"
			if len(list) == 1 {
				href = base + "/proposals/" + url.PathEscape(list[0].ID)
			}
			add(sinceRow{Bold: sinceCount(len(list), "A date poll was opened", "%d date polls opened"), Sub: sinceNames(sinceItemNames(list), len(list)), Href: href, at: list[0].At})
		}
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].at.After(rows[j].at) })
	if len(rows) > sinceRowsShown {
		rows = rows[:sinceRowsShown]
	}
	return rows, failed
}

// --- handler ---

// sinceHandler serves the block and its button.
type sinceHandler struct {
	src     sinceSources
	markers campaigns.HomeSeenStore
	now     func() time.Time
	// zone is the viewer's stored zone, or nil (UTC) when unknown.
	zone func(ctx context.Context, userID string) *time.Location
}

func (h *sinceHandler) seenHref(campaignID string) string {
	return "/campaigns/" + url.PathEscape(campaignID) + "/since-last-here/seen"
}

// Show draws the block, or an empty body that removes the placeholder for a
// guest, for a read it could not finish, and for a member whose marker cannot
// be read.
// GET /campaigns/:id/since-last-here
func (h *sinceHandler) Show(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	if !cc.IsMember || userID == "" {
		return c.HTML(http.StatusOK, "")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), comingUpTimeout)
	defer cancel()
	campaignID := cc.Campaign.ID

	seen, ok, err := h.markers.Get(ctx, campaignID, userID)
	if err != nil || !ok {
		if err != nil {
			logComingUpErr("since last here: marker", campaignID, err)
		}
		return c.HTML(http.StatusOK, "")
	}
	now := h.now().UTC()
	from := sinceEffectiveFrom(seen.SeenUpTo(), now)
	var loc *time.Location
	if h.zone != nil {
		loc = h.zone(ctx, userID)
	}

	// The owner previewing as a player is read as a player.
	asPlayer := campaigns.ViewingAsPlayer(c, cc)
	req := sinceRequest{
		CampaignID: campaignID,
		UserID:     userID,
		Role:       int(cc.VisibilityRole()),
		DMTeam:     cc.CanAuthorDmOnly() && !asPlayer,
		Quests: quests.Viewer{
			UserID:         userID,
			MemberRole:     int(cc.MemberRole),
			VisibilityRole: cc.VisibilityRole(),
			IsDM:           cc.CanControlWorldState(),
		},
		From: from,
		Now:  now,
		Loc:  loc,
	}
	if asPlayer {
		req.Role = int(campaigns.RolePlayer)
		req.Quests.IsDM, req.Quests.MemberRole, req.Quests.VisibilityRole = false, int(campaigns.RolePlayer), int(campaigns.RolePlayer)
	}

	rows, failed := sinceBuild(ctx, h.src, req)
	if len(rows) == 0 && failed {
		return c.HTML(http.StatusOK, "")
	}
	v := sinceView{Rows: rows}
	if len(rows) == 0 {
		v.CaughtUp = sinceCaughtUp(from, now, loc)
	} else {
		v.SeenHref = h.seenHref(campaignID)
		v.SeenVals = `{"upto":"` + strconv.FormatInt(now.Unix(), 10) + `"}`
	}
	return middleware.Render(c, http.StatusOK, sinceLastHereBlock(v))
}

// Seen moves the marker to the moment the member's list was drawn (the
// "upto" the button sends, never later than now and never earlier than the
// marker already is) and swaps the block for the caught-up line.
// POST /campaigns/:id/since-last-here/seen
func (h *sinceHandler) Seen(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	if !cc.IsMember || userID == "" {
		return apperror.NewForbidden("only members of this campaign can mark news as seen")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), comingUpTimeout)
	defer cancel()
	now := h.now().UTC()
	at := now
	if raw := c.FormValue("upto"); raw != "" {
		if secs, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if t := time.Unix(secs, 0).UTC(); t.Before(now) {
				at = t
			}
		}
	}
	ok, err := h.markers.Mark(ctx, cc.Campaign.ID, userID, at)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok {
		return apperror.NewForbidden("only members of this campaign can mark news as seen")
	}
	var loc *time.Location
	if h.zone != nil {
		loc = h.zone(ctx, userID)
	}
	return middleware.Render(c, http.StatusOK, sinceLastHereBlock(sinceView{CaughtUp: sinceCaughtUp(at, now, loc)}))
}

// registerSinceRoutes mounts the block's fragment (public-capable like the
// home; the handler answers a visitor empty) and its member-only button.
func registerSinceRoutes(e *echo.Echo, h *sinceHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/since-last-here", h.Show, campaigns.RequireViewAccess())

	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.POST("/since-last-here/seen", h.Seen, campaigns.RequireRole(campaigns.RolePlayer))
}

// sinceNightsOn builds the NightsOn check: both addons on and both plugins up.
func sinceNightsOn(addons comingUpAddons, healthy func(slug string) bool) func(ctx context.Context, campaignID string) bool {
	return func(ctx context.Context, campaignID string) bool {
		for _, slug := range []string{calendar.PluginSlug, sessions.SessionsAddonSlug} {
			ok, err := addons.IsEnabledForCampaign(ctx, campaignID, slug)
			if err != nil || !ok || (healthy != nil && !healthy(slug)) {
				return false
			}
		}
		return true
	}
}
