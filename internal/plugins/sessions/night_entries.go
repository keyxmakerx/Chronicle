package sessions

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Each game night gives every member a Journal entry of their own, to write
// in during play. The entries are ordinary Journal notes, kept private until
// their writer shares them; session_night_entries says which night each
// belongs to. After the night, the entries the DM can read are read for the
// pages they name, and the night's page asks the DM about each one.

// NightNote is one game-night entry as the night's page needs it. The notes
// widget fills it through NightJournal; PageIDs and Lines come from the page
// links in its body.
type NightNote struct {
	ID        string
	Title     string
	AuthorID  string
	UpdatedAt time.Time
	// Audience is the note's visibility: private, gm, party or custom.
	Audience string
	// PageIDs are the pages the note links, in order.
	PageIDs []string
	// Lines holds, per linked page, the line of the note that names it.
	Lines map[string]string
}

// NightJournal is the notes side of game-night entries, implemented by an
// adapter over the notes widget so every read applies the notes' own
// visibility rule.
type NightJournal interface {
	// CreateEntry makes v's entry with this id and title, in their Game
	// nights folder. An entry that already exists is not an error.
	CreateEntry(ctx context.Context, campaignID string, v permissions.Viewer, id, title string) error
	// ReadableEntries returns the notes among ids that v can read, in the
	// order given; the rest are left out.
	ReadableEntries(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) ([]NightNote, error)
}

// NightPageNamer names the pages a viewer may see; any other id is absent
// from the result.
type NightPageNamer interface {
	PageNames(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) (map[string]string, error)
}

// nightSessionLister is the one session read entries need: the planned
// sessions with a night in [from, to].
type nightSessionLister interface {
	ListByDateRange(ctx context.Context, campaignID, startDate, endDate string) ([]Session, error)
}

// NightNotes is what a night's page shows from the entries: the ones the
// viewer can read, and for the DM the after-night check-in.
type NightNotes struct {
	Entries []NightEntryView `json:"entries"`
	// CheckIn is set only for a viewer who sees DM-only content, once the
	// night has started.
	CheckIn []CheckInPerson `json:"checkIn,omitempty"`
}

// NightEntryView is one entry in "Entries for this night".
type NightEntryView struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	AuthorID string `json:"authorId"`
	// AuthorName is filled by the handler from the member list.
	AuthorName string `json:"authorName,omitempty"`
	Audience   string `json:"audience"`
}

// CheckInPerson is one page the check-in asks about. NoteID is the first
// entry naming it, with MoreNotes counting the others; a page only linked to
// the night as mentioned has no entry. Answer is "" until the DM answers.
type CheckInPerson struct {
	EntityID  string `json:"entityId"`
	Name      string `json:"name"`
	Answer    string `json:"answer,omitempty"`
	NoteID    string `json:"noteId,omitempty"`
	NoteTitle string `json:"noteTitle,omitempty"`
	MoreNotes int    `json:"moreNotes,omitempty"`
	Line      string `json:"line,omitempty"`
}

// NightTag is the night a Journal entry belongs to, for the Journal's tag.
type NightTag struct {
	SessionID string
	Date      string
	Name      string
	Time      string
	// Started is whether the night has begun, so the entry is being read.
	Started bool
}

// NightEntryService gives members their game-night entries and reads them.
type NightEntryService interface {
	// ForNight returns the entries v can read for one night of sess (date is
	// one of its occurrences) and, for the DM, the check-in. sess must carry
	// its linked pages. The viewer's own entry is made first when it is due.
	ForNight(ctx context.Context, sess *Session, date string, v permissions.Viewer, now time.Time) (*NightNotes, error)
	// EnsureEntries makes v's entries for every night that is due and has
	// none yet. An entry its writer deleted is not made again.
	EnsureEntries(ctx context.Context, campaignID string, v permissions.Viewer, now time.Time) error
	// Tags returns, for those of noteIDs that are game-night entries in the
	// campaign, the night each belongs to.
	Tags(ctx context.Context, campaignID string, noteIDs []string, now time.Time) (map[string]NightTag, error)
}

type nightEntryService struct {
	sessions  nightSessionLister
	entries   NightEntryRepository
	journal   NightJournal
	pages     NightPageNamer
	sightings SightingRepository
}

