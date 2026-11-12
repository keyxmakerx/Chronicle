// hero_log.go is the hero's log under the pack: one list, newest first and
// grouped by game night, of what happened to the hero and what their player
// wrote. The facts (an item gained or given, something crafted) are read from
// the move history, never copied; only the written lines are stored.
package armory

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// logFactLimit is how many recent moves the log reads, and logNoteLimit
	// how many written lines.
	logFactLimit = 60
	logNoteLimit = 60
	// maxLogNote is the longest line a player can write.
	maxLogNote = 500
)

// Night is a game night as the log groups by it. Date is YYYY-MM-DD.
type Night struct {
	ID, Name, Date string
}

// NightLister names a campaign's game nights. It keeps the armory from
// importing the sessions plugin.
type NightLister interface {
	Nights(ctx context.Context, campaignID string) ([]Night, error)
}

// LogNote is one written line.
type LogNote struct {
	ID           int64
	CharacterID  string
	AuthorID     string
	Body         string
	ItemID       string
	DirectorOnly bool
	CreatedAt    time.Time
}

// HeroLogStore keeps the written lines.
type HeroLogStore interface {
	List(ctx context.Context, campaignID, characterID string, limit int) ([]LogNote, error)
	Add(ctx context.Context, n LogNote, campaignID string) error
	// Delete removes a line of this character; it reports whether one went.
	Delete(ctx context.Context, campaignID, characterID string, id int64) (LogNote, bool, error)
}

// LogNoteInput is a line to write. ItemID, when set, must be something the
// hero carries.
type LogNoteInput struct {
	Body         string
	ItemID       string
	DirectorOnly bool
}

// LogEntry is one line of the log as drawn. A fact is a sentence with the
// item's name set apart (Before, Bold, After); a note carries its words in
// Before alone.
type LogEntry struct {
	Note         bool
	NoteID       int64
	Before       string
	Bold         string
	After        string
	About        string
	DirectorOnly bool
	CanDelete    bool
	At           time.Time
}

// LogNight is a run of entries under one game night.
type LogNight struct {
	// Name is "" for lines no game night had started for yet.
	Name    string
	Tonight bool
	Entries []LogEntry
}

// splitBold sets the item's name apart inside its sentence.
func splitBold(sentence, name string) (before, bold, after string) {
	if name == "" {
		return sentence, "", ""
	}
	i := strings.Index(sentence, name)
	if i < 0 {
		return sentence, "", ""
	}
	return sentence[:i], name, sentence[i+len(name):]
}

// groupLog puts entries (newest first) under the latest game night that had
// begun on or before each one. A line written between nights belongs to the
// night before it; one from before the first night, or with no nights at all,
// has no night. Groups run newest night first.
func groupLog(entries []LogEntry, nights []Night, today string) []LogNight {
	sorted := append([]Night(nil), nights...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	idx := func(day string) int {
		at := -1
		for i, n := range sorted {
			if n.Date <= day {
				at = i
			}
		}
		return at
	}
	groups := map[int]*LogNight{}
	var order []int
	for _, e := range entries {
		k := idx(e.At.UTC().Format("2006-01-02"))
		g, ok := groups[k]
		if !ok {
			g = &LogNight{}
			if k >= 0 {
				g.Name, g.Tonight = sorted[k].Name, sorted[k].Date == today
			}
			groups[k] = g
			order = append(order, k)
		}
		g.Entries = append(g.Entries, e)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] > order[j] })
	out := make([]LogNight, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out
}

// SetHeroLog turns the log on for character panels. nights may be nil, in
// which case every line is ungrouped.
func (s *stashService) SetHeroLog(store HeroLogStore, nights NightLister) {
	s.notes, s.nights = store, nights
}

// heroLog builds the log for a hero the viewer may act for.
func (s *stashService) heroLog(ctx context.Context, campaignID string, a Actor, ref *EntityRef) ([]LogNight, error) {
	if s.notes == nil {
		return nil, nil
	}
	ep := Endpoint{Kind: EndpointCharacter, ID: ref.ID}
	moves, err := s.Repo.ListMoves(ctx, campaignID, MoveFilter{Endpoint: &ep, Status: MoveApplied, Limit: logFactLimit})
	if err != nil {
		return nil, err
	}
	items := moves[:0:0]
	for _, m := range moves {
		if m.Kind == MoveKindItem {
			items = append(items, m)
		}
	}
	lines, err := s.lines(ctx, campaignID, a, items)
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	for _, l := range lines {
		before, bold, after := splitBold(MoveSummary(l), l.ItemName)
		entries = append(entries, LogEntry{Before: before, Bold: bold, After: after, At: l.CreatedAt})
	}

	notes, err := s.notes.List(ctx, campaignID, ref.ID, logNoteLimit)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		if n.DirectorOnly && !a.SeesDmOnly() {
			continue
		}
		e := LogEntry{Note: true, NoteID: n.ID, Before: n.Body, DirectorOnly: n.DirectorOnly, At: n.CreatedAt,
			CanDelete: a.IsOwner() || n.AuthorID == a.UserID}
		if n.ItemID != "" {
			if it, err := s.Directory.GetEntity(ctx, campaignID, n.ItemID); err == nil && it != nil {
				e.About = it.Name
			}
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At.After(entries[j].At) })

	var nights []Night
	if s.nights != nil {
		// Nights only group lines; the log still reads without them.
		nights, _ = s.nights.Nights(ctx, campaignID)
	}
	return groupLog(entries, nights, time.Now().UTC().Format("2006-01-02")), nil
}

