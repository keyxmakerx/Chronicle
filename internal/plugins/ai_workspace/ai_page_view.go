// ai_page_view.go holds what the Your AI page's two tabs show: the levels on
// Settings and what each covers, and the rows of What it did.

package ai_workspace

import (
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
)

// aiLevel is one step on "How far may your AI go?": a pair of what it sees
// and whether it asks, named for what the owner gets.
type aiLevel struct {
	ID, Name, Short, Tag string
	// Sees and Change are the connect form's values for this level.
	Sees, Change string
	Covers       []coverRow
}

// coverRow is one line of what a level covers. Never marks the closing line
// of what no level allows.
type coverRow struct {
	Icon, Name, Value string
	Never             bool
}

// aiLevels is the three levels, safest first. Every pair of sees and change
// except "players and ask" is a level; that one is only reachable as Custom.
func aiLevels() []aiLevel {
	never := coverRow{Icon: "fa-xmark", Name: "Never", Value: "Changes anything without your yes, or reads a note someone keeps private", Never: true}
	players := []coverRow{
		{"fa-file-lines", "Pages", "Pages players can read, without your DM-only pages and secrets", false},
		{"fa-book-open", "House rules, calendar, maps and shops", "As players see them", false},
		{"fa-note-sticky", "Notes", "None. Notes are left out at this level", false},
	}
	owner := []coverRow{
		{"fa-file-lines", "Pages", "Every page, your DM-only pages and secrets included", false},
		{"fa-book-open", "House rules, calendar, maps and shops", "All of it, as you see it", false},
		{"fa-note-sticky", "Notes", "Your own notes, and ones shared with you", false},
	}
	look := coverRow{Icon: "fa-wand-magic-sparkles", Name: "Changes", Value: "None. It never asks to change anything"}
	ask := coverRow{Icon: "fa-wand-magic-sparkles", Name: "Changes", Value: "Asks for changes to pages, notes and the calendar. Each waits in the AI panel for your yes"}
	with := func(rows []coverRow, more ...coverRow) []coverRow {
		return append(append(append([]coverRow{}, rows...), more...), never)
	}
	return []aiLevel{
		{ID: "players", Name: "Look at what players see", Short: "Reads only what your players can read, and answers.", Tag: "Safest",
			Sees: "players", Change: "look", Covers: with(players, look)},
		{ID: "everything", Name: "Look at everything", Short: "Reads your DM-only pages and secrets too, and answers.",
			Sees: "owner", Change: "look", Covers: with(owner, look)},
		{ID: "ask", Name: "Look and ask for changes", Short: "Reads everything and asks for changes. Each waits for your yes.", Tag: "Most useful",
			Sees: "owner", Change: "ask", Covers: with(owner, ask)},
	}
}

// levelOf is the level a connection is at, or "custom" for the one pair no
// level names. With no connection it is the level the connect form starts on.
func levelOf(conn *connector.Connection) string {
	if conn == nil {
		return "players"
	}
	owner := conn.Privacy != aiexport.PrivacyModeSafe
	switch {
	case !owner && !conn.CanAsk:
		return "players"
	case owner && !conn.CanAsk:
		return "everything"
	case owner && conn.CanAsk:
		return "ask"
	}
	return "custom"
}

// levelState is the Alpine state of the level picker: the level now, what
// it sees now (so moving to "what you see" asks first), and which level's
// "What it covers" is open. live pickers save each pick at once; the
// connect form's picker only fills the form.
func levelState(v ConnectView, live bool) string {
	lv := levelOf(v.Conn)
	return fmt.Sprintf("{ lv: '%s', sees: '%s', change: '%s', wasOwner: %t, live: %t, pending: '', sure: false, peek: '', custom: %t, ", lv, seesValue(v.Conn), changeValue(v.Conn), v.Conn != nil && v.Conn.Privacy != aiexport.PrivacyModeSafe, live, lv == "custom") + levelMethods
}

