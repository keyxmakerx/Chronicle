package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// EventRollAPI is the roll half of the calendar service (calendar.
// EventRollService), for an event's `roll:`, the rolls generator, one
// night's changes and reading results back.
type EventRollAPI interface {
	GetEventRoll(ctx context.Context, eventID, calendarID, campaignID string, year int, v permissions.Viewer) (*calendar.EventRollView, error)
	ListMonthRolls(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) (map[string][]calendar.RollNightView, error)
	SetEventRoll(ctx context.Context, eventID, calendarID, campaignID string, cfg *calendar.RollConfig, year int, v permissions.Viewer) (*calendar.EventRollView, error)
	RollEventNights(ctx context.Context, eventID, calendarID, campaignID string, in calendar.RollNightsInput, v permissions.Viewer) (*calendar.EventRollView, error)
	LockEventRollNights(ctx context.Context, eventID, calendarID, campaignID string, dates []calendar.DayDate, locked bool, v permissions.Viewer) (int, error)
	TellEventRollNight(ctx context.Context, eventID, calendarID, campaignID string, date calendar.DayDate, told *bool, v permissions.Viewer) error
}

// rollDoc is the `roll:` part of the event kind's doc.
const rollDoc = "To roll each night the event lands on (a blood moon that may or may not bring something), add `roll:` (owner or co-DM only), with either `dice` (like 2d10, 1d20+3) and `results`, or `table` (one of the campaign's rolling tables, by name) and optional `tell_players`. `results` are listed lowest first; each has `name`, `players` (what players are told), `director` (for the Director alone), optional `tell_players: true` (players see that night's result once it arrives), and `max` (the highest total it covers) on every result but the last. `on_the_night: true` rolls a night that arrives without a result. `roll: none` removes the roll and every stored night. Naming `roll:` on update replaces the whole roll; to roll its nights ahead use the `rolls` generator.\n\n" +
	"```\n---\nkind: event\nname: Bloodnight\nyear: 1043\nmonth: 1\nday: 1\nrepeat: rule\nrepeat_on:\n  - moon: Sanguin'mor\n    phase: full\nroll:\n  dice: 2d10\n  on_the_night: true\n  results:\n    - max: 11\n      name: Thin blood\n      players: The moon hangs dull and brown.\n      director: Nothing stirs.\n    - max: 16\n      name: The Hunger\n      players: The moon runs red.\n      director: Each Remnant rolls against its need.\n      tell_players: true\n    - name: Red Tide\n      players: The sea itself looks like blood.\n      director: Feral Remnants rise in Saltreach.\n      tell_players: true\n---\n```"

// rollSpec is what a row's `roll:` asks for.
type rollSpec struct {
	set    bool // the row names a roll at all
	remove bool // roll: none
	cfg    *calendar.RollConfig
}

func (s rollSpec) summary() string {
	switch {
	case !s.set:
		return ""
	case s.remove:
		return "removes its roll"
	case s.cfg.Source == calendar.RollSourceTable:
		return "draws from a rolling table each night"
	}
	return fmt.Sprintf("rolls %s each night (%d results)", s.cfg.Dice, len(s.cfg.Results))
}

var rollKeys = map[string]bool{"dice": true, "table": true, "on_the_night": true, "tell_players": true, "results": true}
var rollResultKeys = map[string]bool{"max": true, "name": true, "players": true, "director": true, "tell_players": true}

