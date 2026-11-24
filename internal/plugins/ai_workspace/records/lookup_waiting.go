package records

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// WaitingAPI lists what in a campaign waits on a Director's yes or no,
// adapted in app wiring so this package imports none of the plugins it reads.
type WaitingAPI interface {
	// Waiting lists the queues. olderDays picks which seven-day window of the
	// AI's answered requests comes back in Decided: 0 is the last seven days,
	// 7 the seven before those.
	Waiting(ctx context.Context, campaignID string, a Actor, olderDays int) (WaitingList, error)
}

// The answered requests come in windows of WaitingWindowDays, at most
// WaitingDecidedMax at once. Chronicle keeps only each person's latest 100
// answers, so the cap is the most there can ever be.
const (
	WaitingWindowDays = 7
	WaitingDecidedMax = 100
	// maxWaitingOlder bounds `older` to ten years; a larger number is a typo,
	// and it keeps the day arithmetic far from overflow.
	maxWaitingOlder = 3650
)

// WaitingList is each review queue's items. A queue that couldn't be read is
// named in Missing, so the answer never passes an unread queue off as empty.
type WaitingList struct {
	// Asks are the AI's requests for changes still waiting on the owner's
	// review; Decided are the ones the owner answered in the asked-for
	// window, newest first.
	Asks, Decided []WaitingAsk
	// Entries are pick-list entries players wrote that no Director has
	// accepted yet, handed in or still being written.
	Entries []WaitingEntry
	// Picks are heroes' ask-first picks waiting for a Director's yes.
	Picks []WaitingPick
	// Requests are players' stash moves and ask-to-buy requests.
	Requests []WaitingRequest
	Missing  []string
}

// WaitingAsk is one request for changes an AI sent. Summary is the AI's own
// line; Counts is Chronicle's ("2 to add, 1 to change").
type WaitingAsk struct {
	// ID is the request's number, which withdraw_request takes.
	ID              int64
	Summary, Counts string
	// At is when it was asked, or for a decided one when it was answered.
	At         time.Time
	TurnedDown bool
	// Withdrawn is set when the AI took the request back; Outcome is then
	// the reason it gave.
	Withdrawn bool
	// Outcome is what applying it did, in the review screen's words.
	Outcome string
	// Undone is set once the owner put an applied request's changes back.
	Undone bool
}

// WaitingEntry is one player's unaccepted pick-list entry.
type WaitingEntry struct {
	Name, Field, Writer string
	// For is the character it is kept to, when it is kept to one.
	For string
	// HandedIn is true once the writer sent it for a Director to accept.
	HandedIn bool
	// Note is what a Director asked the writer to change.
	Note string
}

// WaitingPick is one hero's pick of an entry the Directors want to be asked
// about first.
type WaitingPick struct {
	Entry, Field, Player, Hero string
	At                         time.Time
}

// WaitingRequest is one player's request, worded as the DM Screen words it.
type WaitingRequest struct {
	Text string
	At   time.Time
}

