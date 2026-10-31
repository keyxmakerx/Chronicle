package sessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

type fakeBellCampaigns struct {
	off     bool
	members []NightBellMember
}

func (f *fakeBellCampaigns) GameNightsOn(context.Context, string) (bool, error) { return !f.off, nil }
func (f *fakeBellCampaigns) BellMembers(context.Context, string) ([]NightBellMember, error) {
	return f.members, nil
}

type fakeBellSessions struct{ sess *Session }

func (f *fakeBellSessions) GetSession(context.Context, string) (*Session, error) { return f.sess, nil }

type sentBell struct{ user, ntype, msg, link string }

type fakeBellNotifier struct{ sent []sentBell }

func (f *fakeBellNotifier) NotifyUsersWithDetail(_ context.Context, userIDs []string, _, ntype, message, _, link string) error {
	for _, u := range userIDs {
		f.sent = append(f.sent, sentBell{u, ntype, message, link})
	}
	return nil
}

func TestBellWhen(t *testing.T) {
	remind, start, end, ok := bellWhen("2026-10-10", "19:00", "America/New_York")
	if !ok {
		t.Fatal("not ok")
	}
	utc := func(x time.Time) string { return x.UTC().Format(time.RFC3339) }
	if utc(remind) != "2026-10-09T13:00:00Z" || utc(start) != "2026-10-10T23:00:00Z" || utc(end) != "2026-10-11T03:00:00Z" {
		t.Errorf("remind %s, start %s, end %s", utc(remind), utc(start), utc(end))
	}
	_, _, end, _ = bellWhen("2026-10-10", "", "")
	if utc(end) != "2026-10-11T00:00:00Z" {
		t.Errorf("a night with no time ends at %s, want the end of its day", utc(end))
	}
}

func TestBellDayWords(t *testing.T) {
	start := time.Date(2026, 10, 10, 19, 0, 0, 0, time.UTC)
	if got := bellDayWords(start, time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)); got != "tomorrow at 7:00 PM" {
		t.Errorf("got %q", got)
	}
	if got := bellDayWords(start, time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)); got != "today at 7:00 PM" {
		t.Errorf("got %q", got)
	}
	if got := bellDayWords(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)); got != "tomorrow" {
		t.Errorf("no time: got %q", got)
	}
}

// bellRig is one planned night on Sat 10 Oct at 19:00 UTC with a DM and two
// players.
type bellRig struct {
	svc      NightEntryService
	repo     *fakeEntryRepo
	journal  *fakeJournal
	notify   *fakeBellNotifier
	camps    *fakeBellCampaigns
	sess     *Session
	pEntry   string
	p2Entry  string
	dmEntry  string
	nightURL string
}

func newBellRig(t *testing.T) *bellRig {
	t.Helper()
	date, hhmm, tz := "2026-10-10", "19:00", "UTC"
	sess := &Session{ID: "s1", CampaignID: "c1", Name: "Game night 14", Status: StatusPlanned, ScheduledDate: &date, ScheduledTime: &hhmm, ScheduledTZ: &tz}
	r := &bellRig{
		repo:     &fakeEntryRepo{bell: []Session{*sess}},
		journal:  &fakeJournal{},
		notify:   &fakeBellNotifier{},
		sess:     sess,
		pEntry:   nightEntryID("c1", "s1", date, "p1"),
		p2Entry:  nightEntryID("c1", "s1", date, "p2"),
		dmEntry:  nightEntryID("c1", "s1", date, "dm1"),
		nightURL: nightsLink("c1", "s1", date),
	}
	r.camps = &fakeBellCampaigns{members: []NightBellMember{
		{UserID: "dm1", Role: permissions.RoleOwner},
		{UserID: "p1", Role: permissions.RolePlayer},
		{UserID: "p2", Role: permissions.RolePlayer},
	}}
	namer := &fakePageNamer{names: map[string]string{"maudry": "Maudry"}}
	r.svc = NewNightEntryService(&fakeSessionLister{}, r.repo, r.journal, namer, &fakeSightingRepo{})
	r.svc.SetBells(r.camps, &fakeBellSessions{sess: sess}, r.notify)
	return r
}

