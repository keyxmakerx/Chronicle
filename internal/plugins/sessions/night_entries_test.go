package sessions

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestNightStart(t *testing.T) {
	tests := []struct {
		name           string
		date, hhmm, tz string
		want           string // RFC 3339, UTC
	}{
		{"evening in New York", "2026-10-10", "19:00", "America/New_York", "2026-10-10T23:00:00Z"},
		{"no time starts the day", "2026-10-10", "", "Europe/London", "2026-10-09T23:00:00Z"},
		{"no zone is UTC", "2026-10-10", "20:30", "", "2026-10-10T20:30:00Z"},
		{"an unknown zone falls back to UTC", "2026-10-10", "20:00", "Mars/Olympus", "2026-10-10T20:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nightStart(tt.date, tt.hhmm, tt.tz)
			if !ok || got.UTC().Format(time.RFC3339) != tt.want {
				t.Errorf("nightStart = %s (%v), want %s", got.UTC().Format(time.RFC3339), ok, tt.want)
			}
		})
	}
	if _, ok := nightStart("10/10/2026", "", ""); ok {
		t.Error("a bad date should not be ok")
	}
}

func TestNightEntryDue(t *testing.T) {
	today := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	weekly := RecurrenceWeekly
	oneOff := Session{}
	series := Session{IsRecurring: true, RecurrenceType: &weekly}
	tests := []struct {
		name string
		sess Session
		date string
		want bool
	}{
		{"a one-off night as soon as it is planned", oneOff, "2026-11-20", true},
		{"a one-off night too far ahead", oneOff, "2026-12-10", false},
		{"a series night the day before", series, "2026-10-11", true},
		{"a series night two days ahead", series, "2026-10-12", false},
		{"a series night today", series, "2026-10-10", true},
		{"a week back", series, "2026-10-03", true},
		{"more than a week back", oneOff, "2026-10-02", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nightEntryDue(tt.sess, tt.date, today); got != tt.want {
				t.Errorf("due = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNightEntryTitle(t *testing.T) {
	if got := nightEntryTitle("Game night 14", "2026-10-10"); got != "Game night 14, Sat 10 Oct" {
		t.Errorf("title = %q", got)
	}
	if got := nightEntryTitle("  ", "2026-10-10"); got != "Game night, Sat 10 Oct" {
		t.Errorf("unnamed title = %q", got)
	}
	long := ""
	for i := 0; i < 300; i++ {
		long += "é"
	}
	if got := nightEntryTitle(long, "2026-10-10"); len([]rune(got)) != maxEntryNameChars+len(", Sat 10 Oct") {
		t.Errorf("long title has %d characters", len([]rune(got)))
	}
}

type fakeSessionLister struct{ list []Session }

func (f *fakeSessionLister) ListByDateRange(context.Context, string, string, string) ([]Session, error) {
	return f.list, nil
}

type fakeEntryRepo struct {
	rows   []NightEntryRow
	nights []NightEntryNight
}

func (f *fakeEntryRepo) InsertNightEntry(_ context.Context, r NightEntryRow) error {
	for _, x := range f.rows {
		if x.SessionID == r.SessionID && x.Date == r.Date && x.UserID == r.UserID {
			return nil
		}
	}
	f.rows = append(f.rows, r)
	return nil
}
func (f *fakeEntryRepo) ListNightEntries(_ context.Context, sessionID, date string) ([]NightEntryRow, error) {
	var out []NightEntryRow
	for _, r := range f.rows {
		if r.SessionID == sessionID && r.Date == date {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeEntryRepo) ListUserNightEntries(_ context.Context, userID string, _ []string) ([]NightEntryRow, error) {
	var out []NightEntryRow
	for _, r := range f.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeEntryRepo) ListNightEntriesByNote(context.Context, string, []string) ([]NightEntryNight, error) {
	return f.nights, nil
}

// fakeJournal holds notes by id; readable lists who may read each.
type fakeJournal struct {
	notes    map[string]NightNote
	readable map[string][]string
	created  []string
}

func (f *fakeJournal) CreateEntry(_ context.Context, _ string, v permissions.Viewer, id, title string) error {
	if f.notes == nil {
		f.notes = map[string]NightNote{}
	}
	if _, ok := f.notes[id]; !ok {
		f.notes[id] = NightNote{ID: id, Title: title, AuthorID: v.UserID(), Audience: "private"}
		f.created = append(f.created, title)
	}
	return nil
}
func (f *fakeJournal) ReadableEntries(_ context.Context, _ string, v permissions.Viewer, ids []string) ([]NightNote, error) {
	var out []NightNote
	for _, id := range ids {
		n, ok := f.notes[id]
		if !ok {
			continue
		}
		can := n.AuthorID == v.UserID()
		for _, u := range f.readable[id] {
			can = can || u == v.UserID()
		}
		if can {
			out = append(out, n)
		}
	}
	return out, nil
}

type fakePageNamer struct{ names map[string]string }

func (f *fakePageNamer) PageNames(_ context.Context, _ string, _ permissions.Viewer, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := f.names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func TestEnsureEntries(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	weekly := RecurrenceWeekly
	start, oneOffDate := "2026-10-03", "2026-10-31"
	sessions := &fakeSessionLister{list: []Session{
		{ID: "w", CampaignID: "c1", Name: "Weekly", ScheduledDate: &start, IsRecurring: true, RecurrenceType: &weekly},
		{ID: "o", CampaignID: "c1", Name: "One-off", ScheduledDate: &oneOffDate},
	}}
	repo := &fakeEntryRepo{}
	journal := &fakeJournal{}
	svc := NewNightEntryService(sessions, repo, journal, &fakePageNamer{}, &fakeSightingRepo{})

	if err := svc.EnsureEntries(context.Background(), "c1", playerViewer, now); err != nil {
		t.Fatal(err)
	}
	// The weekly's 3 Oct (past, in the week) and 10 Oct (tomorrow); not 17 Oct.
	want := []string{"Weekly, Sat 3 Oct", "Weekly, Sat 10 Oct", "One-off, Sat 31 Oct"}
	if !equalStrings(journal.created, want) {
		t.Errorf("created = %v, want %v", journal.created, want)
	}
	if len(repo.rows) != 3 || repo.rows[1].NoteID != nightEntryID("c1", "w", "2026-10-10", "p1") {
		t.Errorf("rows = %+v", repo.rows)
	}

	t.Run("an entry its writer deleted is not made again", func(t *testing.T) {
		delete(journal.notes, repo.rows[0].NoteID)
		journal.created = nil
		if err := svc.EnsureEntries(context.Background(), "c1", playerViewer, now); err != nil {
			t.Fatal(err)
		}
		if len(journal.created) != 0 {
			t.Errorf("made again: %v", journal.created)
		}
	})

	t.Run("someone looking in without joining gets none", func(t *testing.T) {
		journal.created = nil
		admin := permissions.RequestViewer(permissions.RoleNone, "admin")
		if err := svc.EnsureEntries(context.Background(), "c1", admin, now); err != nil {
			t.Fatal(err)
		}
		if len(journal.created) != 0 {
			t.Errorf("made for a non-member: %v", journal.created)
		}
	})
}

func TestNightEntries_ForNight(t *testing.T) {
	date, hhmm, tz, weekly := "2026-10-10", "19:00", "UTC", RecurrenceWeekly
	sess := &Session{
		ID: "s1", CampaignID: "c1", Name: "Game night", Status: StatusPlanned, ScheduledDate: &date, ScheduledTime: &hhmm, ScheduledTZ: &tz,
		IsRecurring: true, RecurrenceType: &weekly,
		Entities: []SessionEntity{
			{EntityID: "harrow", Role: EntityRoleEncountered},
			{EntityID: "miller", Role: EntityRoleMentioned},
		},
	}
	dmEntry, pEntry, p2Entry := nightEntryID("c1", "s1", date, "dm1"), nightEntryID("c1", "s1", date, "p1"), nightEntryID("c1", "s1", date, "p2")
	journal := &fakeJournal{
		notes: map[string]NightNote{
			dmEntry: {ID: dmEntry, Title: "Mine", AuthorID: "dm1", Audience: "private", PageIDs: []string{"maudry", "harrow", "secret"}, Lines: map[string]string{"maudry": "Met @Maudry at the mill."}},
			pEntry:  {ID: pEntry, Title: "Rowan's", AuthorID: "p1", Audience: "gm", PageIDs: []string{"maudry", "ansel"}},
			// Shared with nobody: never read for the check-in.
			p2Entry: {ID: p2Entry, Title: "Private", AuthorID: "p2", Audience: "private", PageIDs: []string{"factor"}},
			// A note someone else owns under a recorded id is not an entry.
			"stolen": {ID: "stolen", Title: "Not theirs", AuthorID: "p1", Audience: "party"},
		},
		readable: map[string][]string{pEntry: {"dm1"}, "stolen": {"dm1", "p2"}},
	}
	repo := &fakeEntryRepo{rows: []NightEntryRow{
		{SessionID: "s1", Date: date, UserID: "dm1", NoteID: dmEntry},
		{SessionID: "s1", Date: date, UserID: "p1", NoteID: pEntry},
		{SessionID: "s1", Date: date, UserID: "p2", NoteID: p2Entry},
		{SessionID: "s1", Date: date, UserID: "p3", NoteID: "stolen"},
	}}
	namer := &fakePageNamer{names: map[string]string{"maudry": "Maudry", "harrow": "Harrow", "ansel": "Brother Ansel", "miller": "The Miller", "grey": "The Grey Factor", "factor": "The Factor"}}
	sightings := &fakeSightingRepo{rows: []SightingRow{
		{SessionID: "s1", OccurrenceDate: "2026-10-10", EntityID: "ansel", State: SightingNotSeen},
		{SessionID: "s1", OccurrenceDate: "2026-10-10", EntityID: "grey", State: SightingSeen},
		{SessionID: "s1", OccurrenceDate: "2026-10-03", EntityID: "maudry", State: SightingSeen},
	}}
	svc := NewNightEntryService(&fakeSessionLister{}, repo, journal, namer, sightings)
	after := time.Date(2026, 10, 10, 23, 0, 0, 0, time.UTC)

	t.Run("the dm gets the entries they can read and the check-in", func(t *testing.T) {
		got, err := svc.ForNight(context.Background(), sess, date, dmViewer, after)
		if err != nil {
			t.Fatal(err)
		}
		var titles []string
		for _, e := range got.Entries {
			titles = append(titles, e.Title+"|"+e.Audience)
		}
		if !equalStrings(titles, []string{"Mine|private", "Rowan's|gm"}) {
			t.Errorf("entries = %v", titles)
		}
		// Harrow is linked as encountered, so already seen; "secret" is a
		// page the namer won't name; the 3 Oct answer is another night; the
		// private entry's Factor is never read.
		want := []string{"maudry||" + dmEntry + "|1", "ansel|not_seen|" + pEntry + "|0", "miller|||0", "grey|seen||0"}
		var g []string
		for _, p := range got.CheckIn {
			g = append(g, p.EntityID+"|"+p.Answer+"|"+p.NoteID+"|"+strconv.Itoa(p.MoreNotes))
		}
		if !equalStrings(g, want) {
			t.Errorf("check-in = %v, want %v", g, want)
		}
		if got.CheckIn[0].Line != "Met @Maudry at the mill." || got.CheckIn[0].Name != "Maudry" {
			t.Errorf("first row = %+v", got.CheckIn[0])
		}
	})

	t.Run("a player sees their own entry first and no check-in", func(t *testing.T) {
		got, err := svc.ForNight(context.Background(), sess, date, playerViewer, after)
		if err != nil {
			t.Fatal(err)
		}
		if got.CheckIn != nil || len(got.Entries) != 1 || got.Entries[0].ID != pEntry {
			t.Errorf("night = %+v", got)
		}
	})

	t.Run("a member without an entry gets one when it is due", func(t *testing.T) {
		p4 := permissions.RequestViewer(permissions.RolePlayer, "p4")
		got, err := svc.ForNight(context.Background(), sess, date, p4, after)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Entries) != 1 || got.Entries[0].Title != "Game night, Sat 10 Oct" || got.Entries[0].AuthorID != "p4" {
			t.Errorf("entries = %+v", got.Entries)
		}
	})

	t.Run("no check-in before the night starts", func(t *testing.T) {
		got, err := svc.ForNight(context.Background(), sess, date, dmViewer, time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		if got.CheckIn != nil {
			t.Errorf("check-in = %v before the start", got.CheckIn)
		}
	})

	for name, d := range map[string]string{"not a date": "10/10/2026", "not one of its nights": "2026-10-11"} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.ForNight(context.Background(), sess, d, dmViewer, after)
			assertAppError(t, err, http.StatusBadRequest)
		})
	}
}

func TestNightEntries_Tags(t *testing.T) {
	repo := &fakeEntryRepo{nights: []NightEntryNight{
		{NightEntryRow: NightEntryRow{SessionID: "s1", Date: "2026-10-10", NoteID: "n1"}, Name: "Game night", Time: "19:00", TZ: "UTC"},
		{NightEntryRow: NightEntryRow{SessionID: "s1", Date: "2026-10-17", NoteID: "n2"}, Name: "Game night", Time: "19:00", TZ: "UTC"},
	}}
	svc := NewNightEntryService(&fakeSessionLister{}, repo, &fakeJournal{}, &fakePageNamer{}, &fakeSightingRepo{})
	got, err := svc.Tags(context.Background(), "c1", []string{"n1", "n2", "n3"}, time.Date(2026, 10, 10, 19, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["n1"].Started || got["n2"].Started || got["n1"].Name != "Game night" {
		t.Errorf("tags = %+v", got)
	}
}

func TestNightEntries_ForNightMakesNoneForAClosedNight(t *testing.T) {
	date := "2026-10-10"
	sess := &Session{ID: "s1", CampaignID: "c1", Name: "Game night", Status: StatusCancelled, ScheduledDate: &date}
	journal := &fakeJournal{}
	svc := NewNightEntryService(&fakeSessionLister{}, &fakeEntryRepo{}, journal, &fakePageNamer{}, &fakeSightingRepo{})
	if _, err := svc.ForNight(context.Background(), sess, date, playerViewer, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if len(journal.created) != 0 {
		t.Errorf("made %v for a cancelled night", journal.created)
	}
}
