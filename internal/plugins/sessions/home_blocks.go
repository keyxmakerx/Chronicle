package sessions

// The campaign home's "When we play" and "Last time" blocks. The home lives in
// the campaigns plugin, which may not read game nights, so its placeholders
// load these fragments. Both are for members only and need game nights on, as
// the sessions routes do: a visitor to a public campaign never reaches them.

import (
	"fmt"
	"html"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// --- When we play ---

const (
	// playTimesShown is how many windows the block lists.
	playTimesShown = 3
	// playTimeMinHours is the shortest window worth listing: a single free
	// hour is not a game night.
	playTimeMinHours = 2
)

// playTime is one window most of the table is free, in the viewer's zone.
type playTime struct {
	Weekday time.Weekday
	// Start and End are hours of the day, End exclusive (24 is midnight).
	Start, End int
	// Free is how many members are free for all of the window, of Total.
	Free, Total int
}

// Day is the weekday as the block words it: "Thursdays".
func (p playTime) Day() string { return p.Weekday.String() + "s" }

// Span is the hours as "7 to 10 pm", or "11 am to 2 pm" across noon.
func (p playTime) Span() string {
	meridiem := func(h int) string {
		if h%24 < 12 {
			return "am"
		}
		return "pm"
	}
	clock := func(h int) string {
		h %= 24
		switch {
		case h == 0:
			return "midnight"
		case h == 12:
			return "noon"
		case h > 12:
			return fmt.Sprint(h - 12)
		}
		return fmt.Sprint(h)
	}
	end := clock(p.End)
	if p.End%24 != 0 && p.End%24 != 12 {
		end += " " + meridiem(p.End)
	}
	start := clock(p.Start)
	if p.Start%24 != 0 && p.Start%24 != 12 && (meridiem(p.Start) != meridiem(p.End) || p.End == 24) {
		start += " " + meridiem(p.Start)
	}
	return start + " to " + end
}

// Words is how much of the table is free, in words. A count would let a
// reader work out who is missing from a small table.
func (p playTime) Words() string {
	if availabilityStep(p.Free, p.Total) == stepMost {
		return "Most of the table"
	}
	return "About half the table"
}

// Percent is the bar's fill: two steps, matching Words, never an exact share.
func (p playTime) Percent() int {
	if availabilityStep(p.Free, p.Total) == stepMost {
		return 100
	}
	return 50
}

// playTimesFrom finds the windows most of the table is free from anonymous
// per-hour counts. Only days from today on count; a weekday that appears in
// two weeks keeps its first appearance. A window is a run of hours in which
// more than half the table is free; its Free is the fewest free in any hour of
// it, so its words hold for the whole window. The best windows (most
// free, then longest) come first, at most playTimesShown of them. Nothing but
// counts is read, so nobody's own answers can show.
func playTimesFrom(weeks []WeekOverlay, today string) []playTime {
	var all []playTime
	seen := map[time.Weekday]bool{}
	for _, w := range weeks {
		// A table this small would point at a person, as in When we could play.
		if w.TotalMembers <= couldPlaySmallTable {
			continue
		}
		for _, d := range w.Days {
			if d.Date < today || seen[time.Weekday(d.Weekday)] {
				continue
			}
			best, ok := bestRun(d, w.TotalMembers)
			if !ok {
				continue
			}
			seen[time.Weekday(d.Weekday)] = true
			all = append(all, best)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Free != b.Free {
			return a.Free > b.Free
		}
		if la, lb := a.End-a.Start, b.End-b.Start; la != lb {
			return la > lb
		}
		return false
	})
	if len(all) > playTimesShown {
		all = all[:playTimesShown]
	}
	return all
}

// bestRun is the best window of one day: the run of hours with more than half
// the table free (at least playTimeMinHours long) that has the most free, then
// the longest.
func bestRun(d OverlayDay, total int) (playTime, bool) {
	var best playTime
	found := false
	start := -1
	minFree := 0
	flush := func(end int) {
		if start < 0 || end-start < playTimeMinHours {
			start = -1
			return
		}
		c := playTime{Weekday: time.Weekday(d.Weekday), Start: start, End: end, Free: minFree, Total: total}
		if !found || c.Free > best.Free || (c.Free == best.Free && c.End-c.Start > best.End-best.Start) {
			best, found = c, true
		}
		start = -1
	}
	for h := 0; h < len(d.Hours) && h < 24; h++ {
		free := d.Hours[h].Free
		if free*2 > total {
			if start < 0 {
				start, minFree = h, free
			} else if free < minFree {
				minFree = free
			}
			continue
		}
		flush(h)
	}
	flush(min(len(d.Hours), 24))
	return best, found
}

// WhenWePlayBlock draws the windows most of the table is free, or an empty
// answer that removes the placeholder when nobody has said enough yet.
// GET /campaigns/:id/when-we-play
func (h *Handler) WhenWePlayBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	tz := h.resolveViewerTZ(c, userID)
	loc := timeutil.LoadLocation(tz)
	today := time.Now().In(loc)
	todayStr := today.Format("2006-01-02")
	monday, err := timeutil.ParseCivilDate(todayStr)
	if err != nil {
		return c.HTML(http.StatusOK, "")
	}
	var weeks []WeekOverlay
	for _, offset := range []int{0, 7} {
		// Never the detail form: this block is for every member.
		o, err := h.CampaignWeekOverlay(ctx, cc.Campaign.ID, monday.AddDays(offset).String(), tz, false)
		if err != nil {
			return c.HTML(http.StatusOK, "")
		}
		weeks = append(weeks, *o)
	}
	times := playTimesFrom(weeks, todayStr)
	if len(times) == 0 {
		return c.HTML(http.StatusOK, "")
	}
	zone := timeutil.ZoneAbbrevOrEmpty(tz, today)
	if zone == "" {
		zone = tz
	}
	return middleware.Render(c, http.StatusOK, whenWePlayBlock(cc.Campaign.ID, times, zone))
}

