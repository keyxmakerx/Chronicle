package systems

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A Director's save is live at once. When it removes or alters a trait some
// hero holds, the save asks once whether to tell the players, with a line to
// send already written. Nothing is asked for a change no hero's trait feels,
// for an entry players do not read, or for a player's own draft. The question
// is only a suggestion: the save stands whether or not it can be asked.

// TellQuestion is what a save asks the Director, beside the saved entry.
type TellQuestion struct {
	// Holders says who holds what changed: "Sable Varr holds this trait."
	Holders string `json:"holders"`
	// Line is the suggested line for the players, which the Director may edit.
	Line string `json:"line"`
	// Heroes and Traits name who and what, in the order they were found.
	Heroes []string `json:"heroes"`
	Traits []string `json:"traits"`
}

// WithEntryPeople lets a save find the heroes who hold a changed trait.
// Without it no save asks to tell the players.
func WithEntryPeople(p EntryPeople) EntryServiceOption {
	return func(s *systemEntryService) { s.people = p }
}

func (s *systemEntryService) UpdateTelling(ctx context.Context, campaignID string, actor EntryActor, role int, id int64, in UpdateSystemEntryInput) (*SystemEntry, *TellQuestion, error) {
	before, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, nil, err
	}
	was := *before
	was.Properties = copyProps(before.Properties)
	after, err := s.Update(ctx, campaignID, actor, id, in)
	if err != nil {
		return nil, nil, err
	}
	q, err := s.tellQuestion(ctx, campaignID, actor, role, &was, after)
	if err != nil {
		// The save already stands; only the question is lost.
		slog.Warn("could not work out whom an entry change reaches", slog.Int64("entry", id), slog.Any("error", err))
		return after, nil, nil
	}
	return after, q, nil
}

// playersRead reports whether players read this version of an entry.
func playersRead(e *SystemEntry) bool {
	return entryStatusOrAccepted(e.Status) == EntryStatusAccepted && e.Visibility != EntryVisibilityDirectors
}

// tellQuestion is the rule: a Director's save of an entry players read, both
// before and after, that removed or altered a trait a hero holds.
func (s *systemEntryService) tellQuestion(ctx context.Context, campaignID string, actor EntryActor, role int, before, after *SystemEntry) (*TellQuestion, error) {
	if !actor.IsDirector || s.people == nil || !playersRead(before) || !playersRead(after) {
		return nil, nil
	}
	changes := ChangedTraits(before, after)
	if len(changes) == 0 {
		return nil, nil
	}
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return nil, nil
	}
	people, err := s.people.People(ctx, campaignID, after.FieldKey, PeopleReader{UserID: actor.UserID, Role: role}, maxPeopleScanned)
	if err != nil {
		return nil, err
	}
	category := kindCategory(m, after.FieldKey)
	leads := map[string]bool{}
	held := map[int]bool{}
	var heroes []string
	for _, c := range people {
		if !c.Hero {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(c.Value))
		ok, seen := leads[key]
		if !seen {
			res, err := s.ResolveName(ctx, campaignID, category, c.Value, actor)
			if err != nil {
				return nil, err
			}
			ok = res.Entry != nil && res.Entry.EntryID == after.ID &&
				(res.Kind == ResolvedEntry || res.Kind == ResolvedRedirect)
			leads[key] = ok
		}
		if !ok {
			continue
		}
		holds := false
		for i, ch := range changes {
			for _, t := range c.Traits {
				if strings.EqualFold(strings.TrimSpace(t), ch.Name) {
					held[i], holds = true, true
				}
			}
		}
		if holds {
			heroes = append(heroes, c.Name)
		}
	}
	if len(heroes) == 0 {
		return nil, nil
	}
	q := &TellQuestion{Heroes: heroes}
	was, now := namedLists(before.Properties), namedLists(after.Properties)
	var lines []string
	for i, ch := range changes {
		if !held[i] {
			continue
		}
		q.Traits = append(q.Traits, ch.Name)
		lines = append(lines, describeTraitChange(ch, was, now))
	}
	q.Holders = holdersLine(heroes, len(q.Traits))
	q.Line = clipRunes(strings.Join(lines, "; "), maxNoteLine)
	return q, nil
}

// holdersLine is "Sable Varr holds this trait." or "Sable Varr and Wren hold
// these traits."
func holdersLine(heroes []string, traits int) string {
	verb := "holds"
	if len(heroes) > 1 {
		verb = "hold"
	}
	what := "this trait"
	if traits > 1 {
		what = "these traits"
	}
	return joinNames(heroes) + " " + verb + " " + what + "."
}

// describeTraitChange is the suggested line for one trait: what the players
// would need to know, read from the two texts where a number changed.
func describeTraitChange(ch TraitChange, was, now map[string]map[string]namedItem) string {
	key := strings.ToLower(ch.Name)
	if ch.Removed {
		return ch.Name + " removed"
	}
	old, okOld := was[ch.List][key]
	cur, okNow := now[ch.List][key]
	if !okNow {
		cur, okNow = findItem(now, key)
	}
	cost := ""
	if ch.CostChanged {
		cost = "now costs " + pointsWord(ch.CostAfter)
		if ch.CostBefore != nil {
			cost += ", was " + strconv.Itoa(*ch.CostBefore)
		}
	}
	if !ch.TextChanged {
		return ch.Name + " " + cost
	}
	line := ch.Name + " reworded"
	if okOld && okNow && sameJSON(old.Extra, cur.Extra) {
		if n, ok := numberChange(old.Text, cur.Text); ok {
			line = ch.Name + ": " + n
		}
	}
	if cost != "" {
		line += "; it " + cost
	}
	return line
}

func pointsWord(c *int) string {
	if c == nil {
		return "nothing"
	}
	if *c == 1 {
		return "1 point"
	}
	return strconv.Itoa(*c) + " points"
}

var numberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)
var unitRe = regexp.MustCompile(`^\s*([A-Za-z]+)`)

// numberChange reads "5 squares, was 10" from two texts that differ only in
// one number; false when they differ in anything else.
func numberChange(was, now string) (string, bool) {
	was, now = strings.TrimSpace(was), strings.TrimSpace(now)
	if numberRe.ReplaceAllString(was, "#") != numberRe.ReplaceAllString(now, "#") {
		return "", false
	}
	a, b := numberRe.FindAllStringIndex(was, -1), numberRe.FindAllStringIndex(now, -1)
	if len(a) != len(b) {
		return "", false
	}
	diff := -1
	for i := range a {
		if was[a[i][0]:a[i][1]] != now[b[i][0]:b[i][1]] {
			if diff >= 0 {
				return "", false
			}
			diff = i
		}
	}
	if diff < 0 {
		return "", false
	}
	n := now[b[diff][0]:b[diff][1]]
	o := was[a[diff][0]:a[diff][1]]
	if m := unitRe.FindStringSubmatch(now[b[diff][1]:]); m != nil {
		return n + " " + m[1] + ", was " + o, true
	}
	return n + ", was " + o, true
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
