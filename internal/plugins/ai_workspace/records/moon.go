package records

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// MoonKind sets a moon of the main calendar by hand (Umbra): how it moves,
// the words for its states, and its marks. A moon's name, cycle and colour
// stay in the calendar block.
type MoonKind struct {
	Cal   CalendarAPI
	Marks calendar.MoonMarkService
}

func (MoonKind) Name() string  { return "moon" }
func (MoonKind) Label() string { return "Moon set by hand" }
func (MoonKind) Doc() string {
	return "A moon of the main calendar that the Director sets by hand instead of following its cycle (owner or co-DM only), matched by `name`; the moon itself comes from the calendar block. Use `action: update` with any of `moves` (`by_hand` or `cycle`), `words` (your word for each state, replacing every word: `full`, `half`, `crescent`, `dark`, `vanished`; one left out reads as the state's own name) and `marks`. A mark sets the moon's `state` from its `year`, `month`, `day` until the next mark; `players_see: true` lets players see that change, and `remove: true` removes the mark on that day. A moon that moves by cycle keeps its marks for when it is set by hand again. A real-world calendar's moon cannot be set by hand.\n\n" +
		"```\n---\nkind: moon\naction: update\nname: Umbra\nmoves: by_hand\nwords: {vanished: Gone below}\nmarks:\n  - {year: 1043, month: Ashfall, day: 3, state: full}\n  - {year: 1043, month: Ashfall, day: 9, state: vanished, players_see: true}\n  - {year: 1043, month: Ashfall, day: 21, remove: true}\n---\n```"
}

var moonMarkKeys = map[string]bool{"year": true, "month": true, "day": true, "state": true, "players_see": true, "remove": true}

type moonPlan struct {
	cal    *calendar.Calendar
	moon   *calendar.Moon
	mode   string            // "" keeps it
	names  map[string]string // with nameOn, replaces every word
	nameOn bool
	puts   []calendar.MoonMark
	dels   []calendar.DayDate
}

