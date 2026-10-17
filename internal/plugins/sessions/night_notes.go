package sessions

import (
	"context"
	"sort"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// NightNote is one Journal note as a game night's page needs it. The notes
// widget fills it through NightNotesReader; PageIDs and Lines come from the
// page links in its body.
type NightNote struct {
	ID        string
	Title     string
	AuthorID  string
	CreatedAt time.Time
	// Audience is the note's visibility: private, gm, party or custom.
	Audience string
	// PageIDs are the pages the note links, in order.
	PageIDs []string
	// Lines holds, per linked page, the line of the note that names it.
	Lines map[string]string
}

// NightNotesReader lists the Journal notes a viewer can read that were
// written in [from, to), oldest first. Implemented by an adapter over the
// notes widget, so the read applies the notes' own visibility rule.
type NightNotesReader interface {
	NotesWrittenBetween(ctx context.Context, campaignID string, v permissions.Viewer, from, to time.Time) (notes []NightNote, truncated bool, err error)
}

// NightPageNamer names the pages a viewer may see; any other id is absent
// from the result.
type NightPageNamer interface {
	PageNames(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) (map[string]string, error)
}

// NightNotes is what a past night's page shows from the notes: the notes
// written that night, and for the DM the after-night check-in.
type NightNotes struct {
	Notes     []NightNoteRow `json:"notes"`
	Truncated bool           `json:"truncated,omitempty"`
	// CheckIn is set only for a viewer who sees DM-only content, once the
	// night has started.
	CheckIn []CheckInPerson `json:"checkIn,omitempty"`
}

// NightNoteRow is one note in "Written that night".
type NightNoteRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	AuthorID string `json:"authorId"`
	// AuthorName is filled by the handler from the member list.
	AuthorName string    `json:"authorName,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	Audience   string    `json:"audience"`
}

// CheckInPerson is one page the check-in asks about. NoteID is the first
// note that night naming it, with MoreNotes counting the others; a page
// only linked to the night as mentioned has no note. Answer is "" until the
// DM answers.
type CheckInPerson struct {
	EntityID  string     `json:"entityId"`
	Name      string     `json:"name"`
	Answer    string     `json:"answer,omitempty"`
	NoteID    string     `json:"noteId,omitempty"`
	NoteTitle string     `json:"noteTitle,omitempty"`
	NoteAt    *time.Time `json:"noteAt,omitempty"`
	MoreNotes int        `json:"moreNotes,omitempty"`
	Line      string     `json:"line,omitempty"`
}

// NightNotesService files notes under the night they were written in and
// builds the check-in from them.
type NightNotesService interface {
	// ForNight returns the notes v can read that were written during one
	// night of sess (date is one of its occurrences) and, for the DM, the
	// check-in. sess must carry its linked pages.
	ForNight(ctx context.Context, sess *Session, date string, v permissions.Viewer, now time.Time) (*NightNotes, error)
}

type nightNotesService struct {
	notes     NightNotesReader
	pages     NightPageNamer
	sightings SightingRepository
}

// NewNightNotesService creates the service. All three are required.
func NewNightNotesService(notes NightNotesReader, pages NightPageNamer, sightings SightingRepository) NightNotesService {
	return &nightNotesService{notes: notes, pages: pages, sightings: sightings}
}

// The window a night's notes are filed by: from an hour before the start
// (prep and arrivals) until 6 the next morning, on the night's own clock.
const (
	nightLeadIn  = time.Hour
	nightEndHour = 6
)

// nightWindow returns when a night starts and the window its notes are
// filed by. A night with no time starts at the beginning of its day. The
// zone is the one the night was scheduled in, or UTC when none was saved.
func nightWindow(date, hhmm, tz string) (start, from, to time.Time, ok bool) {
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, false
	}
	start, from = day, day
	if hhmm != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, loc); err == nil {
			start, from = t, t.Add(-nightLeadIn)
		}
	}
	to = time.Date(day.Year(), day.Month(), day.Day()+1, nightEndHour, 0, 0, 0, loc)
	return start, from, to, true
}

func (s *nightNotesService) ForNight(ctx context.Context, sess *Session, date string, v permissions.Viewer, now time.Time) (*NightNotes, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, apperror.NewBadRequest("date must be YYYY-MM-DD")
	}
	if len(occurrenceDates(*sess, date, date)) == 0 {
		return nil, apperror.NewBadRequest("that date is not one of this game night's dates")
	}
	start, from, to, _ := nightWindow(date, strPtrVal(sess.ScheduledTime), strPtrVal(sess.ScheduledTZ))
	list, truncated, err := s.notes.NotesWrittenBetween(ctx, sess.CampaignID, v, from, to)
	if err != nil {
		return nil, err
	}
	out := &NightNotes{Notes: make([]NightNoteRow, 0, len(list)), Truncated: truncated}
	for _, n := range list {
		out.Notes = append(out.Notes, NightNoteRow{ID: n.ID, Title: n.Title, AuthorID: n.AuthorID, CreatedAt: n.CreatedAt, Audience: n.Audience})
	}
	if !seesDM(v) || now.Before(start) {
		return out, nil
	}
	out.CheckIn, err = s.checkIn(ctx, sess, date, v, list)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// checkIn lists the pages named in the night's notes or linked to the night
// as mentioned, each with the DM's answer if there is one. Pages linked as
// encountered or key are already in Last seen, so they are not asked about.
func (s *nightNotesService) checkIn(ctx context.Context, sess *Session, date string, v permissions.Viewer, list []NightNote) ([]CheckInPerson, error) {
	settled := map[string]bool{}
	for _, e := range sess.Entities {
		if e.Role == EntityRoleEncountered || e.Role == EntityRoleKey {
			settled[e.EntityID] = true
		}
	}
	var order []string
	people := map[string]*CheckInPerson{}
	add := func(id string) *CheckInPerson {
		if settled[id] {
			return nil
		}
		if p := people[id]; p != nil {
			return p
		}
		p := &CheckInPerson{EntityID: id}
		people[id] = p
		order = append(order, id)
		return p
	}
	for _, n := range list {
		for _, id := range n.PageIDs {
			p := add(id)
			if p == nil {
				continue
			}
			if p.NoteID != "" {
				p.MoreNotes++
				continue
			}
			at := n.CreatedAt
			p.NoteID, p.NoteTitle, p.NoteAt, p.Line = n.ID, n.Title, &at, n.Lines[id]
		}
	}
	for _, e := range sess.Entities {
		if e.Role == EntityRoleMentioned {
			add(e.EntityID)
		}
	}
	answers, err := s.sightings.ListSightingsForNight(ctx, sess.ID, date)
	if err != nil {
		return nil, err
	}
	// A page answered by hand stays listed with its answer, so the DM can
	// change it here.
	answered := make([]string, 0, len(answers))
	for id := range answers {
		answered = append(answered, id)
	}
	sort.Strings(answered)
	for _, id := range answered {
		if p := add(id); p != nil {
			p.Answer = answers[id]
		}
	}
	if len(order) == 0 {
		return nil, nil
	}
	names, err := s.pages.PageNames(ctx, sess.CampaignID, v, order)
	if err != nil {
		return nil, err
	}
	out := make([]CheckInPerson, 0, len(order))
	for _, id := range order {
		name, ok := names[id]
		if !ok {
			continue
		}
		p := people[id]
		p.Name = name
		out = append(out, *p)
	}
	return out, nil
}
