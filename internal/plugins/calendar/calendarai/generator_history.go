package calendarai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// The history generator writes the calendar's past: eras and one-off events
// up to the current year. It writes many events, some hidden, and maybe
// eras, so only the owner or a co-DM may run it, the same people who may
// write eras and hidden events by hand.

const (
	maxGenHistoryYears = 5000 // the generator's own maximum
	maxGenHistoryEras  = 8    // the generator's own maximum
	// historyStoryBudget bounds years x events-per-century / 100, so an
	// honest run stays under maxGenEvents (measured: at most ~255 events).
	historyStoryBudget = 250
	// historyAutoSpan is the longest history the generator picks itself
	// when the row leaves `years` at 0.
	historyAutoSpan = 1100
)

// EraAPI writes a generated history's eras. The calendar service is one.
type EraAPI interface {
	CreateEra(ctx context.Context, calendarID, campaignID string, input calendar.EraInput) (*calendar.Era, error)
}

// historyKinds are the generator's switches, each off/rare/normal/often.
var historyKinds = []string{"foundings", "wars", "plagues", "famines", "discoveries", "coronations", "disasters", "portents", "migrations", "schisms"}

var historyFreqs = map[string]bool{"off": true, "rare": true, "normal": true, "often": true}

var historyRecipes = map[string]string{"kingdom": "history.kingdom", "frontier": "history.frontier", "dark": "history.dark"}

var historyTones = map[string]string{"bright": "bright", "hopeful": "bright", "balanced": "balanced", "grim": "grim"}

var historyThemes = map[string]bool{"pastoral": true, "nautical": true, "dwarven": true, "elven": true, "desert": true, "imperial": true, "fey": true, "grim": true}

// hexColor is the era colour the calendar service accepts.
var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// histEra is an era as the browser run gets it (to keep) and returns it.
type histEra struct {
	Name        string  `json:"name"`
	StartYear   int     `json:"start_year"`
	EndYear     *int    `json:"end_year"`
	Description *string `json:"description,omitempty"`
	Color       string  `json:"color,omitempty"`
}

// histEvent is the subset of a generated history event that is saved:
// history is one-off, all-day, past events, so nothing else is read.
type histEvent struct {
	Name        string  `json:"name"`
	Year        int     `json:"year"`
	Month       int     `json:"month"`
	Day         int     `json:"day"`
	Description *string `json:"description"`
	Visibility  string  `json:"visibility"`
}

// historyPlan is the generator plan plus what a history run needs: the
// built-in its settings lie over, whether it may add eras, and the
// calendar's eras that generated ones must give way to.
type historyPlan struct {
	genPlan
	BasedOn   string    `json:"basedOn"`
	MakesEras bool      `json:"makesEras"`
	KeepEras  []histEra `json:"keepEras,omitempty"`
}

type historyRow struct {
	plan     historyPlan
	cal      *calendar.Calendar
	minYear  int
	summary  string
	warnings []string
}