// AddLogNote writes a line in a hero's log for a viewer who may act for the
// hero. Only a viewer who sees DM-only content may mark a line Director only.
func (s *stashService) AddLogNote(ctx context.Context, campaignID string, a Actor, characterID string, in LogNoteInput) error {
	if s.notes == nil {
		return apperror.NewBadRequest("The log is not available right now.")
	}
	ref, err := s.loadCharacter(ctx, campaignID, characterID)
	if err != nil {
		return err
	}
	ok, err := s.canAct(ctx, campaignID, a, ref.ID)
	if err != nil {
		return err
	}
	if !ok {
		return forbidden()
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return apperror.NewBadRequest("Write a line first.")
	}
	if utf8.RuneCountInString(body) > maxLogNote {
		return apperror.NewBadRequest("Keep a line under 500 characters.")
	}
	if in.DirectorOnly && !a.SeesDmOnly() {
		return forbidden()
	}
	if in.ItemID != "" {
		rel, err := s.hasItem(ctx, campaignID, ref.ID, in.ItemID, a.SeesDmOnly())
		if err != nil {
			return err
		}
		if rel == nil {
			return apperror.NewBadRequest("The hero doesn't carry that item.")
		}
	}
	return s.notes.Add(ctx, LogNote{CharacterID: ref.ID, AuthorID: a.UserID, Body: body, ItemID: in.ItemID, DirectorOnly: in.DirectorOnly}, campaignID)
}

// DeleteLogNote removes a line: its author, or the Owner. A line the viewer
// may not see is reported as missing.
func (s *stashService) DeleteLogNote(ctx context.Context, campaignID string, a Actor, characterID string, id int64) error {
	if s.notes == nil {
		return notFound("note")
	}
	ref, err := s.loadCharacter(ctx, campaignID, characterID)
	if err != nil {
		return err
	}
	ok, err := s.canAct(ctx, campaignID, a, ref.ID)
	if err != nil {
		return err
	}
	if !ok {
		return notFound("note")
	}
	notes, err := s.notes.List(ctx, campaignID, ref.ID, 500)
	if err != nil {
		return err
	}
	for _, n := range notes {
		if n.ID != id {
			continue
		}
		if (n.DirectorOnly && !a.SeesDmOnly()) || (!a.IsOwner() && n.AuthorID != a.UserID) {
			return notFound("note")
		}
		_, _, err := s.notes.Delete(ctx, campaignID, ref.ID, id)
		return err
	}
	return notFound("note")
}

type heroLogRepository struct{ db *sql.DB }

// NewHeroLogRepository returns the SQL store for written log lines.
func NewHeroLogRepository(db *sql.DB) HeroLogStore { return &heroLogRepository{db: db} }

func (r *heroLogRepository) List(ctx context.Context, campaignID, characterID string, limit int) ([]LogNote, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, author_id, body, COALESCE(item_entity_id, ''), director_only, created_at
		FROM armory_hero_notes WHERE campaign_id = ? AND character_id = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, campaignID, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogNote
	for rows.Next() {
		n := LogNote{CharacterID: characterID}
		if err := rows.Scan(&n.ID, &n.AuthorID, &n.Body, &n.ItemID, &n.DirectorOnly, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *heroLogRepository) Add(ctx context.Context, n LogNote, campaignID string) error {
	var item any
	if n.ItemID != "" {
		item = n.ItemID
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO armory_hero_notes
		(campaign_id, character_id, author_id, body, item_entity_id, director_only) VALUES (?, ?, ?, ?, ?, ?)`,
		campaignID, n.CharacterID, n.AuthorID, n.Body, item, n.DirectorOnly)
	return err
}

func (r *heroLogRepository) Delete(ctx context.Context, campaignID, characterID string, id int64) (LogNote, bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM armory_hero_notes WHERE id = ? AND campaign_id = ? AND character_id = ?`, id, campaignID, characterID)
	if err != nil {
		return LogNote{}, false, err
	}
	n, _ := res.RowsAffected()
	return LogNote{ID: id}, n > 0, nil
}
