package sessions

// The campaign home's "When we could play" block: one row per upcoming day
// with a noon-to-midnight bar showing the table's set game night, and soft
// shading where the table tends to be free.
//
// The block is for glancing, so it never reveals who is free. Availability is
// read only as anonymous per-hour counts and reduced at once to three coarse
// steps (nothing, about half, most); the counts, names and poll tallies never
// reach the view types, so no template can print them. A guest of a public
// campaign sees set nights as a date and a time and nothing else.

import (
	"fmt"
	"net/http"
	"net/url"
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

const (
	// couldPlayDays is how far ahead the block looks.
	couldPlayDays = 35
	// couldPlayCollapsed is how many rows show before "Show N more days".
	couldPlayCollapsed = 3
	// couldPlaySmallTable is the largest table that gets no availability
	// words or shading: with so few people, a step names them.
	couldPlaySmallTable = 3
	// couldPlayNightHours is how long a set night is drawn for. Nights store a
	// start time only, so the bar shows a typical evening, not a promise.
	couldPlayNightHours = 4
	// couldPlayAxisStart and couldPlayAxisHours frame the bar: noon to midnight.
	couldPlayAxisStart = 12
	couldPlayAxisHours = 12
	// couldPlayMinRun is the shortest stretch worth shading, as for When we play.
	couldPlayMinRun = 2
	// couldPlayMaxAlpha is the shade's strength, in percent, at the darkest.
	couldPlayMaxAlpha = 60
)

// The coarse steps an hour of the table's freedom is reduced to.
const (
	stepNone = 0
	stepHalf = 1
	stepMost = 2
)

// availabilityStep reduces an hour's anonymous free count to a step. Most is
// three quarters or more and is never worded "everyone": a full table and an
// almost full one read the same, so a lone absentee cannot be inferred.
func availabilityStep(free, total int) int {
	switch {
	case total <= 0:
		return stepNone
	case free*4 >= total*3:
		return stepMost
	case free*2 >= total:
		return stepHalf
	}
	return stepNone
}

// couldPlayNight is one set night as the block draws it.
type couldPlayNight struct {
	// Title is empty for a guest, who is told when and nothing more.
	Title string
	// Clock is "7 pm", empty when the night has no start time.
	Clock string
	// Left and Width place the block on the bar, in percent.
	Left, Width float64
	// Answer is the viewer's own answer ("You're going", "Maybe", "Can't go");
	// Tone colours it. Owes asks for an answer instead.
	Answer, Tone string
	Owes         bool
	Href         string
}

// couldPlayRow is one day.
type couldPlayRow struct {
	Date  string
	Label string
	// Nights are the day's set nights; Phrase is the free-time words.
	Nights []couldPlayNight
	Phrase string
	// Shade is a CSS gradient for the bar, empty when nothing is shaded.
	Shade string
	// Extra rows sit behind "Show N more days".
	Extra bool
}

// couldPlayView is what the template draws.
type couldPlayView struct {
	CampaignID     string
	Guest          bool
	Rows           []couldPlayRow
	ExtraN         int
	Usually        string
	GameNightsHref string
}

// couldPlayNightIn is a night already placed in the viewer's day.
type couldPlayNightIn struct {
	Date    string
	Weekday time.Weekday
	// Hour is the start as hours since midnight; -1 when no time is set.
	Hour float64
	couldPlayNight
}

// clockWord words a time as "7 pm" or "7:30 pm".
func clockWord(t time.Time) string {
	if t.Minute() == 0 {
		return strings.ToLower(t.Format("3 pm"))
	}
	return strings.ToLower(t.Format("3:04 pm"))
}

// couldPlayAnswer words the viewer's own answer to a night. owes is true when
// they have not answered or the night moved since.
func couldPlayAnswer(mine *NightAnswer) (label, tone string, owes bool) {
	switch {
	case mine == nil:
		return "", "", false
	case mine.Answer == NightNoAnswer || mine.Recheck:
		return "", "", true
	case mine.Answer == NightYes:
		return "You're going", "go", false
	case mine.Answer == NightMaybe:
		return "Maybe", "maybe", false
	}
	return "Can't go", "cant", false
}

// couldPlayNightsFrom places each upcoming night in a day of loc. A guest's
// nights keep their own zone and lose title and answer. Nights that have
// ended, or fall outside [today, last], are dropped.
func couldPlayNightsFrom(nights []GameNight, base, userID string, loc *time.Location, guest bool, now time.Time, today, last string) []couldPlayNightIn {
	var out []couldPlayNightIn
	for _, n := range nights {
		if n.Past {
			continue
		}
		nl := loc
		if guest {
			nl = timeutil.LoadLocation(n.TZ)
		}
		in := couldPlayNightIn{Date: n.Date, Hour: -1}
		if n.Time != "" {
			inst, ok := nightInstant(n.Date, n.Time, n.TZ)
			if !ok {
				continue
			}
			if inst.Add(couldPlayNightHours * time.Hour).Before(now) {
				continue
			}
			local := inst.In(nl)
			in.Date = local.Format("2006-01-02")
			in.Hour = float64(local.Hour()) + float64(local.Minute())/60
			in.Clock = clockWord(local)
		}
		if in.Date < today || in.Date > last {
			continue
		}
		d, err := time.Parse("2006-01-02", in.Date)
		if err != nil {
			continue
		}
		in.Weekday = d.Weekday()
		in.Left, in.Width = nightBar(in.Hour)
		if !guest {
			in.Title = n.Name
			n.ForViewer(userID, false)
			in.Answer, in.Tone, in.Owes = couldPlayAnswer(n.Mine)
			in.Href = fmt.Sprintf("%s/game-nights?session=%s&date=%s", base, url.QueryEscape(n.SessionID), url.QueryEscape(n.Date))
		}
		out = append(out, in)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Hour < out[j].Hour
	})
	return out
}

