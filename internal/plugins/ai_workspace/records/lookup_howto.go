package records

import (
	"fmt"
	"sort"
	"strings"
)

// lookupWhatFormat is the `what:` that hands back one kind's block format.
const lookupWhatFormat = "format"

// format answers `what: format`: the full doc of one kind, read from the
// registry so a kind added later is answerable without a change here.
func (run *lookupRun) format(kind string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Format"}
	if run.l.Kinds == nil {
		return ans, badRequestf("block formats can't be looked up on this server")
	}
	kind = normKind(kind)
	if kind == "" {
		return ans, badRequestf("it needs `block:`, the kind to explain: one of %s", run.kindNames())
	}
	k, ok := run.l.Kinds.Get(kind)
	if !ok {
		ans.Chip = "Format · " + kind
		return ans, badRequestf("there is no `kind: %s`; the kinds are %s", kind, run.kindNames())
	}
	ans.Chip = "Format · " + k.Name()
	ans.Text = "## Format: kind: " + k.Name() + " (" + k.Label() + ")\n\n" + strings.TrimSpace(k.Doc()) + "\n"
	return ans, nil
}

func (run *lookupRun) kindNames() string {
	var names []string
	for _, k := range run.l.Kinds.Kinds() {
		names = append(names, k.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// isNoCalendar reports a refusal caused by the campaign having no main
// calendar, which a lookup answers with how to make one rather than as a
// failure.
func isNoCalendar(err error) bool {
	return err != nil && strings.HasPrefix(planError(err), needsCalendarPrefix)
}

// noCalendar is the answer to a calendar, events or weather lookup when
// there is no main calendar: which calendars exist (as the Actor sees
// them) and the block that fixes it.
func (run *lookupRun) noCalendar(chip string) LookupAnswer {
	ans := LookupAnswer{Chip: chip + " · no main calendar"}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", chip)
	if chip != "Calendar" {
		b.WriteString("None: the campaign has no main calendar yet. Ask `what: calendar` for how to set one up.\n")
		ans.Text = b.String()
		return ans
	}
	var names []string
	if l, ok := run.l.Cal.(calendarLister); ok {
		if cals, err := l.ListCalendars(run.ctx, run.campaignID, run.a.Viewer()); err == nil {
			for _, c := range cals {
				names = append(names, quote(c.Name))
			}
		}
	}
	if len(names) > 0 {
		fmt.Fprintf(&b, "Not set up yet: the campaign has %s, but none is the main calendar. Events, weather and generators all use the main one.\n\n", plural(len(names), "calendar ("+names[0]+")", "calendars ("+strings.Join(names, ", ")+")"))
		b.WriteString("To make one of them the main calendar (the owner only), send:\n\n```\n---\nkind: calendar\naction: update\nname: " + names[0] + "\nmain: true\n---\n```\n\n")
		b.WriteString("Any other `kind: calendar` key in that block changes the calendar at the same time. To make a new calendar instead, use `action: create`; ask `what: format` with `block: calendar` for every key.\n")
	} else {
		b.WriteString("The campaign has no calendar yet. Events, weather and generators all need one.\n\n")
		b.WriteString("To make one (the owner only), send a `kind: calendar` block with `action: create`. It needs `name`, `months`, `weekdays` and today's date (`current_year`, `current_month`, `current_day`), and becomes the main calendar. Its format:\n\n")
		b.WriteString(strings.TrimSpace(CalendarKind{}.Doc()) + "\n")
	}
	ans.Text = b.String()
	return ans
}
