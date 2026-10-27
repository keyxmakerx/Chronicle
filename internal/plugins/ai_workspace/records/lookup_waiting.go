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
	Waiting(ctx context.Context, campaignID string, a Actor) (WaitingList, error)
}

// WaitingList is each review queue's items. A queue that couldn't be read is
// named in Missing, so the answer never passes an unread queue off as empty.
type WaitingList struct {
	// Asks are the AI's requests for changes still waiting on the owner's
	// review; Decided are the latest ones the owner answered.
	Asks, Decided []WaitingAsk
	// Entries are pick-list entries players wrote that no Director has
	// accepted yet, handed in or still being written.
	Entries []WaitingEntry
	// Requests are players' stash moves and ask-to-buy requests.
	Requests []WaitingRequest
	Missing  []string
}

// WaitingAsk is one request for changes an AI sent. Summary is the AI's own
// line; Counts is Chronicle's ("2 to add, 1 to change").
type WaitingAsk struct {
	Summary, Counts string
	// At is when it was asked, or for a decided one when it was answered.
	At         time.Time
	TurnedDown bool
	// Outcome is what applying it did, in the review screen's words.
	Outcome string
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

// WaitingRequest is one player's request, worded as the DM Screen words it.
type WaitingRequest struct {
	Text string
	At   time.Time
}

// waiting answers `what: waiting`. Every queue is Director business, so
// Safe privacy gets a refusal, as game nights do.
func (run *lookupRun) waiting() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Waiting for review"}
	if run.l.Waiting == nil {
		return ans, badRequestf("what is waiting for review can't be looked up on this server")
	}
	if !run.director() {
		return ans, badRequestf("what is waiting for review is left out in Safe privacy; choose Permitted to include it")
	}
	w, err := run.l.Waiting.Waiting(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	handedIn := 0
	for _, e := range w.Entries {
		if e.HandedIn {
			handedIn++
		}
	}
	ans.Chip = fmt.Sprintf("Waiting for review · %d", len(w.Asks)+handedIn+len(w.Requests))

	var b strings.Builder
	b.WriteString("## Waiting for review\n\nOnly a Director can say yes or no to these, in Chronicle. Nothing here can be accepted or turned down from a lookup or a request for changes.\n")

	fmt.Fprintf(&b, "\n### Changes your AI asked for, waiting on the owner (%d)\n\n", len(w.Asks))
	var lines []string
	for _, q := range w.Asks {
		lines = append(lines, fmt.Sprintf("- %s%s, sent %s", quoted(q.Summary), askCounts(q.Counts), askDay(q.At)))
	}
	writeOrNone(&b, lines, "None. The owner reviews them on the campaign's Your AI page.")

	if len(w.Decided) > 0 {
		b.WriteString("\n### Your AI's requests the owner answered lately\n\n")
		lines = lines[:0]
		for _, q := range w.Decided {
			verdict := "applied"
			if q.TurnedDown {
				verdict = "turned down, nothing changed"
			} else if o := strings.TrimSpace(q.Outcome); o != "" {
				verdict += ": " + oneLine(o, 160)
			}
			lines = append(lines, fmt.Sprintf("- %s%s, %s (%s)", quoted(q.Summary), askCounts(q.Counts), verdict, askDay(q.At)))
		}
		writeLines(&b, lines)
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