// --- Last time ---

// recapExcerptLen caps the excerpt: the block leads into the recap, it is not
// the recap.
const recapExcerptLen = 420

var (
	recapBlockBreak = regexp.MustCompile(`(?i)</(?:p|li|div|blockquote|pre|h[1-6])>|<br\s*/?>`)
	recapHeading    = regexp.MustCompile(`(?i)^\s*<h[1-6][\s>]`)
	recapTag        = regexp.MustCompile(`<[^>]*>`)
)

// recapExcerpt is the opening of a served recap as plain text: whole
// paragraphs while they fit the cap, the first one cut at a word when it does
// not. Headings are skipped. The caller serves the HTML first, so secrets and
// hidden page names are already gone.
func recapExcerpt(recapHTML string) string {
	var out []string
	length := 0
	for _, chunk := range recapBlockBreak.Split(recapHTML, -1) {
		if recapHeading.MatchString(chunk) {
			continue
		}
		plain := strings.Join(strings.Fields(html.UnescapeString(recapTag.ReplaceAllString(chunk, " "))), " ")
		if plain == "" {
			continue
		}
		if length+len([]rune(plain)) > recapExcerptLen {
			if len(out) == 0 {
				r := []rune(plain)
				cut := string(r[:recapExcerptLen])
				if i := strings.LastIndex(cut, " "); i > recapExcerptLen/2 {
					cut = cut[:i]
				}
				out = append(out, strings.TrimRight(cut, " ,;:")+"…")
			}
			break
		}
		out = append(out, plain)
		length += len([]rune(plain))
	}
	return strings.Join(out, "\n\n")
}

// lastTimeView is what the block draws.
type lastTimeView struct {
	Name    string
	Date    string
	Excerpt string
	Href    string
}

// LastTimeBlock draws the latest night's recap as the viewer may read it, or
// an empty answer that removes the placeholder when no recap was written.
// GET /campaigns/:id/last-time
func (h *Handler) LastTimeBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	loc := timeutil.LoadLocation(h.resolveViewerTZ(c, auth.GetUserID(c)))
	night, err := h.svc.LastGameNight(ctx, cc.Campaign.ID, time.Now().In(loc).Format("2006-01-02"))
	if err != nil || night == nil {
		return c.HTML(http.StatusOK, "")
	}
	// The list read may be thinner than the page's; read the night itself.
	s, err := h.svc.GetSession(ctx, night.ID)
	if err != nil || s == nil || s.CampaignID != cc.Campaign.ID || !hasRecap(s) {
		return c.HTML(http.StatusOK, "")
	}
	// The Director previewing as a player reads what a player reads: serve the
	// recap for a context with no DM standing.
	served := cc
	if campaigns.ViewingAsPlayer(c, cc) {
		pc := *cc
		pc.MemberRole = campaigns.RolePlayer
		pc.IsDmGranted = false
		served = &pc
	}
	h.serveRecap(c, served, s)
	excerpt := ""
	if s.RecapHTML != nil {
		excerpt = recapExcerpt(*s.RecapHTML)
	}
	if excerpt == "" {
		return c.HTML(http.StatusOK, "")
	}
	view := lastTimeView{
		Name:    s.Name,
		Date:    glanceDay(s.ScheduledDate, time.Now().In(loc)),
		Excerpt: excerpt,
		Href:    "/campaigns/" + cc.Campaign.ID + "/sessions/" + s.ID,
	}
	return middleware.Render(c, http.StatusOK, lastTimeBlock(view))
}

// glanceDay words a session's YYYY-MM-DD date as "12 Nov", adding the year
// when it is not this year's; "" for no date.
func glanceDay(date *string, now time.Time) string {
	if date == nil {
		return ""
	}
	d, err := time.Parse("2006-01-02", *date)
	if err != nil {
		return ""
	}
	if d.Year() != now.Year() {
		return d.Format("2 Jan 2006")
	}
	return d.Format("2 Jan")
}
