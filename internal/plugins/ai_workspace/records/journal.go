package records

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// JournalAPI is the Journal, Jot notes and Player Notes as the AI reads
// them, adapted in app wiring over the two notes widgets so each one's own
// visibility rule decides every row: the viewer's own notes, and the ones
// other members shared with them. A note someone keeps private is never
// listed, whoever asks.
type JournalAPI interface {
	JournalNotes(ctx context.Context, campaignID string, v permissions.Viewer) ([]JournalNote, error)
}

// JournalNote is one Journal note, jot or Player Note the viewer can read.
type JournalNote struct {
	ID       string
	Title    string
	AuthorID string
	Author   string // the writer's display name
	PageID   string // set on a jot or a Player Note: the page it sits on
	Folder   string // the folder path in the writer's Journal, "" at the top
	// PlayerNote marks a note from the Player Notes on a page, which only
	// its writer can change.
	PlayerNote bool
	// Audience is who the writer shared it with: private, gm, party or
	// custom, or for a Player Note dm_only, dm_scribe or everyone.
	Audience string
	// Night names the game night this is an entry for, "" when it is not one.
	Night    string
	Archived bool
	Updated  time.Time
	Body     *string // sanitized HTML
}

var audienceWords = map[string]string{
	string(notes.VisibilityPrivate): "private",
	string(notes.VisibilityGM):      "shared with the DM",
	string(notes.VisibilityParty):   "shared with everyone",
	string(notes.VisibilityCustom):  "shared with some people",
	"dm_only":                       "shared with the DM",
	"dm_scribe":                     "shared with the DM and Scribes",
	"everyone":                      "shared with everyone",
}

// journal reads the Actor's Journal once per paste. Notes are the Actor's
// own writing kept from players, so a player's view (Safe) never reads them.
func (run *lookupRun) journal() ([]JournalNote, error) {
	if run.l.Journal == nil {
		return nil, badRequestf("notes can't be looked up on this server")
	}
	if !run.director() {
		return nil, badRequestf("your notes are left out while your AI sees what players see; choose \"What you see\" to include them")
	}
	if !run.journalRead {
		run.journalRead = true
		run.journalNotes, run.journalErr = run.l.Journal.JournalNotes(run.ctx, run.campaignID, run.a.Viewer())
	}
	return run.journalNotes, run.journalErr
}

// canChange reports whether a note another member wrote may be changed by a
// `note` block (title and text), with its writer. Player Notes never may.
func (run *lookupRun) canChange(n JournalNote) (NoteWriter, bool) {
	if n.PlayerNote || n.AuthorID == run.a.UserID || run.l.NoteAccess == nil {
		return NoteWriter{}, false
	}
	if run.access == nil {
		a, err := run.l.NoteAccess.NoteAccess(run.ctx, run.campaignID)
		if err != nil {
			return NoteWriter{}, false
		}
		run.access = &a
	}
	if !run.access.CoDMs && !run.access.Scribes {
		return NoteWriter{}, false
	}
	if run.editable == nil {
		run.editable = map[string]NoteWriter{}
	}
	w, ok := run.editable[n.AuthorID]
	if !ok {
		var err error
		if w, err = run.l.NoteAccess.NoteWriter(run.ctx, run.campaignID, n.AuthorID); err != nil {
			return NoteWriter{}, false
		}
		run.editable[n.AuthorID] = w
	}
	return w, run.access.Allows(w.Kind)
}

// pageName is a jot's page as the Actor sees it.
func (run *lookupRun) pageName(id string) string {
	if pages, err := run.visiblePages(); err == nil {
		for _, p := range pages {
			if p.ID == id {
				return p.Name
			}
		}
	}
	return ""
}

// noteDetails is a note's place and audience in plain words.
func (run *lookupRun) noteDetails(n JournalNote, mine bool) string {
	var parts []string
	if !mine {
		parts = append(parts, "by "+firstNonEmpty(n.Author, "another member"))
	}
	switch {
	case n.PlayerNote:
		parts = append(parts, "a Player Note on "+firstNonEmpty(run.pageName(n.PageID), "a page"))
	case n.PageID != "":
		parts = append(parts, "a jot on "+firstNonEmpty(run.pageName(n.PageID), "a page"))
	case n.Folder != "":
		parts = append(parts, "in the folder "+n.Folder)
	}
	if n.Night != "" {
		parts = append(parts, "game night entry for "+n.Night)
	}
	if w := audienceWords[n.Audience]; w != "" {
		parts = append(parts, w)
	}
	if n.Archived {
		parts = append(parts, "archived")
	}
	if !mine {
		if _, ok := run.canChange(n); ok {
			parts = append(parts, "I can change its title and text")
		}
	}
	return strings.Join(parts, "; ")
}

