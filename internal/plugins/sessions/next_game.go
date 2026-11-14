package sessions

// The campaign home's "Next game" block: how long until the next set game
// night, as one big number, with the viewer's own answer beside it.
//
// It reads the same nights as "When we could play" and reuses its placement
// of a night in the viewer's day, so the two blocks never disagree about when
// a night falls. Everything the template draws is already worded here; a
// guest of a public campaign gets a date and a time and nothing else.

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

const (
	// nextGameDays is how far ahead the next night is looked for. The window
	// the nights service accepts is 62 days wide, and this plus the day of
	// slack either side stays inside it.
	nextGameDays = 60
	// nextGameEvening is the hour from which "today" reads "Tonight".
	nextGameEvening = 17
)

// nextGameView is what the template draws.
type nextGameView struct {
	// Set is false when no night is planned.
	Set   bool
	Guest bool
	// Count and Unit are the big number ("3", "days"); both are empty on the
	// day itself, when Today holds the words instead.
	Count, Unit string
	Today       string
	// Title is empty for a guest, who is told when and nothing more.
	Title string
	// When is the date and time, "Thu 19 Nov, 7 pm".
	When string
	// Answer, Tone and Owes are the viewer's own answer; Href is where to
	// give one.
	Answer, Tone   string
	Owes           bool
	Href           string
	GameNightsHref string
}

// buildNextGame words the first of nights, which must already be placed in
// the viewer's days and sorted. today is the viewer's own date.
func buildNextGame(nights []couldPlayNightIn, today string, guest bool, gameNightsHref string) nextGameView {
	v := nextGameView{Guest: guest, GameNightsHref: gameNightsHref}
	if len(nights) == 0 {
		return v
	}
	n := nights[0]
	d, err := time.Parse("2006-01-02", n.Date)
	if err != nil {
		return v
	}
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return v
	}
	v.Set = true
	v.Title = n.Title
	v.When = d.Format("Mon 2 Jan")
	if n.Clock != "" {
		v.When += ", " + n.Clock
	}
	v.Answer, v.Tone, v.Owes, v.Href = n.Answer, n.Tone, n.Owes, n.Href

	days := int(d.Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		word := "Today"
		if n.Hour >= nextGameEvening {
			word = "Tonight"
		}
		if n.Clock != "" {
			word += ", " + n.Clock
		}
		v.Today = word
	case days == 1:
		v.Count, v.Unit = "1", "day"
	default:
		v.Count, v.Unit = fmt.Sprint(days), "days"
	}
	return v
}

// NextGameBlock draws the block, or an empty answer that removes the
// placeholder when the nights cannot be read. A guest of a public campaign
// gets it only where the owner placed the block on the Guests home.
// GET /campaigns/:id/next-game
func (h *Handler) NextGameBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	guest := !cc.IsMember
	// The route is reachable on any public campaign, so placement is the
	// owner's consent to show a visitor game nights.
	userID := auth.GetUserID(c)
	if !campaigns.BlockVisibleTo(ctx, cc, h.placement, campaigns.BlockNextGame, c.QueryParam("on"), userID) {
		return c.HTML(http.StatusOK, "")
	}
	tz := "UTC"
	if !guest {
		tz = h.resolveViewerTZ(c, userID)
	}
	loc := timeutil.LoadLocation(tz)
	now := time.Now()
	today := now.In(loc).Format("2006-01-02")
	todayDate, err := timeutil.ParseCivilDate(today)
	if err != nil {
		return c.HTML(http.StatusOK, "")
	}
	last := todayDate.AddDays(nextGameDays - 1).String()
	base := "/campaigns/" + url.PathEscape(cc.Campaign.ID)

	var members []NightMember
	if !guest && h.memberLister != nil {
		list, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID)
		if err != nil {
			return c.HTML(http.StatusOK, "")
		}
		for _, m := range list {
			members = append(members, NightMember{UserID: m.UserID, Name: m.DisplayName})
		}
	}
	from := todayDate.AddDays(-1).String()
	to := todayDate.AddDays(nextGameDays).String()
	nights, err := h.svc.ListGameNights(ctx, cc.Campaign.ID, from, to, today, members)
	if err != nil {
		return c.HTML(http.StatusOK, "")
	}
	placed := couldPlayNightsFrom(nights, base, userID, loc, guest, now, today, last)
	return middleware.Render(c, http.StatusOK, nextGameBlock(buildNextGame(placed, today, guest, base+"/game-nights")))
}