// planHistory checks a history row against the calendar as it is now. It
// runs again at Apply, so the era rule follows any eras an earlier row of
// the same paste added.
func (k GeneratorKind) planHistory(ctx context.Context, campaignID string, a Actor, r Record) (historyRow, error) {
	var row historyRow
	if r.Action != ActionCreate {
		return row, apperror.NewBadRequest("generators only add things; use action: create")
	}
	if !a.CanAuthorDmOnly() {
		return row, apperror.NewBadRequest("only the owner or a co-DM can write the calendar's history")
	}
	p := historyPlan{genPlan: genPlan{Generator: "history", Seed: r.Str("seed")}}
	if p.Seed == "" {
		p.Seed = newSeed()
	}
	recipe := strings.TrimPrefix(strings.ToLower(r.Str("recipe")), "history.")
	if recipe == "" {
		recipe = "kingdom"
	}
	if p.BasedOn = historyRecipes[recipe]; p.BasedOn == "" {
		return row, apperror.NewBadRequest("recipe must be kingdom, frontier or dark")
	}
	details := map[string]any{}
	years, err := intIn(r, "years", 0, maxGenHistoryYears)
	if err != nil {
		return row, err
	}
	if years > 0 {
		details["years"] = years
	}
	eras, err := intIn(r, "eras", 0, maxGenHistoryEras)
	if err != nil {
		return row, err
	}
	if eras > 0 {
		details["eras"] = eras
	}
	density, ok, err := r.Float("density")
	if err != nil {
		return row, err
	}
	if ok {
		if density < 0.5 || density > 20 {
			return row, apperror.NewBadRequest("density (events per century) must be from 0.5 to 20")
		}
		details["density"] = density
	} else {
		density = 4 // the generator's default
	}
	if s := strings.ToLower(r.Str("tone")); s != "" {
		if historyTones[s] == "" {
			return row, apperror.NewBadRequest("tone must be bright, balanced or grim")
		}
		details["tone"] = historyTones[s]
	}
	if s := strings.ToLower(r.Str("theme")); s != "" {
		if !historyThemes[s] {
			return row, apperror.NewBadRequest("theme must be pastoral, nautical, dwarven, elven, desert, imperial, fey or grim")
		}
		details["theme"] = s
	}
	if s := strings.TrimSpace(r.Str("realm")); s != "" {
		if utf8.RuneCountInString(s) > 60 {
			return row, apperror.NewBadRequest("realm: the realm's name can be at most 60 characters")
		}
		details["realm"] = s
	}
	secrets := true
	if v, ok := r.Bool("secrets"); ok {
		secrets = v
		details["secrets"] = v
	}
	makes := map[string]any{}
	for _, kind := range historyKinds {
		if !r.Has(kind) {
			continue
		}
		v := strings.ToLower(r.Str(kind))
		if !historyFreqs[v] {
			return row, badRequestf("%s must be off, rare, normal or often", kind)
		}
		makes[kind] = v
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return row, err
	}
	now := cal.CurrentYear
	if now < 2 {
		return row, badRequestf("%s's current year is %d, so it has no past to fill; move its current date forward first", cal.Name, now)
	}
	span := historyAutoSpan
	if years > 0 {
		span = years
	}
	span = min(span, now-1)
	if stories := int(math.Ceil(density * float64(span) / 100)); stories > historyStoryBudget {
		return row, badRequestf("that history would have about %d stories; at most %d fit in one run, so lower `years` or `density`", stories, historyStoryBudget)
	}
	row.minYear = 1
	if years > 0 {
		row.minYear = max(1, now-years)
	}
	cj, err := json.Marshal(cal)
	if err != nil {
		return row, err
	}
	p.Calendar = cj
	p.Recipe = map[string]any{"generator": "history", "makes": makes, "details": details}
	// The owner's rule: a calendar with more than one era keeps its eras
	// and the run adds only events.
	p.MakesEras = len(cal.Eras) <= 1 && k.Eras != nil
	for _, e := range cal.Eras {
		p.KeepEras = append(p.KeepEras, histEra{Name: e.Name, StartYear: e.StartYear, EndYear: e.EndYear})
	}
	row.plan, row.cal = p, cal

	back := fmt.Sprintf("the %d years before %d", span, now)
	if years == 0 {
		back = fmt.Sprintf("as far back as the generator chooses (at most %d years before %d)", span, now)
	}
	what := "eras and events"
	switch {
	case len(cal.Eras) > 1:
		what = "events only (the calendar keeps its own eras)"
	case !p.MakesEras:
		what = "events only (eras can't be written on this server)"
	case len(cal.Eras) == 1:
		row.warnings = append(row.warnings, "New eras that overlap "+quote(cal.Eras[0].Name)+" give way to it")
	}
	hidden := "with hidden history only the owner and co-DMs see"
	if !secrets {
		hidden = "with no hidden history"
	}
	row.summary = fmt.Sprintf("a %s history on %s: %s for %s, %s", recipe, cal.Name, what, back, hidden)
	return row, nil
}