func (r *bellRig) send(t *testing.T, now time.Time) []sentBell {
	t.Helper()
	r.notify.sent = nil
	if err := r.svc.SendBells(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	return r.notify.sent
}

func TestSendBells_TheDayBefore(t *testing.T) {
	r := newBellRig(t)
	if got := r.send(t, time.Date(2026, 10, 9, 8, 59, 0, 0, time.UTC)); len(got) != 0 {
		t.Fatalf("sent before 9:00 the day before: %+v", got)
	}
	got := r.send(t, time.Date(2026, 10, 9, 9, 5, 0, 0, time.UTC))
	if len(got) != 3 {
		t.Fatalf("sent %d, want one per member: %+v", len(got), got)
	}
	for _, b := range got {
		if b.ntype != NotifNightEntryReady || b.msg != "Game night 14 is tomorrow at 7:00 PM. Your Journal has an entry ready for it." {
			t.Errorf("bell %+v", b)
		}
		if b.link != journalLink("c1", nightEntryID("c1", "s1", "2026-10-10", b.user)) {
			t.Errorf("%s's bell opens %s, want their entry", b.user, b.link)
		}
	}
	if len(r.journal.created) != 3 || len(r.repo.rows) != 3 {
		t.Errorf("entries made: %v, rows %d", r.journal.created, len(r.repo.rows))
	}
	if got := r.send(t, time.Date(2026, 10, 9, 9, 15, 0, 0, time.UTC)); len(got) != 0 {
		t.Errorf("sent twice: %+v", got)
	}

	t.Run("a deleted entry points at the night instead", func(t *testing.T) {
		r := newBellRig(t)
		r.repo.rows = []NightEntryRow{{SessionID: "s1", Date: "2026-10-10", UserID: "p1", NoteID: r.pEntry}}
		got := r.send(t, time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC))
		for _, b := range got {
			if b.user != "p1" {
				continue
			}
			if b.msg != "Game night 14 is today at 7:00 PM." || b.link != r.nightURL {
				t.Errorf("bell %+v", b)
			}
			return
		}
		t.Error("p1 got no bell")
	})

	t.Run("a completed or switched-off night sends none", func(t *testing.T) {
		r := newBellRig(t)
		r.repo.bell[0].Status = StatusCompleted
		if got := r.send(t, time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)); len(got) != 0 {
			t.Errorf("completed: %+v", got)
		}
		r = newBellRig(t)
		r.camps.off = true
		if got := r.send(t, time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)); len(got) != 0 {
			t.Errorf("game nights off: %+v", got)
		}
	})
}

func TestSendBells_AfterTheNight(t *testing.T) {
	r := newBellRig(t)
	r.repo.rows = []NightEntryRow{
		{SessionID: "s1", Date: "2026-10-10", UserID: "dm1", NoteID: r.dmEntry},
		{SessionID: "s1", Date: "2026-10-10", UserID: "p1", NoteID: r.pEntry},
		{SessionID: "s1", Date: "2026-10-10", UserID: "p2", NoteID: r.p2Entry},
	}
	r.journal.notes = map[string]NightNote{
		r.dmEntry: {ID: r.dmEntry, AuthorID: "dm1", Audience: "private"},
		// Shared with the GM, naming one person.
		r.pEntry: {ID: r.pEntry, Title: "Rowan's", AuthorID: "p1", Audience: "gm", PageIDs: []string{"maudry"}},
		// Private: never read, so its writer is told how to share it.
		r.p2Entry: {ID: r.p2Entry, AuthorID: "p2", Audience: "private", PageIDs: []string{"secret"}},
	}
	r.journal.readable = map[string][]string{r.pEntry: {"dm1"}}

	if got := r.send(t, time.Date(2026, 10, 10, 22, 30, 0, 0, time.UTC)); len(got) != 0 {
		t.Fatalf("sent while the night is on: %+v", got)
	}
	got := r.send(t, time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC))
	want := []sentBell{
		{"dm1", NotifNightCheckIn, "Game night 14 is over. 1 person named in the entries may have met the party.", r.nightURL},
		{"p2", NotifNightShareEntry, "Game night 14 is over. Share your entry with the GM if you want the people you met counted.", journalLink("c1", r.p2Entry)},
	}
	if len(got) != len(want) {
		t.Fatalf("sent %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("bell %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := r.send(t, time.Date(2026, 10, 11, 1, 0, 0, 0, time.UTC)); len(got) != 0 {
		t.Errorf("sent twice: %+v", got)
	}

	t.Run("an entry shared later brings the check-in then", func(t *testing.T) {
		r2 := newBellRig(t)
		r2.repo.rows = []NightEntryRow{{SessionID: "s1", Date: "2026-10-10", UserID: "p2", NoteID: r2.p2Entry}}
		r2.journal.notes = map[string]NightNote{r2.p2Entry: {ID: r2.p2Entry, AuthorID: "p2", Audience: "private", PageIDs: []string{"maudry"}}}
		got := r2.send(t, time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC))
		if len(got) != 1 || got[0].ntype != NotifNightShareEntry {
			t.Fatalf("at the end: %+v, want only p2's share item", got)
		}
		r2.journal.readable = map[string][]string{r2.p2Entry: {"dm1"}}
		got = r2.send(t, time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC))
		if len(got) != 1 || got[0].user != "dm1" || got[0].ntype != NotifNightCheckIn {
			t.Errorf("after sharing: %+v, want the DM's check-in", got)
		}
	})

	t.Run("nothing to check in about sends the DM nothing", func(t *testing.T) {
		r2 := newBellRig(t)
		if got := r2.send(t, time.Date(2026, 10, 10, 23, 30, 0, 0, time.UTC)); len(got) != 0 {
			t.Errorf("sent %+v", got)
		}
	})

	t.Run("a night long over sends nothing", func(t *testing.T) {
		r2 := newBellRig(t)
		r2.repo.rows, r2.journal.notes, r2.journal.readable = r.repo.rows, r.journal.notes, r.journal.readable
		if got := r2.send(t, time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)); len(got) != 0 {
			t.Errorf("sent %+v", got)
		}
	})
}