// NewNightEntryService creates the service. All five are required.
func NewNightEntryService(sessions nightSessionLister, entries NightEntryRepository, journal NightJournal, pages NightPageNamer, sightings SightingRepository) NightEntryService {
	return &nightEntryService{sessions: sessions, entries: entries, journal: journal, pages: pages, sightings: sightings}
}

// When an entry is due: a one-off night's as soon as it is planned (up to
// entryLookAheadDays ahead), each night of a series the day before. Nights
// up to entryLookBackDays past still get one, for whoever opens the Journal
// only after playing.
const (
	entryLookBackDays  = 7
	entryLookAheadDays = 60
	// maxEntriesPerEnsure bounds the notes one Journal load can make.
	maxEntriesPerEnsure = 20
)

// nightEntryNamespace seeds entry ids, so an entry is found again from its
// night and writer and two loads at once collide on the key instead of
// making two.
var nightEntryNamespace = uuid.MustParse("2bbef4ab-010d-4861-9f3f-2b8cbce9193f")

func nightEntryID(campaignID, sessionID, date, userID string) string {
	return uuid.NewSHA1(nightEntryNamespace, []byte(campaignID+"\x00"+sessionID+"\x00"+date+"\x00"+userID)).String()
}

// maxEntryNameChars keeps the title inside the notes' 200-character limit
// with room for the date.
const maxEntryNameChars = 160

// nightEntryTitle is "Game night 14, Sat 10 Oct": the date tells a series'
// nights apart.
func nightEntryTitle(name, date string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Game night"
	}
	if utf8.RuneCountInString(name) > maxEntryNameChars {
		name = string([]rune(name)[:maxEntryNameChars])
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return name
	}
	return name + ", " + d.Format("Mon 2 Jan")
}

// nightEntryDue reports whether date's entry should exist by today.
func nightEntryDue(sess Session, date string, today time.Time) bool {
	day := func(n int) string { return today.UTC().AddDate(0, 0, n).Format("2006-01-02") }
	if date < day(-entryLookBackDays) || date > day(entryLookAheadDays) {
		return false
	}
	return !sess.IsRecurring || date <= day(1)
}

// nightStart is when a night starts: its time on its date, in the zone it
// was scheduled in (UTC when none was saved), or the start of its day when
// it has no time.
func nightStart(date, hhmm, tz string) (time.Time, bool) {
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, false
	}
	if hhmm != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, loc); err == nil {
			return t, true
		}
	}
	return day, true
}

func (s *nightEntryService) EnsureEntries(ctx context.Context, campaignID string, v permissions.Viewer, now time.Time) error {
	uid := v.UserID()
	if !getsEntries(v) {
		return nil
	}
	today := now.UTC()
	from := today.AddDate(0, 0, -entryLookBackDays).Format("2006-01-02")
	to := today.AddDate(0, 0, entryLookAheadDays).Format("2006-01-02")
	list, err := s.sessions.ListByDateRange(ctx, campaignID, from, to)
	if err != nil {
		return err
	}
	type night struct {
		sess Session
		date string
	}
	var due []night
	var ids []string
	for _, sess := range list {
		n := len(due)
		for _, d := range occurrenceDates(sess, from, to) {
			if nightEntryDue(sess, d, today) {
				due = append(due, night{sess, d})
			}
		}
		if len(due) > n {
			ids = append(ids, sess.ID)
		}
	}
	if len(due) == 0 {
		return nil
	}
	rows, err := s.entries.ListUserNightEntries(ctx, uid, ids)
	if err != nil {
		return err
	}
	have := make(map[string]bool, len(rows))
	for _, r := range rows {
		have[r.SessionID+"|"+r.Date] = true
	}
	made := 0
	for _, n := range due {
		if have[n.sess.ID+"|"+n.date] {
			continue
		}
		if made == maxEntriesPerEnsure {
			break
		}
		if err := s.makeEntry(ctx, campaignID, v, n.sess, n.date); err != nil {
			return err
		}
		made++
	}
	return nil
}

// getsEntries reports whether v is a member, who has a Journal; a site admin
// looking in without joining does not.
func getsEntries(v permissions.Viewer) bool {
	return v.UserID() != "" && v.Role() >= permissions.RolePlayer
}