// levelMethods are the picker's moves. Picking a level that sees more than
// the AI does now waits on a confirm, which also sends widen=yes, as the
// handler requires.
const levelMethods = `lvOf(s, c) { return s === 'players' ? (c === 'ask' ? 'custom' : 'players') : (c === 'ask' ? 'ask' : 'everything') },
	pick(id, s, c) {
		if (this.sees === s && this.change === c) return
		if (this.live && s === 'owner' && !this.wasOwner) { this.pending = [id, s, c]; return }
		this.set(id, s, c)
	},
	set(id, s, c) {
		this.lv = id; this.sees = s; this.change = c; this.pending = ''
		if (this.live) this.$nextTick(() => this.$refs.lvform.requestSubmit())
	},
	confirm() { const [id, s, c] = this.pending; this.sure = true; this.set(id, s, c) },
	keep() { this.pending = ''; this.$refs.custom.querySelectorAll('input').forEach((i) => { i.checked = i.value === this.sees || i.value === this.change }) } }`

// callRow is one call on What it did, with what the filters read.
type callRow struct {
	connector.Call
	// Kind is "ask" for a request for changes, else "look".
	Kind    string
	Icon    string
	Waiting bool
	// Left is where its tick sits on the sitting's strip, 0 to 100.
	Left float64
}

// toolAskName is the connector's request-for-changes tool, as calls log it.
const toolAskName = "ask_for_changes"

// sittingRows is a sitting's calls as rows, newest first.
func sittingRows(v ConnectView, s connector.Sitting) []callRow {
	span := s.End.Sub(s.Start)
	out := make([]callRow, 0, len(s.Calls))
	for _, c := range s.Calls {
		r := callRow{Call: c, Kind: "look", Icon: callIcon(c), Left: 50}
		if span > 0 {
			r.Left = float64(c.At.Sub(s.Start)) / float64(span) * 100
		}
		if c.Tool == toolAskName {
			r.Kind = "ask"
			r.Waiting = c.OK && callWaiting(v, c)
		}
		out = append(out, r)
	}
	return out
}

// callWaiting reports whether a request call is one still waiting for the
// owner: a waiting request sent within a minute of it, whose own line the
// call quotes.
func callWaiting(v ConnectView, c connector.Call) bool {
	for _, q := range v.Requests {
		if d := q.CreatedAt.Sub(c.At); d < -time.Minute || d > time.Minute {
			continue
		}
		if q.Summary == "" || strings.Contains(c.Summary, q.Summary) {
			return true
		}
	}
	return false
}

// callIcon is a call's icon: what kind of thing it read, a request, or the
// mark of a call that wasn't answered.
func callIcon(c connector.Call) string {
	if !c.OK {
		return "fa-circle-info"
	}
	switch c.Tool {
	case toolAskName:
		return "fa-wand-magic-sparkles"
	case "read_page":
		return "fa-file-lines"
	case "list_pages", "list_tags", "my_notes", "game_nights":
		return "fa-list"
	case "house_rules", "block_format", "how_to_write_changes":
		return "fa-book-open"
	case "read_note":
		return "fa-note-sticky"
	case "connect":
		return "fa-plug"
	}
	return "fa-magnifying-glass"
}

// callText is what the search box matches: the call's line and its note.
func callText(c connector.Call) string {
	return strings.ToLower(c.Summary + " " + c.Note)
}

// historyCounts is what the filter chips count across the sittings shown.
type historyCounts struct{ All, Asks, Waiting, Failed int }

func countCalls(v ConnectView) historyCounts {
	var n historyCounts
	for _, s := range shownSittings(v) {
		for _, r := range sittingRows(v, s) {
			n.All++
			if r.Kind == "ask" {
				n.Asks++
			}
			if r.Waiting {
				n.Waiting++
			}
			if !r.OK {
				n.Failed++
			}
		}
	}
	return n
}

// sittingSpan is a sitting's times: "1:56 pm to 2:02 pm", or one time.
func sittingSpan(v ConnectView, s connector.Sitting) string {
	from, to := clockText(v, s.Start), clockText(v, s.End)
	if from == to {
		return from
	}
	return from + " to " + to
}

// stripStyle places a tick on a sitting's strip, kept 3px in from its
// rounded ends so the first and last ticks show whole.
func stripStyle(left float64) string {
	return fmt.Sprintf("left: calc(3px + (100%% - 8px) * %.3f)", left/100)
}

// waitingIn and failedIn count a sitting's calls still waiting for the
// owner and calls that weren't answered.
func waitingIn(rows []callRow) int {
	n := 0
	for _, r := range rows {
		if r.Waiting {
			n++
		}
	}
	return n
}

func failedIn(rows []callRow) int {
	n := 0
	for _, r := range rows {
		if !r.OK {
			n++
		}
	}
	return n
}