// nightBar places a night on the noon-to-midnight bar. A night with no time
// spans the whole bar; one that starts before noon is clipped to it.
func nightBar(hour float64) (left, width float64) {
	if hour < 0 {
		return 0, 100
	}
	start := hour
	if start < couldPlayAxisStart {
		start = couldPlayAxisStart
	}
	end := hour + couldPlayNightHours
	if end > couldPlayAxisStart+couldPlayAxisHours {
		end = couldPlayAxisStart + couldPlayAxisHours
	}
	left = (start - couldPlayAxisStart) * 100 / couldPlayAxisHours
	width = (end - start) * 100 / couldPlayAxisHours
	if width < 6 {
		width = 6
	}
	if left+width > 100 {
		left = 100 - width
	}
	return left, width
}

// daySteps is a day's twelve steps, noon to midnight, from the anonymous
// counts. Stretches shorter than couldPlayMinRun are dropped: one free hour is
// not a game night.
func daySteps(d OverlayDay, total int) [couldPlayAxisHours]int {
	var steps [couldPlayAxisHours]int
	for i := range steps {
		h := couldPlayAxisStart + i
		if h < len(d.Hours) {
			steps[i] = availabilityStep(d.Hours[h].Free, total)
		}
	}
	for i := 0; i < len(steps); {
		if steps[i] == stepNone {
			i++
			continue
		}
		j := i
		for j < len(steps) && steps[j] != stepNone {
			j++
		}
		if j-i < couldPlayMinRun {
			for k := i; k < j; k++ {
				steps[k] = stepNone
			}
		}
		i = j
	}
	return steps
}

// shadeGradient draws the steps as a left-to-right grey gradient; the accent
// is kept for set nights, so a likely evening never reads as a booked one.
// Each hour's strength is blended with its neighbours' so the bands melt into
// one another instead of ending at a hard edge; the stops sit at hour centres
// so the transition is spread across the hours either side of a change.
func shadeGradient(steps [couldPlayAxisHours]int) string {
	var parts []string
	n := len(steps)
	at := func(i int) float64 {
		if i < 0 {
			i = 0
		}
		if i >= n {
			i = n - 1
		}
		return float64(steps[i]) / stepMost
	}
	for i := 0; i < n; i++ {
		v := 0.25*at(i-1) + 0.5*at(i) + 0.25*at(i+1)
		alpha := int(v*couldPlayMaxAlpha + 0.5)
		pos := float64(2*i+1) * 100 / float64(2*n)
		parts = append(parts, fmt.Sprintf("color-mix(in srgb, var(--color-text-muted) %d%%, transparent) %.1f%%", alpha, pos))
	}
	return "linear-gradient(to right, " + strings.Join(parts, ", ") + ")"
}