// fakeBellRoster answers ListGameNights with one night's list.
type fakeBellRoster struct {
	night GameNight
	today string
}

func (f *fakeBellRoster) ListGameNights(_ context.Context, _, _, _, today string, _ []NightMember) ([]GameNight, error) {
	f.today = today
	return []GameNight{f.night}, nil
}

func TestSendBells_Unanswered(t *testing.T) {
	night := GameNight{SessionID: "s1", Date: "2026-10-10", OrganizerID: "dm1", Roster: []NightAnswer{
		{UserID: "dm1", Answer: NightNoAnswer},
		{UserID: "p1", Answer: NightNoAnswer},
		{UserID: "p2", Answer: NightYes},
		{UserID: "p3", Answer: NightNoAnswer, Excluded: true},
	}}
	tests := []struct {
		name  string
		now   time.Time
		past  bool
		other string
		want  []string
	}{
		{"the day before, only an open answer is asked", time.Date(2026, 10, 9, 9, 5, 0, 0, time.UTC), false, "", []string{"p1"}},
		{"not before 9:00 the day before", time.Date(2026, 10, 9, 8, 59, 0, 0, time.UTC), false, "", nil},
		{"a night already over asks no one", time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC), true, "", nil},
		{"another night's list is ignored", time.Date(2026, 10, 9, 9, 5, 0, 0, time.UTC), false, "s2", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newBellRig(t)
			n := night
			n.Past = tc.past
			if tc.other != "" {
				n.SessionID = tc.other
			}
			roster := &fakeBellRoster{night: n}
			r.svc.SetBellRoster(roster)
			var asked []string
			for _, b := range r.send(t, tc.now) {
				if b.ntype != NotifNightReminder {
					continue
				}
				asked = append(asked, b.user)
				if b.msg != "Game night 14 is tomorrow at 7:00 PM. Can you make it?" || b.link != r.nightURL {
					t.Errorf("bell %+v", b)
				}
			}
			if strings.Join(asked, ",") != strings.Join(tc.want, ",") {
				t.Errorf("asked %v, want %v", asked, tc.want)
			}
			if len(tc.want) > 0 {
				if roster.today != "2026-10-09" {
					t.Errorf("today = %q, want the night's own date for now", roster.today)
				}
				for _, b := range r.send(t, tc.now.Add(10*time.Minute)) {
					if b.ntype == NotifNightReminder {
						t.Errorf("asked twice: %+v", b)
					}
				}
			}
		})
	}
}
