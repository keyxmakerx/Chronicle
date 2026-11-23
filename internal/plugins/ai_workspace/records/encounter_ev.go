package records

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The encounters lookup adds up the EV written on each creature page, so an
// AI planning a fight sees what the pages say it costs. It only adds: the
// party's budget and the difficulty come from the game system's published
// formulas, which run in the system's own panel in the encounter builder,
// never here, so the answer says so instead of guessing them.

// evFieldKey and orgFieldKey are the creature fields a game system's preset
// declares for a creature's encounter value and its organization (Draw
// Steel's words, the same EV Foundry reports for its monsters). A campaign
// whose categories declare no EV field gets no EV lines at all.
const (
	evFieldKey  = "ev"
	orgFieldKey = "organization"
	// minionOrg is the organization whose EV a page gives per set of
	// creatures, not per creature; multiplying it by the count would
	// overstate the fight, so those rows are named and left out.
	minionOrg = "minion"
)

// evNote is the one line under the heading that says what the EV lines are.
const evNote = "EV lines add up the EV written on each creature page, times how many there are. They are not the party's budget or the fight's difficulty: the encounter builder's game-system panel works those out from the published formulas. A creature without EV on its page is named, never guessed.\n\n"

// evReader reads EV off the creature pages, once per lookup.
type evReader struct {
	pages map[string]*entities.Entity
	// cut is true when the page walk stopped early, so a page not found
	// may simply not have been read.
	cut bool
}

// encounterEV is the reader for this lookup, or nil when the campaign
// declares no EV field (a system without one) or pages can't be read.
func (run *lookupRun) encounterEV() *evReader {
	if run.l.Pages == nil {
		return nil
	}
	types, err := run.l.Pages.GetEntityTypes(run.ctx, run.campaignID)
	if err != nil || !anyTypeHasField(types, evFieldKey) {
		return nil
	}
	pages, err := run.visiblePages()
	if err != nil {
		return nil
	}
	r := &evReader{pages: make(map[string]*entities.Entity, len(pages)), cut: run.pagesCut}
	for i := range pages {
		r.pages[pages[i].ID] = &pages[i]
	}
	return r
}

func anyTypeHasField(types []entities.EntityType, key string) bool {
	for _, t := range types {
		for _, f := range t.Fields {
			if f.Key == key {
				return true
			}
		}
	}
	return false
}

// evOf reads a page's stored EV; ok is false when it holds no positive
// number. A director may write EV by hand, so a number typed as text counts.
func evOf(fields map[string]any) (float64, bool) {
	var n float64
	switch v := fields[evFieldKey].(type) {
	case float64:
		n = v
	case int:
		n = float64(v)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	return n, n > 0
}

func isMinion(fields map[string]any) bool {
	s, _ := fields[orgFieldKey].(string)
	return strings.EqualFold(strings.TrimSpace(s), minionOrg)
}

// line is one creature list's EV as answer lines: the sum, then what is not
// in it and why. label names the list ("" for the base creatures).
func (r *evReader) line(label string, cs []EncounterCreature) string {
	var parts, noPage, noEV, unread, minions []string
	var sum float64
	for _, c := range cs {
		if c.EntityID == "" {
			noPage = append(noPage, quote(c.Name))
			continue
		}
		p := r.pages[c.EntityID]
		if p == nil {
			unread = append(unread, quote(c.Name))
			continue
		}
		ev, ok := evOf(p.FieldsData)
		switch {
		case !ok:
			noEV = append(noEV, quote(c.Name))
		case isMinion(p.FieldsData):
			minions = append(minions, fmt.Sprintf("%d %s (EV %s on its page)", c.Count, c.Name, fmtEV(ev)))
		default:
			sum += ev * float64(c.Count)
			parts = append(parts, fmt.Sprintf("%d × %s at EV %s", c.Count, c.Name, fmtEV(ev)))
		}
	}
	var b strings.Builder
	head := "EV"
	if label != "" {
		head = "EV of " + quote(label)
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, "%s, added up from the creature pages: %s (%s).\n", head, fmtEV(sum), strings.Join(parts, ", "))
	} else {
		fmt.Fprintf(&b, "%s: no creature here has EV on its page.\n", head)
	}
	if len(noPage) > 0 {
		b.WriteString("Not counted, no page: " + strings.Join(noPage, ", ") + ".\n")
	}
	if len(noEV) > 0 {
		b.WriteString("Not counted, no EV on the page: " + strings.Join(noEV, ", ") + ".\n")
	}
	if len(unread) > 0 {
		why := "the page could not be read"
		if r.cut {
			why = "the page was past the pages read"
		}
		b.WriteString("Not counted, " + why + ": " + strings.Join(unread, ", ") + ".\n")
	}
	if len(minions) > 0 {
		b.WriteString("Not counted, minions, whose page EV is for a set of them rather than each one (the builder's panel prices the sets): " + strings.Join(minions, ", ") + ".\n")
	}
	return b.String()
}

// fmtEV writes a whole EV without a decimal point.
func fmtEV(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