// shaded reports whether any hour of the day has a step.
func shaded(steps [couldPlayAxisHours]int) bool {
	for _, s := range steps {
		if s != stepNone {
			return true
		}
	}
	return false
}

// clock12 words an hour of the day on a 12-hour dial without am or pm.
func clock12(h int) string {
	h %= 12
	if h == 0 {
		h = 12
	}
	return fmt.Sprint(h)
}

// partOfDay names the stretch of the day an hour (possibly fractional) is in.
func partOfDay(h float64) string {
	switch {
	case h < 5:
		return "late night"
	case h < 12:
		return "morning"
	case h < 17:
		return "afternoon"
	case h < 22:
		return "evening"
	}
	return "late evening"
}

// stepPhrase words a day's steps: the longest most-of-the-table stretch as
// "Evening, about 7 to 10", else the longest half stretch as "Evening might
// work". It reads only the steps, never a count.
func stepPhrase(steps [couldPlayAxisHours]int) string {
	bestFrom, bestTo, bestLen := 0, 0, 0
	for _, minStep := range []int{stepMost, stepHalf} {
		for i := 0; i < len(steps); {
			if steps[i] < minStep {
				i++
				continue
			}
			j := i
			for j < len(steps) && steps[j] >= minStep {
				j++
			}
			if j-i >= couldPlayMinRun && j-i > bestLen {
				bestFrom, bestTo, bestLen = i, j, j-i
			}
			i = j
		}
		if bestLen == 0 {
			continue
		}
		from, to := couldPlayAxisStart+bestFrom, couldPlayAxisStart+bestTo
		part := partOfDay(float64(from+to) / 2)
		part = strings.ToUpper(part[:1]) + part[1:]
		if minStep == stepMost {
			return fmt.Sprintf("%s, about %s to %s", part, clock12(from), clock12(to))
		}
		return part + " might work"
	}
	return ""
}

// usuallyLine words the table's habit from its set nights: "Usually plays
// Thursday evenings". Empty with fewer than two nights, since one is no habit.
func usuallyLine(nights []couldPlayNightIn) string {
	if len(nights) < 2 {
		return ""
	}
	days := map[time.Weekday]int{}
	for _, n := range nights {
		days[n.Weekday]++
	}
	best, bestN := time.Sunday, 0
	for d := time.Sunday; d <= time.Saturday; d++ {
		if days[d] > bestN {
			best, bestN = d, days[d]
		}
	}
	parts := map[string]int{}
	for _, n := range nights {
		if n.Weekday == best && n.Hour >= 0 {
			parts[partOfDay(n.Hour)]++
		}
	}
	order := []string{"morning", "afternoon", "evening", "late evening", "late night"}
	part, partN := "", 0
	for _, p := range order {
		if parts[p] > partN {
			part, partN = p, parts[p]
		}
	}
	line := "Usually plays " + best.String()
	if part == "" {
		return line + "s"
	}
	if part == "late evening" {
		part = "evening"
	}
	return line + " " + part + "s"
}

// couldPlayInput is everything the pure builder needs.
type couldPlayInput struct {
	CampaignID string
	Guest      bool
	Today      string
	Last       string
	Nights     []couldPlayNightIn
	Weeks      []WeekOverlay
	PollDates  map[string]bool
}