func (k GeneratorKind) planHistoryRow(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	row, err := k.planHistory(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	cj, err := json.Marshal(row.plan)
	if err != nil {
		return Plan{Error: "could not prepare the generator"}
	}
	return Plan{Summary: row.summary + ". Runs when you press Import.", Client: string(cj), Warnings: row.warnings}
}

// applyHistory checks the browser's output against the plan and writes it.
// Every era is checked before anything is written, so a bad era writes
// nothing; events then go in one by one, as the events generator's do.
func (k GeneratorKind) applyHistory(ctx context.Context, campaignID string, a Actor, r Record) error {
	row, err := k.planHistory(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(r.Generated) == "" {
		return apperror.NewBadRequest("the generator did not run in your browser; try Import again")
	}
	var out struct {
		Eras   []histEra   `json:"eras"`
		Events []histEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(r.Generated), &out); err != nil {
		return apperror.NewBadRequest("the generated history could not be read")
	}
	if len(out.Events) > maxGenEvents {
		return badRequestf("the generator returned %d events", len(out.Events))
	}
	if len(out.Eras) > maxGenHistoryEras {
		return badRequestf("the generator returned %d eras", len(out.Eras))
	}
	cal := row.cal
	var eras []calendar.EraInput
	// Eras a run may not make are left out, not refused: the run was made
	// against the calendar at review, and an earlier row may since have
	// given it its eras.
	if row.plan.MakesEras {
		if eras, err = historyEras(cal, out.Eras); err != nil {
			return err
		}
	}
	for _, in := range eras {
		if _, err := k.Eras.CreateEra(ctx, cal.ID, campaignID, in); err != nil {
			return err
		}
	}

	existing, err := k.Cal.ListEventsForCalendar(ctx, campaignID, cal.ID, a.Role)
	if err != nil {
		return apperror.NewBadRequest("could not read the calendar's events")
	}
	have := make(map[string]bool, len(existing))
	for _, ex := range existing {
		have[eventKey(ex.Name, ex.Year, ex.Month, ex.Day)] = true
	}
	onDay := calendar.AnnouncedOnDay
	var failed, added, skipped int
	for _, e := range out.Events {
		e.Name = strings.TrimSpace(e.Name)
		if e.Name == "" || e.Year < row.minYear || e.Year > cal.CurrentYear ||
			e.Month < 1 || e.Month > len(cal.Months) || e.Day < 1 || e.Day > cal.MonthDays(e.Month-1, e.Year) {
			failed++
			continue
		}
		if have[eventKey(e.Name, e.Year, e.Month, e.Day)] {
			skipped++
			continue
		}
		// Hidden history stays Director-only; anything else is for everyone.
		vis := "everyone"
		if e.Visibility == "dm_only" {
			vis = "dm_only"
		}
		var desc *string
		if e.Description != nil {
			if desc, err = bodyHTML(r, *e.Description); err != nil {
				failed++
				continue
			}
		}
		if _, err := k.Cal.CreateEvent(ctx, cal.ID, campaignID, calendar.CreateEventInput{
			Name: e.Name, Year: e.Year, Month: e.Month, Day: e.Day, DescriptionHTML: desc,
			Visibility: vis, Announced: &onDay, AllDay: true,
			CreatedBy: a.UserID, CanAuthorDmOnly: a.CanAuthorDmOnly(), Author: a.Viewer(),
		}); err != nil {
			failed++
			continue
		}
		have[eventKey(e.Name, e.Year, e.Month, e.Day)] = true
		added++
	}
	if failed > 0 {
		return badRequestf("%d of %d generated history events could not be added (%d were added, %d were already there). Running it again only adds the missing ones", failed, len(out.Events), added, skipped)
	}
	return nil
}

// historyEras checks generated eras: inside the calendar's past, not over
// one another or over an era the calendar has. An era already there by
// name is skipped, so a rerun adds nothing twice. An open end runs on
// forever, as the calendar reads it.
func historyEras(cal *calendar.Calendar, gen []histEra) ([]calendar.EraInput, error) {
	type span struct {
		name     string
		from, to int
	}
	open := func(end *int) int {
		if end == nil {
			return math.MaxInt
		}
		return *end
	}
	taken := make([]span, 0, len(cal.Eras)+len(gen))
	for _, e := range cal.Eras {
		taken = append(taken, span{e.Name, e.StartYear, open(e.EndYear)})
	}
	var out []calendar.EraInput
	for _, g := range gen {
		g.Name = strings.TrimSpace(g.Name)
		if g.Name == "" {
			return nil, apperror.NewBadRequest("the generator returned an era with no name; nothing was written")
		}
		if findEra(cal, g.Name) != nil {
			continue
		}
		if g.StartYear < 1 || g.StartYear > cal.CurrentYear || (g.EndYear != nil && (*g.EndYear < g.StartYear || *g.EndYear > cal.CurrentYear)) {
			return nil, badRequestf("the generated era %s is outside the calendar's past; nothing was written", quote(g.Name))
		}
		s := span{g.Name, g.StartYear, open(g.EndYear)}
		for _, t := range taken {
			if s.from <= t.to && t.from <= s.to {
				return nil, badRequestf("the generated era %s overlaps the era %s; nothing was written", quote(g.Name), quote(t.name))
			}
		}
		taken = append(taken, s)
		in := calendar.EraInput{Name: g.Name, StartYear: g.StartYear, StartMonth: 1, StartDay: 1, EndYear: g.EndYear, Description: g.Description, SortOrder: len(cal.Eras) + len(out)}
		if hexColor.MatchString(g.Color) {
			in.Color = g.Color
		}
		out = append(out, in)
	}
	return out, nil
}

// newSeed fixes a run's seed at review, so Import runs the same history
// however many times the page is submitted.
func newSeed() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// intIn reads an optional whole-number key within lo..hi; absent is 0.
func intIn(r Record, key string, lo, hi int) (int, error) {
	n, ok, err := r.Int(key)
	if err != nil {
		return 0, err
	}
	if ok && (n < lo || n > hi) {
		return 0, badRequestf("%s must be from %d to %d", key, lo, hi)
	}
	return n, nil
}