// makeEntry creates the note, then records it; a note made by a load that
// lost the race to record it is the same note, since the id is fixed.
func (s *nightEntryService) makeEntry(ctx context.Context, campaignID string, v permissions.Viewer, sess Session, date string) error {
	id := nightEntryID(campaignID, sess.ID, date, v.UserID())
	if err := s.journal.CreateEntry(ctx, campaignID, v, id, nightEntryTitle(sess.Name, date)); err != nil {
		return err
	}
	return s.entries.InsertNightEntry(ctx, NightEntryRow{SessionID: sess.ID, Date: date, UserID: v.UserID(), NoteID: id})
}

func (s *nightEntryService) Tags(ctx context.Context, campaignID string, noteIDs []string, now time.Time) (map[string]NightTag, error) {
	out := map[string]NightTag{}
	if len(noteIDs) == 0 {
		return out, nil
	}
	rows, err := s.entries.ListNightEntriesByNote(ctx, campaignID, noteIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		start, ok := nightStart(r.Date, r.Time, r.TZ)
		out[r.NoteID] = NightTag{SessionID: r.SessionID, Date: r.Date, Name: r.Name, Time: r.Time, Started: ok && !now.Before(start)}
	}
	return out, nil
}

func (s *nightEntryService) ForNight(ctx context.Context, sess *Session, date string, v permissions.Viewer, now time.Time) (*NightNotes, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, apperror.NewBadRequest("date must be YYYY-MM-DD")
	}
	if len(occurrenceDates(*sess, date, date)) == 0 {
		return nil, apperror.NewBadRequest("that date is not one of this game night's dates")
	}
	rows, err := s.entries.ListNightEntries(ctx, sess.ID, date)
	if err != nil {
		return nil, err
	}
	// Only a planned night makes entries, as on the Journal's side.
	if uid := v.UserID(); getsEntries(v) && sess.Status == StatusPlanned && nightEntryDue(*sess, date, now) {
		mine := false
		for _, r := range rows {
			mine = mine || r.UserID == uid
		}
		if !mine {
			// The page still shows without it; the next load tries again.
			if err := s.makeEntry(ctx, sess.CampaignID, v, *sess, date); err != nil {
				slog.Warn("making a game-night entry failed", slog.String("session_id", sess.ID), slog.Any("error", err))
			} else {
				rows = append(rows, NightEntryRow{SessionID: sess.ID, Date: date, UserID: uid, NoteID: nightEntryID(sess.CampaignID, sess.ID, date, uid)})
			}
		}
	}
	ids := make([]string, 0, len(rows))
	owner := make(map[string]string, len(rows))
	for _, r := range rows {
		ids = append(ids, r.NoteID)
		owner[r.NoteID] = r.UserID
	}
	var list []NightNote
	if len(ids) > 0 {
		if list, err = s.journal.ReadableEntries(ctx, sess.CampaignID, v, ids); err != nil {
			return nil, err
		}
	}
	// A note is the night's entry only while its writer still owns it.
	kept := list[:0]
	for _, n := range list {
		if n.AuthorID == owner[n.ID] {
			kept = append(kept, n)
		}
	}
	list = kept
	uid := v.UserID()
	sort.SliceStable(list, func(i, j int) bool { return list[i].AuthorID == uid && list[j].AuthorID != uid })
	out := &NightNotes{Entries: make([]NightEntryView, 0, len(list))}
	for _, n := range list {
		out.Entries = append(out.Entries, NightEntryView{ID: n.ID, Title: n.Title, AuthorID: n.AuthorID, Audience: n.Audience})
	}
	start, _ := nightStart(date, strPtrVal(sess.ScheduledTime), strPtrVal(sess.ScheduledTZ))
	if !seesDM(v) || now.Before(start) {
		return out, nil
	}
	out.CheckIn, err = s.checkIn(ctx, sess, date, v, list)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// checkIn lists the pages named in the night's entries or linked to the
// night as mentioned, each with the DM's answer if there is one. Pages
// linked as encountered or key are already in Last seen, so they are not
// asked about.
func (s *nightEntryService) checkIn(ctx context.Context, sess *Session, date string, v permissions.Viewer, list []NightNote) ([]CheckInPerson, error) {
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
			p.NoteID, p.NoteTitle, p.Line = n.ID, n.Title, n.Lines[id]
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