// waiting answers `what: waiting`. Every queue is Director business, so
// Safe privacy gets a refusal, as game nights do.
func (run *lookupRun) waiting(r Record) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Waiting for review"}
	if run.l.Waiting == nil {
		return ans, badRequestf("what is waiting for review can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("what is waiting for review is left out in Safe privacy; choose Permitted to include it")
	}
	older, _, err := r.Int("older")
	if err != nil || older < 0 || older > maxWaitingOlder {
		return ans, badRequestf("`older` is a number of days back, 0 or more (like 7 for the week before this one)")
	}
	w, err := run.l.Waiting.Waiting(run.ctx, run.campaignID, run.a, older)
	if err != nil {
		return ans, err
	}
	handedIn := 0
	for _, e := range w.Entries {
		if e.HandedIn {
			handedIn++
		}
	}
	ans.Chip = fmt.Sprintf("Waiting for review · %d", len(w.Asks)+handedIn+len(w.Picks)+len(w.Requests))

	var b strings.Builder
	b.WriteString("## Waiting for review\n\nOnly a Director can say yes or no to these, in Chronicle. Nothing here can be accepted or turned down from a lookup or a request for changes.\n")

	fmt.Fprintf(&b, "\n### Changes your AI asked for, waiting on the owner (%d)\n\n", len(w.Asks))
	var lines []string
	for _, q := range w.Asks {
		lines = append(lines, fmt.Sprintf("- %s%s%s, sent %s", askNumber(q.ID), quoted(q.Summary), askCounts(q.Counts), askDay(q.At)))
	}
	writeOrNone(&b, lines, "None. The owner reviews them on the campaign's Your AI page.")
	if len(w.Asks) > 0 {
		b.WriteString("\nwithdraw_request takes one of these back by its number, with a reason the owner sees.\n")
	}

	b.WriteString("\n### Your AI's requests the owner answered, " + decidedWindow(older) + "\n\n")
	b.WriteString("For 30 days after applying a request the owner can undo it from the Your AI page (What it did); an undone one says so below.\n\n")
	lines = lines[:0]
	for _, q := range w.Decided {
		verdict := "applied"
		if q.Withdrawn {
			verdict = "withdrawn by your AI before the owner answered, nothing changed"
			if o := strings.TrimSpace(q.Outcome); o != "" {
				verdict += "; the reason it gave: " + oneLine(o, 300)
			}
		} else if q.TurnedDown {
			// Said plainly, so the AI doesn't take the owner's No for a
			// fault and send the same changes again.
			verdict = "turned down by the owner, nothing changed; that was their choice, not an error"
		} else if q.Undone {
			// Said plainly, so the AI doesn't build on changes that are gone.
			verdict = "applied, then undone by the owner, so none of its changes stand now"
		} else if o := strings.TrimSpace(q.Outcome); o != "" {
			verdict += ": " + oneLine(o, 1000)
		}
		lines = append(lines, fmt.Sprintf("- %s%s%s, %s (%s)", askNumber(q.ID), quoted(q.Summary), askCounts(q.Counts), verdict, askDay(q.At)))
	}
	b.WriteString(decidedCount(len(w.Decided), older))
	if len(lines) > 0 {
		run.writeLines(&b, lines)
	}

	fmt.Fprintf(&b, "\n### Entries players wrote (%d handed in, %d still being written)\n\n", handedIn, len(w.Entries)-handedIn)
	lines = lines[:0]
	for _, e := range w.Entries {
		line := "- " + quoted(e.Name)
		if e.Field != "" {
			line += " (" + e.Field + ")"
		}
		if e.Writer != "" {
			line += " by " + e.Writer
		}
		if e.For != "" {
			line += ", for " + e.For
		}
		if e.HandedIn {
			line += ": handed in, waiting for a Director to accept it or hand it back"
		} else {
			line += ": still being written"
		}
		if n := strings.TrimSpace(e.Note); n != "" {
			line += "; the Director asked: " + quoted(n)
		}
		lines = append(lines, line)
	}
	writeOrNone(&b, lines, "None.")

	fmt.Fprintf(&b, "\n### Heroes' picks waiting for a Director's yes (%d)\n\n", len(w.Picks))
	lines = lines[:0]
	for _, p := range w.Picks {
		line := "- " + quoted(p.Entry)
		if p.Field != "" {
			line += " (" + p.Field + ")"
		}
		if p.Player != "" {
			line += " asked for by " + p.Player
		}
		if p.Hero != "" {
			line += " for " + p.Hero
		}
		lines = append(lines, line+" ("+askDay(p.At)+")")
	}
	writeOrNone(&b, lines, "None.")

	fmt.Fprintf(&b, "\n### Players' stash moves and requests to buy (%d)\n\n", len(w.Requests))
	lines = lines[:0]
	for _, q := range w.Requests {
		lines = append(lines, fmt.Sprintf("- %s (%s)", oneLine(q.Text, 200), askDay(q.At)))
	}
	writeOrNone(&b, lines, "None.")

	if len(w.Missing) > 0 {
		fmt.Fprintf(&b, "\nCouldn't be read just now, so may also have items waiting: %s.\n", strings.Join(w.Missing, ", "))
	}
	ans.Text = b.String()
	return ans, nil
}

// decidedWindow names the window of answers shown: "in the last 7 days" or
// "from 7 to 14 days ago".
func decidedWindow(older int) string {
	if older == 0 {
		return fmt.Sprintf("in the last %d days", WaitingWindowDays)
	}
	return fmt.Sprintf("from %d to %d days ago", older, older+WaitingWindowDays)
}

// decidedCount says how many answers the window holds and how to read the
// next-older window, so an AI never takes a short list for the whole history.
func decidedCount(n, older int) string {
	var b strings.Builder
	switch {
	case n == 0:
		b.WriteString("None answered " + decidedWindow(older) + ".")
	case n >= WaitingDecidedMax:
		fmt.Fprintf(&b, "Showing the newest %d answers %s, the most one answer shows.", n, decidedWindow(older))
	default:
		fmt.Fprintf(&b, "Showing %s %s.", plural(n, "answer", "answers"), decidedWindow(older))
	}
	fmt.Fprintf(&b, " For older ones ask again with `older: %d` (days %d to %d); Chronicle keeps each person's latest %d answers.\n", older+WaitingWindowDays, older+WaitingWindowDays, older+2*WaitingWindowDays, WaitingDecidedMax)
	return b.String()
}

// writeOrNone writes the lines, or none when there are none.
func writeOrNone(b *strings.Builder, lines []string, none string) {
	if len(lines) == 0 {
		b.WriteString(none + "\n")
		return
	}
	writeLines(b, lines)
}

// quoted marks text someone else wrote, on one line, as quoted.
func quoted(s string) string { return "\"" + oneLine(s, 160) + "\"" }

func askCounts(c string) string {
	if c = strings.TrimSpace(c); c == "" {
		return ""
	}
	return " (" + c + ")"
}

func askDay(t time.Time) string {
	if t.IsZero() {
		return "date unknown"
	}
	return t.UTC().Format("2006-01-02")
}

// askNumber is "request 12: " for a request with a number, so the AI can
// name it to withdraw_request; "" when the number is unknown.
func askNumber(id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("request %d: ", id)
}
