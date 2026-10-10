package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// fakeNoteStore is an in-memory notes.NoteService for the three calls the
// screen makes. raceOnCreate makes Create fail the way a duplicate key does,
// after another DM's note landed first.
type fakeNoteStore struct {
	notes.NoteService
	byID         map[string]*notes.Note
	creates      int
	raceOnCreate bool
}

func (f *fakeNoteStore) GetByID(_ context.Context, id string) (*notes.Note, error) {
	if n, ok := f.byID[id]; ok {
		c := *n
		return &c, nil
	}
	return nil, apperror.NewNotFound("note not found")
}

func (f *fakeNoteStore) Create(_ context.Context, campaignID string, creator permissions.Viewer, req notes.CreateNoteRequest) (*notes.Note, error) {
	f.creates++
	if f.raceOnCreate {
		f.byID[req.ID] = &notes.Note{ID: req.ID, CampaignID: campaignID, UserID: "own1", Title: "Theirs", SharedWithGM: true}
		return nil, errors.New("duplicate key")
	}
	n := &notes.Note{ID: req.ID, CampaignID: campaignID, UserID: creator.UserID(), Title: req.Title, SharedWithGM: req.Visibility == notes.VisibilityGM}
	f.byID[req.ID] = n
	return n, nil
}

func (f *fakeNoteStore) Update(_ context.Context, id string, _ permissions.Viewer, req notes.UpdateNoteRequest) (*notes.Note, error) {
	n := f.byID[id]
	if req.Title != nil {
		n.Title = *req.Title
	}
	if req.Entry != nil {
		n.Entry = req.Entry
	}
	c := *n
	return &c, nil
}

// fakeMembers lists a campaign whose owner is "own1".
type fakeMembers struct{}

func (fakeMembers) ListMembers(context.Context, string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{
		{UserID: "codm", Role: campaigns.RolePlayer},
		{UserID: "own1", Role: campaigns.RoleOwner},
	}, nil
}

type failingCreate struct{ fakeNoteStore }

func (f *failingCreate) Create(context.Context, string, permissions.Viewer, notes.CreateNoteRequest) (*notes.Note, error) {
	return nil, errors.New("db down")
}

