package sessions

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestNightWindow(t *testing.T) {
	tests := []struct {
		name             string
		date, hhmm, tz   string
		start, from, end string // RFC 3339, UTC
	}{
		{"evening in New York", "2026-10-10", "19:00", "America/New_York", "2026-10-10T23:00:00Z", "2026-10-10T22:00:00Z", "2026-10-11T10:00:00Z"},
		{"no time starts the day", "2026-10-10", "", "Europe/London", "2026-10-09T23:00:00Z", "2026-10-09T23:00:00Z", "2026-10-11T05:00:00Z"},
		{"no zone is UTC", "2026-10-10", "20:30", "", "2026-10-10T20:30:00Z", "2026-10-10T19:30:00Z", "2026-10-11T06:00:00Z"},
		// The clocks go back overnight: 6 a.m. is still 6 a.m. local.
		{"across a clock change", "2026-10-24", "20:00", "Europe/London", "2026-10-24T19:00:00Z", "2026-10-24T18:00:00Z", "2026-10-25T06:00:00Z"},
		{"an unknown zone falls back to UTC", "2026-10-10", "20:00", "Mars/Olympus", "2026-10-10T20:00:00Z", "2026-10-10T19:00:00Z", "2026-10-11T06:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, from, to, ok := nightWindow(tt.date, tt.hhmm, tt.tz)
			if !ok {
				t.Fatal("not ok")
			}
			for _, c := range []struct {
				what string
				got  time.Time
				want string
			}{{"start", start, tt.start}, {"from", from, tt.from}, {"to", to, tt.end}} {
				if g := c.got.UTC().Format(time.RFC3339); g != c.want {
					t.Errorf("%s = %s, want %s", c.what, g, c.want)
				}
			}
		})
	}
}

type fakeNightNotes struct {
	notes    []NightNote
	from, to time.Time
	viewer   permissions.Viewer
}

func (f *fakeNightNotes) NotesWrittenBetween(_ context.Context, _ string, v permissions.Viewer, from, to time.Time) ([]NightNote, bool, error) {
	f.from, f.to, f.viewer = from, to, v
	return f.notes, false, nil
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

func TestNightNotes_ForNight(t *testing.T) {
	date, hhmm, tz, weekly := "2026-10-10", "19:00", "UTC", RecurrenceWeekly
	sess := &Session{
		ID: "s1", CampaignID: "c1", ScheduledDate: &date, ScheduledTime: &hhmm, ScheduledTZ: &tz,
		IsRecurring: true, RecurrenceType: &weekly,
		Entities: []SessionEntity{
			{EntityID: "harrow", Role: EntityRoleEncountered},
			{EntityID: "miller", Role: EntityRoleMentioned},
		},
	}
	at := func(h int) time.Time { return time.Date(2026, 10, 10, h, 0, 0, 0, time.UTC) }
	reader := &fakeNightNotes{notes: []NightNote{
		{ID: "n1", Title: "Mill", AuthorID: "dm1", CreatedAt: at(19), Audience: "party", PageIDs: []string{"maudry", "harrow", "secret"}, Lines: map[string]string{"maudry": "Met @Maudry at the mill."}},
		{ID: "n2", Title: "Ledger", AuthorID: "p1", CreatedAt: at(21), Audience: "gm", PageIDs: []string{"maudry", "ansel"}},
	}}
	namer := &fakePageNamer{names: map[string]string{"maudry": "Maudry", "harrow": "Harrow", "ansel": "Brother Ansel", "miller": "The Miller", "grey": "The Grey Factor"}}
	sightings := &fakeSightingRepo{rows: []SightingRow{
		{SessionID: "s1", OccurrenceDate: "2026-10-10", EntityID: "ansel", State: SightingNotSeen},
		{SessionID: "s1", OccurrenceDate: "2026-10-10", EntityID: "grey", State: SightingSeen},
		{SessionID: "s1", OccurrenceDate: "2026-10-03", EntityID: "maudry", State: SightingSeen},
	}}
	svc := NewNightNotesService(reader, namer, sightings)
	after := at(23)

	t.Run("the dm gets the notes and the check-in", func(t *testing.T) {
		got, err := svc.ForNight(context.Background(), sess, date, dmViewer, after)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Notes) != 2 || got.Notes[1].Audience != "gm" {
			t.Errorf("notes = %+v", got.Notes)
		}
		if w := "2026-10-10T18:00:00Z|2026-10-11T06:00:00Z"; reader.from.Format(time.RFC3339)+"|"+reader.to.Format(time.RFC3339) != w {
			t.Errorf("window = %s..%s, want %s", reader.from, reader.to, w)
		}
		// Harrow is linked as encountered, so already seen; "secret" is a
		// page the namer won't name; the 3 Oct answer is another night.
		want := []string{"maudry||n1|1", "ansel|not_seen|n2|0", "miller|||0", "grey|seen||0"}
		var g []string
		for _, p := range got.CheckIn {
			g = append(g, p.EntityID+"|"+p.Answer+"|"+p.NoteID+"|"+strconv.Itoa(p.MoreNotes))
		}
		if !equalStrings(g, want) {
			t.Errorf("check-in = %v, want %v", g, want)
		}
		if got.CheckIn[0].Line != "Met @Maudry at the mill." || got.CheckIn[0].Name != "Maudry" || !got.CheckIn[0].NoteAt.Equal(at(19)) {
			t.Errorf("first row = %+v", got.CheckIn[0])
		}
	})

	t.Run("a player gets the notes they can read and no check-in", func(t *testing.T) {
		got, err := svc.ForNight(context.Background(), sess, date, playerViewer, after)
		if err != nil {
			t.Fatal(err)
		}
		if got.CheckIn != nil || reader.viewer.UserID() != "p1" {
			t.Errorf("check-in = %v, read as %q", got.CheckIn, reader.viewer.UserID())
		}
	})

	t.Run("no check-in before the night starts", func(t *testing.T) {
		got, err := svc.ForNight(context.Background(), sess, date, dmViewer, at(18))
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