// buildCouldPlay assembles the rows. A day earns a row with a set night, a
// poll date, or shading of at least the middle step. A guest gets set nights
// and the habit line only.
func buildCouldPlay(in couldPlayInput) couldPlayView {
	view := couldPlayView{
		CampaignID:     in.CampaignID,
		Guest:          in.Guest,
		GameNightsHref: "/campaigns/" + url.PathEscape(in.CampaignID) + "/game-nights",
	}
	rows := map[string]*couldPlayRow{}
	row := func(date string) *couldPlayRow {
		r := rows[date]
		if r == nil {
			r = &couldPlayRow{Date: date, Label: dayLabel(date, in.Today)}
			rows[date] = r
		}
		return r
	}
	for _, n := range in.Nights {
		r := row(n.Date)
		r.Nights = append(r.Nights, n.couldPlayNight)
	}
	if in.Guest {
		view.Usually = usuallyLine(in.Nights)
	} else {
		for date := range in.PollDates {
			if date >= in.Today && date <= in.Last {
				row(date)
			}
		}
		for _, w := range in.Weeks {
			if w.TotalMembers <= couldPlaySmallTable {
				continue
			}
			for _, d := range w.Days {
				if d.Date < in.Today {
					continue
				}
				steps := daySteps(d, w.TotalMembers)
				if !shaded(steps) {
					continue
				}
				r := row(d.Date)
				r.Shade = shadeGradient(steps)
				r.Phrase = stepPhrase(steps)
			}
		}
		for date, r := range rows {
			if in.PollDates[date] {
				if r.Phrase == "" {
					r.Phrase = "In a poll"
				} else {
					r.Phrase += " · in a poll"
				}
			}
		}
	}
	dates := make([]string, 0, len(rows))
	for d := range rows {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for i, d := range dates {
		r := *rows[d]
		r.Extra = i >= couldPlayCollapsed
		view.Rows = append(view.Rows, r)
	}
	if n := len(view.Rows) - couldPlayCollapsed; n > 0 {
		view.ExtraN = n
	}
	return view
}

// dayLabel is "Today", "Tomorrow" or "Thu 19 Nov".
func dayLabel(date, today string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	if t, err := time.Parse("2006-01-02", today); err == nil {
		switch int(d.Sub(t).Hours() / 24) {
		case 0:
			return "Today"
		case 1:
			return "Tomorrow"
		}
	}
	return d.Format("Mon 2 Jan")
}

// MoreWord words the expander: "Show 4 more days".
func (v couldPlayView) MoreWord() string {
	if v.ExtraN == 1 {
		return "Show 1 more day"
	}
	return fmt.Sprintf("Show %d more days", v.ExtraN)
}

// WhenWeCouldPlayBlock draws the block, or an empty answer that removes the
// placeholder when a source cannot be read. Members see their own answers and
// the table's usual free times; a guest of a public campaign (only when the
// owner placed the block on the Guests home) sees set nights as date and time.
// GET /campaigns/:id/when-we-could-play
func (h *Handler) WhenWeCouldPlayBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	guest := !cc.IsMember
	// A visitor sees game nights only where the owner chose to show them: the
	// route is reachable on any public campaign, so placement is the consent.
	userID := auth.GetUserID(c)
	if !campaigns.BlockVisibleTo(ctx, cc, h.placement, campaigns.BlockWhenWeCouldPlay, c.QueryParam("on"), userID) {
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
	last := todayDate.AddDays(couldPlayDays - 1).String()
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
	// The window opens a day early and closes a day late so a night in a far
	// zone still lands on its own day once converted.
	from := todayDate.AddDays(-1).String()
	to := todayDate.AddDays(couldPlayDays).String()
	nights, err := h.svc.ListGameNights(ctx, cc.Campaign.ID, from, to, today, members)
	if err != nil {
		return c.HTML(http.StatusOK, "")
	}
	in := couldPlayInput{
		CampaignID: cc.Campaign.ID,
		Guest:      guest,
		Today:      today,
		Last:       last,
		Nights:     couldPlayNightsFrom(nights, base, userID, loc, guest, now, today, last),
	}
	if !guest {
		// Never the detail form: the block is for every member.
		for offset := 0; offset < couldPlayDays; offset += 7 {
			o, err := h.CampaignWeekOverlay(ctx, cc.Campaign.ID, todayDate.AddDays(offset).String(), tz, false)
			if err != nil {
				break
			}
			in.Weeks = append(in.Weeks, *o)
		}
		if polls, err := h.svc.ListOpenProposalViews(ctx, cc.Campaign.ID, userID, tz); err == nil {
			in.PollDates = map[string]bool{}
			for _, p := range polls {
				for _, o := range p.Options {
					in.PollDates[o.Local.Date] = true
				}
			}
		}
	}
	return middleware.Render(c, http.StatusOK, whenWeCouldPlayBlock(buildCouldPlay(in)))
}

// homeHasBlock reports whether a home layout places blockType in a column or
// a tile section.
func homeHasBlock(l *campaigns.DashboardLayout, blockType string) bool {
	return campaigns.HomeHasBlock(l, blockType)
}