func TestDMNotesAdapter_Save(t *testing.T) {
	ctx := context.Background()
	owner := dmscreen.Viewer{UserID: "own1", Role: 3}
	coDM := dmscreen.Viewer{UserID: "codm", Role: 3}

	t.Run("first save creates with the title, later saves keep it", func(t *testing.T) {
		f := &fakeNoteStore{byID: map[string]*notes.Note{}}
		a := &dmNotesAdapter{notes: f, members: fakeMembers{}}
		first, err := a.Save(ctx, "c1", "s1", owner, "Session 12: DM notes", `{"v":1}`, "", "")
		if err != nil || first.Title != "Session 12: DM notes" {
			t.Fatalf("first: %v %+v", err, first)
		}
		second, err := a.Save(ctx, "c1", "s1", owner, "Session 13: DM notes", `{"v":2}`, "", dmscreen.BodyVersion(first.Entry))
		if err != nil || second.Title != "Session 12: DM notes" || second.Entry != `{"v":2}` || f.creates != 1 {
			t.Fatalf("second: %v %+v creates %d", err, second, f.creates)
		}
	})

	t.Run("a create that loses the race reports a conflict and keeps the winner's note", func(t *testing.T) {
		f := &fakeNoteStore{byID: map[string]*notes.Note{}, raceOnCreate: true}
		a := &dmNotesAdapter{notes: f, members: fakeMembers{}}
		_, err := a.Save(ctx, "c1", "s1", owner, "Session 12: DM notes", `{"v":1}`, "", "")
		if !errors.Is(err, dmscreen.ErrNoteChanged) {
			t.Fatalf("err = %v, want ErrNoteChanged", err)
		}
		if f.byID[dmNoteID("c1", "s1")].Entry != nil {
			t.Error("the losing save overwrote the winner's note")
		}
	})

	t.Run("a co-DM's first save creates the note in the owner's name, and a later co-DM save updates it", func(t *testing.T) {
		f := &fakeNoteStore{byID: map[string]*notes.Note{}}
		a := &dmNotesAdapter{notes: f, members: fakeMembers{}}
		first, err := a.Save(ctx, "c1", "s1", coDM, "t", `{"v":1}`, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if got := f.byID[dmNoteID("c1", "s1")].UserID; got != "own1" {
			t.Fatalf("note owner = %q, want the campaign owner", got)
		}
		second, err := a.Save(ctx, "c1", "s1", coDM, "t", `{"v":2}`, "", dmscreen.BodyVersion(first.Entry))
		if err != nil || second.Entry != `{"v":2}` {
			t.Fatalf("second: %v %+v", err, second)
		}
	})

	t.Run("a note at the id owned by someone else is hidden and never written over", func(t *testing.T) {
		id := dmNoteID("c1", "s1")
		body := `{"v":"theirs"}`
		f := &fakeNoteStore{byID: map[string]*notes.Note{id: {ID: id, CampaignID: "c1", UserID: "squatter", SharedWithGM: true, Entry: &body}}}
		a := &dmNotesAdapter{notes: f, members: fakeMembers{}}
		if got, err := a.Find(ctx, "c1", "s1", owner); err != nil || got != nil {
			t.Fatalf("Find = %+v, %v; want absent", got, err)
		}
		if _, err := a.Save(ctx, "c1", "s1", owner, "t", `{"v":1}`, "", ""); err == nil {
			t.Fatal("want a refusal")
		}
		if *f.byID[id].Entry != body || f.creates != 0 {
			t.Error("the squatter's note was touched")
		}
	})

	t.Run("a create that fails for another reason fails", func(t *testing.T) {
		f := &failingCreate{fakeNoteStore{byID: map[string]*notes.Note{}}}
		if _, err := (&dmNotesAdapter{notes: f, members: fakeMembers{}}).Save(ctx, "c1", "s1", owner, "t", "{}", "", ""); err == nil {
			t.Fatal("want the create error")
		}
	})

	t.Run("stale or missing versions are refused", func(t *testing.T) {
		f := &fakeNoteStore{byID: map[string]*notes.Note{}}
		a := &dmNotesAdapter{notes: f, members: fakeMembers{}}
		first, _ := a.Save(ctx, "c1", "s1", owner, "t", `{"v":1}`, "", "")
		for _, version := range []string{"", "stale"} {
			if _, err := a.Save(ctx, "c1", "s1", owner, "t", `{"v":9}`, "", version); !errors.Is(err, dmscreen.ErrNoteChanged) {
				t.Errorf("version %q: err = %v, want ErrNoteChanged", version, err)
			}
		}
		if got, _ := a.Find(ctx, "c1", "s1", owner); got.Entry != first.Entry {
			t.Error("a refused save changed the note")
		}
		// A tab that saw a note must not recreate one that was deleted.
		if _, err := a.Save(ctx, "c1", "other-night", owner, "t", "{}", "", "someversion"); !errors.Is(err, dmscreen.ErrNoteChanged) {
			t.Errorf("deleted note: err = %v", err)
		}
	})
}

func TestDMNoteID(t *testing.T) {
	a := dmNoteID("c1", "s1:2026-10-16")
	tests := []struct {
		name string
		x, y string
		same bool
	}{
		{"same night, same id", a, dmNoteID("c1", "s1:2026-10-16"), true},
		{"two occurrences of one series differ", a, dmNoteID("c1", "s1:2026-10-23"), false},
		{"same night in another campaign differs", a, dmNoteID("c2", "s1:2026-10-16"), false},
		{"standing note differs from a night's", dmNoteID("c1", ""), a, false},
		{"standing note is stable", dmNoteID("c1", ""), dmNoteID("c1", ""), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if (tc.x == tc.y) != tc.same {
				t.Errorf("%s vs %s, same = %v", tc.x, tc.y, tc.same)
			}
		})
	}
}

func TestNightNoteKey(t *testing.T) {
	tests := []struct {
		name  string
		night sessions.GameNight
		want  string
	}{
		{"one-off night keeps its note if moved", sessions.GameNight{SessionID: "s1", Date: "2026-10-16"}, "s1"},
		{"each occurrence of a repeating session", sessions.GameNight{SessionID: "s1", Date: "2026-10-16", Recurring: true}, "s1:2026-10-16"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := nightNoteKey(tc.night); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLegacyNoteText(t *testing.T) {
	got := legacyNoteText([]notes.Block{{Type: "text", Value: "one"}, {Type: "checklist", Items: []notes.ChecklistItem{{Text: "two"}, {Text: "three"}}}})
	if got != "one\ntwo\nthree" {
		t.Errorf("got %q", got)
	}
	if toScreenNote(&notes.Note{ID: "n", Content: []notes.Block{{Type: "text"}}}).Legacy == "" {
		t.Error("a block-only note must read as legacy, so the screen never overwrites it")
	}
}
