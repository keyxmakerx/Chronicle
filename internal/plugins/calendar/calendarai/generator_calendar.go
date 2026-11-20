package calendarai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// The whole-calendar and moons generators. Both only add: a generated
// calendar is always a new calendar, because rewriting the months under an
// existing calendar's events and dates is what the typed calendar block
// does with its own preview and warnings; and generated moons join the main
// calendar beside the moons it has, which go to the engine as kept, so none
// is ever replaced. Owner only, as the wizard and the typed calendar block
// are.

// The engine's own ids and bounds for these generators. A JS test reads
// these lists from this file and checks them against the engine.
var (
	genCalTemplates = []string{"gregorian-like", "harptos-like", "even", "lunar", "wild"}
	genThemes       = []string{"pastoral", "nautical", "dwarven", "elven", "desert", "imperial", "fey", "grim"}
	genFreqs        = []string{"off", "rare", "normal", "often"}
	genMoonCycles   = []string{"natural", "varied", "fast", "slow"}
	// genCalStyles maps a row's `style` to the engine's built-in recipe.
	genCalStyles = map[string]string{
		"familiar":      "calendar.gregorian-like",
		"tendays":       "calendar.harptos-like",
		"deep-count":    "calendar.dwarven",
		"moon-reckoned": "calendar.lunar",
		"strange":       "calendar.wild",
	}
)

const (
	maxGenCalMoons     = 5  // the calendar generator's own maximum
	maxGenCalSeasons   = 6  // likewise
	maxGenSkyMoons     = 6  // the moons generator's most moons in one sky, kept ones included
	maxGenCalNameRunes = 80 // the calendar generator's name field
	// maxGenCalendarBytes is what the wizard's Generate step accepts; a real
	// generated calendar is a few KB.
	maxGenCalendarBytes = 256 * 1024
)

// buildPlan is what the review screen runs for these two generators. Base
// is a built-in recipe the row's own settings are laid over; Locked is the
// moons the engine must keep and stay clear of.
type buildPlan struct {
	Generator string           `json:"generator"`
	Base      string           `json:"base,omitempty"`
	Recipe    map[string]any   `json:"recipe"`
	Seed      string           `json:"seed"`
	Locked    []map[string]any `json:"locked,omitempty"`
}