func (k MoonKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (moonPlan, string, error) {
	var p moonPlan
	if !a.CanAuthorDmOnly() {
		return p, "", apperror.NewBadRequest("only the owner or a co-DM can set a moon by hand")
	}
	if k.Marks == nil {
		return p, "", apperror.NewBadRequest("moons set by hand aren't available on this server")
	}
	if r.Action != ActionUpdate {
		return p, "", apperror.NewBadRequest("a moon block only takes action: update; add the moon itself in the calendar block")
	}
	cal, err := defaultCalendar(ctx, k.Cal, campaignID, a)
	if err != nil {
		return p, "", err
	}
	for i := range cal.Moons {
		if sameName(cal.Moons[i].Name, r.Name) {
			p.moon = &cal.Moons[i]
		}
	}
	if p.moon == nil {
		if cal.ID == "" {
			return p, "", badRequestf("no moon called %s on the calendar this import makes; add it to that calendar block's moons", quote(r.Name))
		}
		return p, "", badRequestf("no moon called %s on %s", quote(r.Name), cal.Name)
	}
	p.cal = cal
	name := p.moon.Name
	var did []string

	if r.Has("moves") {
		switch strings.ReplaceAll(strings.ToLower(r.Str("moves")), " ", "_") {
		case "by_hand", "hand", "manual":
			p.mode = calendar.MoonManual
		case "cycle", "by_cycle":
			p.mode = calendar.MoonCycle
		default:
			return p, "", apperror.NewBadRequest("moves is `by_hand` or `cycle`")
		}
		if p.mode == calendar.MoonManual && cal.IsRealLife() {
			return p, "", badRequestf("%s follows the real Moon and cannot be set by hand", name)
		}
		if p.mode != p.moon.PhaseMode {
			did = append(did, map[string]string{calendar.MoonManual: "sets " + name + " by hand", calendar.MoonCycle: "has " + name + " follow its cycle"}[p.mode])
		}
	}
	if r.Has("words") {
		raw, ok := r.Fields["words"].(map[string]any)
		if !ok && r.Fields["words"] != nil {
			return p, "", apperror.NewBadRequest("words is a state and its word, like `{vanished: Gone below}`")
		}
		p.nameOn, p.names = true, map[string]string{}
		for st, w := range lowerMap(raw) {
			if !calendar.ValidMoonState(st) {
				return p, "", badRequestf("words: %q is not a state; use full, half, crescent, dark or vanished", st)
			}
			word := strings.TrimSpace(fmt.Sprint(w))
			if w == nil {
				word = ""
			}
			if utf8.RuneCountInString(word) > calendar.MaxMoonStateWord {
				return p, "", badRequestf("words: a state's word is at most %d characters", calendar.MaxMoonStateWord)
			}
			p.names[st] = word
		}
		did = append(did, "words for "+name+"'s states")
	}

	if r.Has("marks") {
		mode := p.mode
		if mode == "" {
			mode = p.moon.PhaseMode
		}
		if mode != calendar.MoonManual {
			return p, "", badRequestf("%s follows its cycle; add `moves: by_hand` to give it marks", name)
		}
		items, _ := r.Fields["marks"].([]any)
		if items == nil {
			return p, "", apperror.NewBadRequest("marks is a list of `{year, month, day, state}`")
		}
		seen := map[calendar.DayDate]bool{}
		for i, it := range items {
			raw, ok := it.(map[string]any)
			if !ok {
				return p, "", badRequestf("mark %d needs keys such as year, month, day and state", i+1)
			}
			mr := Record{Fields: lowerMap(raw)}
			for key := range mr.Fields {
				if !moonMarkKeys[key] {
					return p, "", badRequestf("mark %d: %q is not a mark key", i+1, key)
				}
			}
			y, m, d, ok, err := readDate(cal, mr, "")
			if err != nil {
				return p, "", badRequestf("mark %d: %s", i+1, planError(err))
			}
			if !ok {
				return p, "", badRequestf("mark %d needs a year, month and day", i+1)
			}
			dd := calendar.DayDate{Year: y, Month: m, Day: d}
			if seen[dd] {
				return p, "", badRequestf("mark %d: %s is listed twice", i+1, dateLabel(cal, y, m, d))
			}
			seen[dd] = true
			if rm, _ := mr.Bool("remove"); rm {
				p.dels = append(p.dels, dd)
				continue
			}
			st := strings.ToLower(mr.Str("state"))
			if !calendar.ValidMoonState(st) {
				return p, "", badRequestf("mark %d: state is full, half, crescent, dark or vanished", i+1)
			}
			shown, _ := mr.Bool("players_see")
			p.puts = append(p.puts, calendar.MoonMark{Year: y, Month: m, Day: d, State: st, Shown: shown})
		}
		if n := len(p.puts); n > 0 {
			did = append(did, "sets "+plural(n, "mark", "marks"))
		}
		if n := len(p.dels); n > 0 {
			did = append(did, "removes "+plural(n, "mark", "marks"))
		}
	}
	if len(did) == 0 {
		return p, "", apperror.NewBadRequest("say what to change: moves, words or marks")
	}
	return p, strings.Join(did, "; "), nil
}

func (k MoonKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	_, summary, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	return Plan{Summary: summary}
}

func (k MoonKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, _, err := k.plan(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	v := a.Viewer()
	if p.mode != "" || p.nameOn {
		mode, names := p.mode, p.moon.StateNames
		if mode == "" {
			mode = p.moon.PhaseMode
		}
		if p.nameOn {
			names = p.names
		}
		if _, err := k.Marks.SetMoonMode(ctx, p.cal.ID, campaignID, p.moon.ID, mode, names, v); err != nil {
			return err
		}
	}
	for _, d := range p.dels {
		// A mark already gone is what the block asked for.
		if _, err := k.Marks.DeleteMoonMark(ctx, p.cal.ID, campaignID, p.moon.ID, d, v); err != nil && apperror.SafeCode(err) != http.StatusNotFound {
			return err
		}
	}
	for _, mk := range p.puts {
		if _, err := k.Marks.SetMoonMark(ctx, p.cal.ID, campaignID, p.moon.ID, mk, v); err != nil {
			return err
		}
	}
	return nil
}

// Export lists the main calendar's moons set by hand, with every mark, so
// an AI can carry them on.
func (k MoonKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() || k.Marks == nil {
		return "", nil
	}
	cal, err := k.Cal.GetDefaultCalendarForViewer(ctx, campaignID, a.Viewer())
	if err != nil || cal == nil {
		return "", nil
	}
	var b strings.Builder
	for _, m := range cal.Moons {
		if !m.Manual() {
			continue
		}
		b.WriteString("---\nkind: moon\nname: " + quote(m.Name) + "\nmoves: by_hand\n")
		if len(m.StateNames) > 0 {
			states := make([]string, 0, len(m.StateNames))
			for st := range m.StateNames {
				states = append(states, st)
			}
			sort.Strings(states)
			b.WriteString("words:\n")
			for _, st := range states {
				b.WriteString("  " + st + ": " + quote(m.StateNames[st]) + "\n")
			}
		}
		if len(m.Marks) > 0 {
			b.WriteString("marks:\n")
			for _, mk := range m.Marks {
				fmt.Fprintf(&b, "  - {year: %d, month: %d, day: %d, state: %s, players_see: %t}\n", mk.Year, mk.Month, mk.Day, mk.State, mk.Shown)
			}
		}
		b.WriteString("---\n\n")
	}
	return b.String(), nil
}