// readRoll reads a row's `roll:`. A table is named by the campaign's own
// name for it and stored by id.
func readRoll(ctx context.Context, campaignID string, a Actor, r Record, tables RollTablesAPI) (rollSpec, error) {
	var s rollSpec
	v, ok := r.Fields["roll"]
	if !ok {
		return s, nil
	}
	s.set = true
	if !a.CanAuthorDmOnly() {
		return s, apperror.NewBadRequest("only the owner or a co-DM can give an event a roll")
	}
	if str, isStr := v.(string); isStr && strings.EqualFold(strings.TrimSpace(str), "none") {
		s.remove = true
		return s, nil
	}
	raw, isMap := v.(map[string]any)
	if !isMap {
		return s, apperror.NewBadRequest("roll must be keys such as `dice:` and `results:`, or `none`")
	}
	m := lowerMap(raw)
	for k := range m {
		if !rollKeys[k] {
			return s, badRequestf("%q is not a roll key", k)
		}
	}
	str := func(m map[string]any, k string) string {
		if m[k] == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(m[k]))
	}
	boolean := func(m map[string]any, k string) bool {
		switch t := m[k].(type) {
		case bool:
			return t
		case string:
			t = strings.ToLower(strings.TrimSpace(t))
			return t == "true" || t == "yes" || t == "on"
		}
		return false
	}
	cfg := &calendar.RollConfig{OnTheNight: boolean(m, "on_the_night")}
	if name := str(m, "table"); name != "" {
		if m["dice"] != nil || m["results"] != nil {
			return s, apperror.NewBadRequest("a roll uses dice and results, or a table, not both")
		}
		id, err := findRollTable(ctx, campaignID, tables, name)
		if err != nil {
			return s, err
		}
		cfg.Source, cfg.TableID, cfg.TellPlayers = calendar.RollSourceTable, id, boolean(m, "tell_players")
	} else {
		cfg.Source, cfg.Dice = calendar.RollSourceDice, str(m, "dice")
		items, _ := m["results"].([]any)
		for i, it := range items {
			rm, ok := it.(map[string]any)
			if !ok {
				return s, badRequestf("roll result %d must be keys such as `name:` and `max:`", i+1)
			}
			rm = lowerMap(rm)
			for k := range rm {
				if !rollResultKeys[k] {
					return s, badRequestf("roll result %d: %q is not a result key", i+1, k)
				}
			}
			b := calendar.RollBand{Name: str(rm, "name"), Players: str(rm, "players"), Director: str(rm, "director"), TellPlayers: boolean(rm, "tell_players")}
			if rm["max"] != nil {
				n, ok, err := (Record{Fields: map[string]any{"max": rm["max"]}}).Int("max")
				if err != nil || !ok {
					return s, badRequestf("roll result %d: max must be a whole number", i+1)
				}
				b.Max = &n
			}
			cfg.Results = append(cfg.Results, b)
		}
	}
	if err := cfg.Normalize(); err != nil {
		return s, badRequestf("roll: %s", planError(err))
	}
	s.cfg = cfg
	return s, nil
}

// findRollTable finds one of the campaign's rolling tables by name or id.
func findRollTable(ctx context.Context, campaignID string, tables RollTablesAPI, name string) (string, error) {
	if tables == nil {
		return "", apperror.NewBadRequest("rolling tables aren't available on this server")
	}
	doc, err := tables.Get(ctx, campaignID)
	if err != nil {
		return "", apperror.NewBadRequest("could not read the rolling tables")
	}
	for _, t := range doc.Tables {
		if sameName(t.Name, name) || t.ID == name {
			return t.ID, nil
		}
	}
	return "", badRequestf("no rolling table called %s", quote(name))
}

// applyRoll stores the row's roll on an event.
func applyRoll(ctx context.Context, rolls EventRollAPI, cal *calendar.Calendar, campaignID, eventID string, a Actor, s rollSpec) error {
	if !s.set {
		return nil
	}
	if rolls == nil {
		return apperror.NewBadRequest("event rolls aren't available on this server")
	}
	_, err := rolls.SetEventRoll(ctx, eventID, cal.ID, campaignID, s.cfg, cal.CurrentYear, a.Viewer())
	return err
}

// rollEvent finds the event a rolls row names, by name, on the main
// calendar. It must be unique and already have a roll.
func rollEvent(ctx context.Context, svc CalendarAPI, cal *calendar.Calendar, campaignID string, a Actor, name string) (*calendar.Event, error) {
	if name == "" {
		return nil, apperror.NewBadRequest("say which event to roll (`event`)")
	}
	evs, err := svc.ListEventsForCalendar(ctx, campaignID, cal.ID, a.Role)
	if err != nil {
		return nil, apperror.NewBadRequest("could not read the calendar's events")
	}
	var hit *calendar.Event
	for i := range evs {
		if !sameName(evs[i].Name, name) {
			continue
		}
		if hit != nil {
			return nil, badRequestf("more than one event is called %s; rename one first", quote(name))
		}
		hit = &evs[i]
	}
	if hit == nil {
		return nil, badRequestf("no event called %s on %s", quote(name), cal.Name)
	}
	return hit, nil
}