func genSeed(r Record) string {
	if s := r.Str("seed"); s != "" {
		return s
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func oneOf(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// pickKey reads an optional choice key, lower-cased, refusing anything off
// the list.
func pickKey(r Record, key string, list []string) (string, error) {
	v := strings.ToLower(r.Str(key))
	if v == "" || oneOf(list, v) {
		return v, nil
	}
	return "", badRequestf("%s: %q is not one of %s", key, v, strings.Join(list, ", "))
}

// countKey reads an optional whole number in [lo, hi].
func countKey(r Record, key string, lo, hi int) (int, bool, error) {
	n, ok, err := r.Int(key)
	if err != nil {
		return 0, true, err
	}
	if ok && (n < lo || n > hi) {
		return 0, true, badRequestf("%s must be %d to %d", key, lo, hi)
	}
	return n, ok, nil
}

func ownerOnlyGen(a Actor, what string) error {
	if a.Role < permissions.RoleOwner {
		return badRequestf("only the campaign owner can %s", what)
	}
	return nil
}

func (k GeneratorKind) clientPlan(p buildPlan, summary string, warnings []string) Plan {
	cj, err := json.Marshal(p)
	if err != nil {
		return Plan{Error: "could not prepare the generator"}
	}
	return Plan{Summary: summary + ". Runs when you press Import.", Client: string(cj), Warnings: warnings}
}

// ---- calendar ----

// calRow is a checked calendar row: the plan, and the bounds its output must
// keep.
type calRow struct {
	plan     buildPlan
	name     string // the calendar's name when the row sets one
	moons    int
	seasons  int
	main     *calendar.Calendar // nil when the new calendar becomes the main one
	summary  string
	warnings []string
}

func (k GeneratorKind) planCalendarRow(ctx context.Context, campaignID string, a Actor, r Record) (calRow, error) {
	row := calRow{moons: maxGenCalMoons, seasons: maxGenCalSeasons}
	if err := ownerOnlyGen(a, "make a calendar"); err != nil {
		return row, err
	}
	if k.Structure == nil {
		return row, apperror.NewBadRequest("generated calendars aren't available on this server")
	}
	if r.Action != ActionCreate {
		return row, apperror.NewBadRequest("the calendar generator only makes a new calendar; use action: create (to change a calendar, use a kind: calendar block)")
	}
	makes, details := map[string]any{}, map[string]any{}
	p := buildPlan{Generator: "calendar", Seed: genSeed(r)}
	style := strings.ToLower(r.Str("style"))
	if style != "" {
		base, ok := genCalStyles[style]
		if !ok {
			return row, badRequestf("style: %q is not one of familiar, tendays, deep-count, moon-reckoned, strange", style)
		}
		p.Base = base
	}
	for _, c := range []struct {
		key, field string
		list       []string
	}{{"template", "template", genCalTemplates}, {"theme", "theme", genThemes}} {
		v, err := pickKey(r, c.key, c.list)
		if err != nil {
			return row, err
		}
		if v != "" {
			details[c.field] = v
		}
	}
	if v, ok, err := r.Float("randomness"); err != nil {
		return row, err
	} else if ok {
		if v < 0 || v > 1 {
			return row, apperror.NewBadRequest("randomness must be from 0 to 1")
		}
		details["randomness"] = v
	}
	for _, c := range []struct {
		key, field string
		lo, hi     int
	}{{"months", "months", 1, 24}, {"week_length", "weekLength", 1, 14}, {"current_year", "currentYear", 1, 100000}} {
		n, ok, err := countKey(r, c.key, c.lo, c.hi)
		if err != nil {
			return row, err
		}
		if ok {
			details[c.field] = n
		}
	}
	if n := r.Str("calendar_name"); n != "" {
		if utf8.RuneCountInString(n) > maxGenCalNameRunes {
			return row, badRequestf("calendar_name is at most %d characters", maxGenCalNameRunes)
		}
		details["name"], row.name = n, n
	}
	if n, ok, err := countKey(r, "moons", 0, maxGenCalMoons); err != nil {
		return row, err
	} else if ok {
		makes["moons"], row.moons = n, n
	}
	if n, ok, err := countKey(r, "seasons", 0, maxGenCalSeasons); err != nil {
		return row, err
	} else if ok {
		makes["seasons"], row.seasons = n, n
	}
	if v, err := pickKey(r, "festival_days", genFreqs); err != nil {
		return row, err
	} else if v != "" {
		makes["festival-days"] = v
	}
	for _, c := range []struct{ key, field string }{{"leap_years", "leap"}, {"festivals", "festivals"}} {
		if v, ok := r.Bool(c.key); ok {
			makes[c.field] = v
		}
	}
	p.Recipe = map[string]any{"generator": "calendar", "makes": makes, "details": details}
	row.plan = p

	main, err := k.mainCalendar(ctx, campaignID, a)
	if err != nil {
		return row, err
	}
	row.main = main
	if row.name != "" {
		if err := k.calendarNameFree(ctx, campaignID, a, row.name, main); err != nil {
			return row, err
		}
	}
	what := "a new calendar"
	if row.name != "" {
		what = "a new calendar called " + quote(row.name)
	}
	if main == nil {
		row.summary = what + "; it becomes the main calendar"
		row.warnings = append(row.warnings, "The calendar is made when you press Import, so blocks below that need a calendar can't be checked against it yet; add those in a later import")
	} else {
		row.summary = fmt.Sprintf("%s beside %s, which stays the main calendar", what, quote(main.Name))
		row.warnings = append(row.warnings, fmt.Sprintf("Events, weather, moons and the other generators keep using %s; make the new calendar the main one with a kind: calendar block (main: true) or on the Calendars page", quote(main.Name)))
	}
	return row, nil
}

// mainCalendar is the campaign's main calendar, a pending one from a
// calendar block earlier in the paste, or nil when there is none.
func (k GeneratorKind) mainCalendar(ctx context.Context, campaignID string, a Actor) (*calendar.Calendar, error) {
	if p := pendingCalendar(ctx); p != nil {
		return p, nil
	}
	cal, err := k.Structure.GetDefaultCalendarForViewer(ctx, campaignID, a.Viewer())
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return nil, nil
		}
		return nil, apperror.NewBadRequest("could not read the campaign's calendar; try again in a moment")
	}
	return cal, nil
}

// calendarNameFree refuses a name another of the campaign's calendars has,
// so a second Import of the same page can't make the calendar twice.
func (k GeneratorKind) calendarNameFree(ctx context.Context, campaignID string, a Actor, name string, main *calendar.Calendar) error {
	if main != nil && sameName(main.Name, name) {
		return badRequestf("a calendar called %s already exists; nothing was made", quote(name))
	}
	cals, err := k.Structure.ListCalendars(ctx, campaignID, a.Viewer())
	if err != nil {
		return apperror.NewBadRequest("could not read the campaign's calendars; try again in a moment")
	}
	for _, c := range cals {
		if sameName(c.Name, name) {
			return badRequestf("a calendar called %s already exists; nothing was made", quote(name))
		}
	}
	return nil
}

func (k GeneratorKind) planCalendar(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	row, err := k.planCalendarRow(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	return k.clientPlan(row.plan, row.summary, row.warnings)
}

// applyCalendar reads the generated calendar the way the wizard's Generate
// step does (the uploaded-file parser, with its clamps and limits), holds it
// to what the generator makes, and creates it.
func (k GeneratorKind) applyCalendar(ctx context.Context, campaignID string, a Actor, r Record) error {
	row, err := k.planCalendarRow(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if len(r.Generated) > maxGenCalendarBytes {
		return badRequestf("the generated calendar is too large (at most %d KB)", maxGenCalendarBytes/1024)
	}
	raw := strings.TrimSpace(r.Generated)
	if raw == "" {
		return apperror.NewBadRequest("the generator did not run in your browser; try Import again")
	}
	var head struct {
		Format string `json:"format"`
	}
	if json.Unmarshal([]byte(raw), &head) != nil || head.Format != "chronicle-calendar-v1" {
		return apperror.NewBadRequest("the generated calendar could not be read")
	}
	ir, err := calendar.DetectAndParse([]byte(raw))
	if err != nil {
		return badRequestf("the generated calendar could not be used: %s", err.Error())
	}
	switch {
	case len(ir.Events) > 0:
		return apperror.NewBadRequest("the generated calendar carried events, which the generator never makes")
	case ir.Settings.TracksRealTime:
		return apperror.NewBadRequest("the generated calendar follows the real-world date, which the generator never makes")
	case len(ir.Moons) > row.moons:
		return badRequestf("the generator returned %d moons", len(ir.Moons))
	case len(ir.Seasons) > row.seasons:
		return badRequestf("the generator returned %d seasons", len(ir.Seasons))
	case row.name != "" && ir.CalendarName != row.name:
		return apperror.NewBadRequest("the generated calendar's name is not the one asked for")
	case strings.TrimSpace(ir.CalendarName) == "":
		return apperror.NewBadRequest("the generated calendar has no name")
	}
	if err := k.calendarNameFree(ctx, campaignID, a, ir.CalendarName, row.main); err != nil {
		return err
	}
	// The service makes a calendar the main one only when the campaign has
	// none, as the wizard relies on.
	_, err = k.Structure.CreateCalendarFromImport(ctx, campaignID, ir, calendar.CreateCalendarFromImportOptions{})
	return err
}

// ---- moons ----

type moonsRow struct {
	plan    buildPlan
	cal     *calendar.Calendar
	count   int
	summary string
}

func (k GeneratorKind) planMoonsRow(ctx context.Context, campaignID string, a Actor, r Record) (moonsRow, error) {
	var row moonsRow
	if err := ownerOnlyGen(a, "add moons to the calendar"); err != nil {
		return row, err
	}
	if k.Structure == nil {
		return row, apperror.NewBadRequest("generated moons aren't available on this server")
	}
	if r.Action != ActionCreate {
		return row, apperror.NewBadRequest("the moons generator only adds moons; use action: create (to change a moon, use a kind: calendar block)")
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return row, err
	}
	if cal.UsesRealTime() {
		return row, apperror.NewBadRequest("this calendar follows the real-world date, so its moons can't be changed")
	}
	room := maxGenSkyMoons - len(cal.Moons)
	if room < 1 {
		return row, badRequestf("%s already has %s; the moons generator makes skies of up to %d, so add more in a kind: calendar block", cal.Name, plural(len(cal.Moons), "moon", "moons"), maxGenSkyMoons)
	}
	n, ok, err := countKey(r, "count", 1, room)
	if err != nil {
		return row, err
	}
	if !ok {
		return row, badRequestf("say how many moons to add (`count`, 1 to %d)", room)
	}
	row.cal, row.count = cal, n
	// The engine counts kept moons in its total.
	makes := map[string]any{"count": len(cal.Moons) + n}
	details := map[string]any{}
	for _, key := range []string{"pale", "coloured", "strange"} {
		v, err := pickKey(r, key, genFreqs)
		if err != nil {
			return row, err
		}
		if v != "" {
			makes[key] = v
		}
	}
	if v, err := pickKey(r, "theme", genThemes); err != nil {
		return row, err
	} else if v != "" {
		details["theme"] = v
	}
	if v, err := pickKey(r, "cycles", genMoonCycles); err != nil {
		return row, err
	} else if v != "" {
		details["cycles"] = v
	}
	hidden, _, err := countKey(r, "hidden", 0, n)
	if err != nil {
		return row, err
	}
	details["hidden"] = hidden

	p := buildPlan{Generator: "moons", Seed: genSeed(r), Recipe: map[string]any{"generator": "moons", "makes": makes, "details": details}}
	kept := make([]string, 0, len(cal.Moons))
	for _, m := range cal.Moons {
		p.Locked = append(p.Locked, map[string]any{"name": m.Name, "cycle_days": m.CycleDays})
		kept = append(kept, m.Name)
	}
	row.plan = p
	row.summary = "adds " + plural(n, "moon", "moons") + " to " + cal.Name
	if hidden > 0 {
		row.summary += fmt.Sprintf(" (%d hidden from players)", hidden)
	}
	if len(kept) > 0 {
		row.summary += ", keeping " + strings.Join(kept, ", ") + " as they are"
	}
	return row, nil
}

func (k GeneratorKind) planMoons(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	row, err := k.planMoonsRow(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	return k.clientPlan(row.plan, row.summary, nil)
}

// genMoon is one generated moon as the review screen sends it.
type genMoon struct {
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
	Hidden      bool    `json:"hidden"`
}

// applyMoons checks the generated moons as a calendar block's moons are
// checked, then saves the calendar's structure with every moon it had plus
// the new ones, as the calendar block saves a moons list.
func (k GeneratorKind) applyMoons(ctx context.Context, campaignID string, a Actor, r Record) error {
	row, err := k.planMoonsRow(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if strings.TrimSpace(r.Generated) == "" {
		return apperror.NewBadRequest("the generator did not run in your browser; try Import again")
	}
	var got []genMoon
	if err := json.Unmarshal([]byte(r.Generated), &got); err != nil {
		return apperror.NewBadRequest("the generated moons could not be read")
	}
	if len(got) == 0 || len(got) > row.count {
		return badRequestf("the generator returned %d moons", len(got))
	}
	cal := row.cal
	items := make([]any, 0, len(got))
	for _, m := range got {
		if baseMoon(cal, m.Name) != nil || baseMoonInItems(items, m.Name) {
			return badRequestf("the generator returned the moon %s, which is already on %s; nothing was changed", quote(m.Name), cal.Name)
		}
		items = append(items, map[string]any{"name": m.Name, "cycle": m.CycleDays, "phase_offset": m.PhaseOffset, "color": m.Color, "hidden": m.Hidden})
	}
	s, err := readCalSpec(Record{Fields: map[string]any{"moons": items}}, nil)
	if err != nil {
		return err
	}
	all := make([]calendar.MoonInput, 0, len(cal.Moons)+len(s.moons))
	for _, m := range cal.Moons {
		all = append(all, calendar.MoonInput{Name: m.Name, CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset, Color: m.Color})
	}
	edit, err := structureEdit(&calSpec{moons: append(all, s.moons...)}, cal)
	if err != nil {
		return err
	}
	pv, err := k.Structure.PreviewStructureEdit(ctx, cal.ID, campaignID, *edit)
	if err != nil {
		return err
	}
	_, err = k.Structure.ApplyStructureEdit(ctx, cal.ID, campaignID, pv.Fingerprint, *edit)
	return err
}

func baseMoonInItems(items []any, name string) bool {
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && sameName(fmt.Sprint(m["name"]), name) {
			return true
		}
	}
	return false
}