func (run *lookupRun) notes() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Notes"}
	all, err := run.journal()
	if err != nil {
		return ans, err
	}
	var own, jots, player, shared []string
	for _, n := range all {
		line := fmt.Sprintf("- %s (%s)", firstNonEmpty(n.Title, "Untitled"), run.noteDetails(n, n.AuthorID == run.a.UserID))
		switch {
		case n.AuthorID != run.a.UserID:
			shared = append(shared, line)
		case n.PlayerNote:
			player = append(player, line)
		case n.PageID != "":
			jots = append(jots, line)
		default:
			own = append(own, line)
		}
	}
	ans.Chip = fmt.Sprintf("Notes · %d", len(all))
	var b strings.Builder
	section := func(title string, lines []string) {
		fmt.Fprintf(&b, "## %s (%d)\n\n", title, len(lines))
		if len(lines) == 0 {
			b.WriteString("None.\n\n")
			return
		}
		writeLines(&b, lines)
		b.WriteString("\n")
	}
	section("My Journal", own)
	section("My jots on pages", jots)
	section("My Player Notes on pages", player)
	section("Shared with me by other members", shared)
	b.WriteString("Read one with `what: note` and its `name`. Notes other members wrote can be read, and changed only where marked \"I can change its title and text\". My Journal notes and jots are changed with a `note` block, my Player Notes with a `player-note` block.\n")
	ans.Text = b.String()
	return ans, nil
}

// note answers one note's text, matched by title; `page` picks a jot and
// `by` the writer when titles repeat.
func (run *lookupRun) note(r Record) (LookupAnswer, error) {
	name := firstNonEmpty(r.Name, r.Str("title"))
	ans := LookupAnswer{Chip: "Note · " + oneLine(name, 60)}
	if name == "" {
		return ans, badRequestf("give the note's `name` (its title)")
	}
	all, err := run.journal()
	if err != nil {
		return ans, err
	}
	page, by := r.Str("page"), r.Str("by")
	var hits []JournalNote
	for _, n := range all {
		if !sameName(n.Title, name) {
			continue
		}
		if page != "" && (n.PageID == "" || !sameName(run.pageName(n.PageID), page)) {
			continue
		}
		if by != "" && !sameName(n.Author, by) {
			continue
		}
		hits = append(hits, n)
	}
	switch {
	case len(hits) == 0:
		return ans, badRequestf("no note called %s that you can read", quote(name))
	case len(hits) > 1:
		// Mine first, then the newest: the one most likely meant.
		sort.SliceStable(hits, func(i, j int) bool {
			mi, mj := hits[i].AuthorID == run.a.UserID, hits[j].AuthorID == run.a.UserID
			if mi != mj {
				return mi
			}
			return hits[i].Updated.After(hits[j].Updated)
		})
	}
	n := hits[0]
	mine := n.AuthorID == run.a.UserID
	body, _ := aiexport.RenderNoteText(notes.Note{Title: n.Title, EntryHTML: n.Body})
	var b strings.Builder
	fmt.Fprintf(&b, "## Note: %s\n\n", firstNonEmpty(n.Title, "Untitled"))
	fmt.Fprintf(&b, "*%s", run.noteDetails(n, mine))
	if !n.Updated.IsZero() {
		fmt.Fprintf(&b, "; updated %s", n.Updated.UTC().Format("2006-01-02"))
	}
	b.WriteString("*\n\n")
	w, editable := run.canChange(n)
	switch {
	case !mine && editable:
		fmt.Fprintf(&b, "Written by %s (%s) and shared with me. A `note` block with `action: update` and `by: %s` can change its title and text, never who sees it.\n\n", firstNonEmpty(w.Name, "another member"), w.Kind, firstNonEmpty(w.Name, n.Author))
	case !mine:
		b.WriteString("Written by another member and shared with me: read it, but no block can change it.\n\n")
	case n.PlayerNote:
		b.WriteString("My Player Note: change it with a `player-note` block naming its page.\n\n")
	}
	if len(hits) > 1 {
		fmt.Fprintf(&b, "(%d notes have this title; this is the first. Add `page` or `by` to pick another.)\n\n", len(hits))
	}
	if strings.TrimSpace(body) == "" {
		body = "_(empty)_"
	}
	b.WriteString(body)
	b.WriteString("\n")
	ans.Text = b.String()
	return ans, nil
}