// RollNightKind changes one night of an event's roll: reroll it, lock or
// unlock it, tell players or stop telling them.
type RollNightKind struct {
	Cal   CalendarAPI
	Rolls EventRollAPI
}

func (RollNightKind) Name() string  { return "roll_night" }
func (RollNightKind) Label() string { return "Rolled night" }
func (RollNightKind) Doc() string {
	return "One night of an event's roll (owner or co-DM only), matched by the event's `name` and the night's `year`, `month`, `day`. Use `action: update` with any of `reroll: true` (a locked night is refused), `locked` (true/false: a locked night keeps its result when the year is rolled again) and `tell_players` (true/false, or `default` for what its result says). Ask `what: events` for the results so far.\n\n" +
		"```\n---\nkind: roll_night\naction: update\nname: Bloodnight\nyear: 1043\nmonth: 3\nday: 17\nreroll: true\nlocked: true\n---\n```"
}

type rollNightPlan struct {
	cal    *calendar.Calendar
	ev     *calendar.Event
	date   calendar.DayDate
	reroll bool
	lock   *bool
	tell   *bool
	tellOn bool // tell_players was written
}

func (k RollNightKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (rollNightPlan, string, error) {
	var p rollNightPlan
	if !a.CanAuthorDmOnly() {
		return p, "", apperror.NewBadRequest("only the owner or a co-DM can change a rolled night")
	}
	if k.Rolls == nil {
		return p, "", apperror.NewBadRequest("event rolls aren't available on this server")
	}
	if r.Action != ActionUpdate {
		return p, "", apperror.NewBadRequest("a rolled night only takes action: update")
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return p, "", err
	}
	ev, err := rollEvent(ctx, k.Cal, cal, campaignID, a, r.Name)
	if err != nil {
		return p, "", err
	}
	y, m, d, ok, err := readDate(cal, r, "")
	if err != nil {
		return p, "", err
	}
	if !ok {
		return p, "", apperror.NewBadRequest("say which night with year, month and day")
	}
	p.cal, p.ev, p.date = cal, ev, calendar.DayDate{Year: y, Month: m, Day: d}
	var did []string
	if v, ok := r.Bool("reroll"); ok && v {
		p.reroll = true
		did = append(did, "rerolls")
	}
	if r.Has("locked") {
		v, _ := r.Bool("locked")
		p.lock = &v
		did = append(did, map[bool]string{true: "locks", false: "unlocks"}[v])
	}
	if r.Has("tell_players") {
		p.tellOn = true
		if !strings.EqualFold(r.Str("tell_players"), "default") {
			v, _ := r.Bool("tell_players")
			p.tell = &v
			did = append(did, map[bool]string{true: "tells players", false: "stops telling players"}[v])
		} else {
			did = append(did, "tells players as its result says")
		}
	}
	if len(did) == 0 {
		return p, "", apperror.NewBadRequest("say what to do: reroll, locked or tell_players")
	}
	return p, strings.Join(did, ", ") + " " + ev.Name + " on " + dateLabel(cal, y, m, d), nil
}

func (k RollNightKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	_, summary, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	return Plan{Summary: summary}
}

func (k RollNightKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, _, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	v := a.Viewer()
	// Unlock first so "reroll: true, locked: false" rerolls, and lock last
	// so "reroll: true, locked: true" keeps the new result.
	if p.lock != nil && !*p.lock {
		if _, err := k.Rolls.LockEventRollNights(ctx, p.ev.ID, p.cal.ID, campaignID, []calendar.DayDate{p.date}, false, v); err != nil {
			return err
		}
	}
	if p.reroll {
		if _, err := k.Rolls.RollEventNights(ctx, p.ev.ID, p.cal.ID, campaignID, calendar.RollNightsInput{Dates: []calendar.DayDate{p.date}}, v); err != nil {
			return err
		}
	}
	if p.lock != nil && *p.lock {
		if _, err := k.Rolls.LockEventRollNights(ctx, p.ev.ID, p.cal.ID, campaignID, []calendar.DayDate{p.date}, true, v); err != nil {
			return err
		}
	}
	if p.tellOn {
		return k.Rolls.TellEventRollNight(ctx, p.ev.ID, p.cal.ID, campaignID, p.date, p.tell, v)
	}
	return nil
}

func (RollNightKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
