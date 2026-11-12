package armory

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeLog struct{ notes []LogNote }

func (f *fakeLog) List(_ context.Context, _, characterID string, _ int) ([]LogNote, error) {
	var out []LogNote
	for i := len(f.notes) - 1; i >= 0; i-- {
		if f.notes[i].CharacterID == characterID {
			out = append(out, f.notes[i])
		}
	}
	return out, nil
}

func (f *fakeLog) Add(_ context.Context, n LogNote, _ string) error {
	n.ID = int64(len(f.notes) + 1)
	n.CreatedAt = time.Now()
	f.notes = append(f.notes, n)
	return nil
}

func (f *fakeLog) Delete(_ context.Context, _, _ string, id int64) (LogNote, bool, error) {
	for i, n := range f.notes {
		if n.ID == id {
			f.notes = append(f.notes[:i], f.notes[i+1:]...)
			return n, true, nil
		}
	}
	return LogNote{}, false, nil
}

type fakeNights []Night

func (f fakeNights) Nights(context.Context, string) ([]Night, error) { return f, nil }

func at(day string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04", day+" 20:00")
	return t
}

// A line belongs to the latest night that had begun by its day; one from
// before the first night, or with no nights at all, has none.
func TestGroupLog(t *testing.T) {
	entries := []LogEntry{
		{Before: "d", At: at("2026-11-12")},
		{Before: "c", At: at("2026-11-10")},
		{Before: "b", At: at("2026-11-05")},
		{Before: "a", At: at("2026-10-01")},
	}
	nights := []Night{{Name: "Night 14", Date: "2026-11-12"}, {Name: "Night 13", Date: "2026-11-05"}}
	tests := []struct {
		name   string
		nights []Night
		want   []string // group names, in order
		sizes  []int
	}{
		{"nights and a before-the-first", nights, []string{"Night 14", "Night 13", ""}, []int{1, 2, 1}},
		{"no nights at all", nil, []string{""}, []int{4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := groupLog(entries, tc.nights, "2026-11-12")
			if len(got) != len(tc.want) {
				t.Fatalf("groups = %d, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, g := range got {
				if g.Name != tc.want[i] || len(g.Entries) != tc.sizes[i] {
					t.Errorf("group %d = %q/%d, want %q/%d", i, g.Name, len(g.Entries), tc.want[i], tc.sizes[i])
				}
			}
		})
	}
	if g := groupLog(entries, nights, "2026-11-12"); !g[0].Tonight || g[1].Tonight {
		t.Error("only the night dated today is tonight")
	}
}

func TestSplitBold(t *testing.T) {
	b, m, a := splitBold("Given 2 × Rope by Tess.", "Rope")
	if b != "Given 2 × " || m != "Rope" || a != " by Tess." {
		t.Errorf("%q %q %q", b, m, a)
	}
	if b, m, _ := splitBold("Moved something.", "Rope"); b != "Moved something." || m != "" {
		t.Error("a name that is not in the sentence stays unbolded")
	}
}

func logFx(store HeroLogStore) *shareFx {
	f, _ := placesFx()
	f.svc.(*stashService).SetHeroLog(store, fakeNights{{Name: "Night 1", Date: "2026-01-01"}})
	return f
}

// Who may write, mark Director only, name an item, and remove a line.
func TestHeroLog_Notes(t *testing.T) {
	ctx := context.Background()
	tess := Actor{"u1", rPlayer}
	tests := []struct {
		name    string
		who     Actor
		in      LogNoteInput
		wantErr bool
	}{
		{"own player", robin, LogNoteInput{Body: "Owes a favour"}, false},
		{"about a held item", robin, LogNoteInput{Body: "Hums", ItemID: "i1"}, false},
		{"about an item not held", robin, LogNoteInput{Body: "Hums", ItemID: "nope"}, true},
		{"another player", tess, LogNoteInput{Body: "x"}, true},
		{"blank", robin, LogNoteInput{Body: "  "}, true},
		{"too long", robin, LogNoteInput{Body: strings.Repeat("a", maxLogNote+1)}, true},
		{"player marks Director only", robin, LogNoteInput{Body: "x", DirectorOnly: true}, true},
		{"director marks Director only", gm, LogNoteInput{Body: "x", DirectorOnly: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeLog{}
			f := logFx(store)
			err := f.svc.AddLogNote(ctx, "camp", tc.who, "c2", tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if !tc.wantErr && len(store.notes) != 1 {
				t.Fatalf("stored %d", len(store.notes))
			}
		})
	}
}

// The panel shows a player the lines that are not Director only, shows the
// Director all of them, and lets only the author or Owner remove a line.
func TestHeroLog_Panel(t *testing.T) {
	ctx := context.Background()
	store := &fakeLog{}
	f := logFx(store)
	if err := f.svc.AddLogNote(ctx, "camp", robin, "c2", LogNoteInput{Body: "Mine"}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AddLogNote(ctx, "camp", gm, "c2", LogNoteInput{Body: "Secret", DirectorOnly: true}); err != nil {
		t.Fatal(err)
	}
	lines := func(a Actor) (texts []string, v *CharacterPanelView) {
		v, err := f.svc.CharacterPanel(ctx, "camp", a, "c2")
		if err != nil || v == nil {
			t.Fatalf("%v %v", v, err)
		}
		for _, n := range v.Log {
			for _, e := range n.Entries {
				if e.Note {
					texts = append(texts, e.Before)
				}
			}
		}
		return texts, v
	}
	if got, v := lines(robin); len(got) != 1 || got[0] != "Mine" || !v.HasLog || v.CanDirect {
		t.Errorf("player sees %v (CanDirect %v)", got, v.CanDirect)
	}
	if got, v := lines(gm); len(got) != 2 || !v.CanDirect {
		t.Errorf("director sees %v", got)
	}

	if err := f.svc.DeleteLogNote(ctx, "camp", robin, "c2", 2); err == nil {
		t.Error("a player cannot reach a Director-only line, let alone remove it")
	}
	if err := f.svc.DeleteLogNote(ctx, "camp", robin, "c2", 1); err != nil {
		t.Errorf("author removes own line: %v", err)
	}
	if err := f.svc.DeleteLogNote(ctx, "camp", gm, "c2", 2); err != nil {
		t.Errorf("owner removes any line: %v", err)
	}
	if len(store.notes) != 0 {
		t.Errorf("left %d", len(store.notes))
	}
}

// With no store the panel has no log and writing is refused.
func TestHeroLog_Off(t *testing.T) {
	f, _ := placesFx()
	v, err := f.svc.CharacterPanel(context.Background(), "camp", robin, "c2")
	if err != nil || v == nil || v.HasLog {
		t.Fatalf("HasLog on without a store: %v %v", v, err)
	}
	if err := f.svc.AddLogNote(context.Background(), "camp", robin, "c2", LogNoteInput{Body: "x"}); err == nil {
		t.Fatal("want a refusal")
	}
}

// The log draws the written line with its Director-only and about marks, and
// the form posts to the hero's log route.
func TestHeroLog_Renders(t *testing.T) {
	v := &CharacterPanelView{CampaignID: "camp", Character: EntityRef{ID: "c2", Name: "Orrin"}, HasLog: true, CanDirect: true,
		Log: []LogNight{{Name: "Night 14", Tonight: true, Entries: []LogEntry{
			{Before: "Given ", Bold: "Lantern", After: " by Saatraaol."},
			{Note: true, NoteID: 7, Before: "Hums.", About: "Lantern", DirectorOnly: true, CanDelete: true},
		}}}}
	var sb strings.Builder
	if err := CharacterPanel(v, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	for _, want := range []string{"s log</h4>", `hx-post="/campaigns/camp/armory/characters/c2/log"`, "<b>Lantern</b>", "hl-dm-note",
		"about Lantern", "tonight", `/log/7/delete`, `name="director_only"`, "Money and requests"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// A viewer who may act for a hero still gets no panel for a page hidden from
// them, so its name and log stay out of the response.
func TestCharacterPanel_HiddenHeroGivesNoPanel(t *testing.T) {
	f, _ := placesFx()
	f.vis.hidden["c2"] = true
	scribe := Actor{"u9", rScribe}
	v, err := f.svc.CharacterPanel(context.Background(), "camp", scribe, "c2")
	if err != nil || v != nil {
		t.Fatalf("panel for a hidden hero: %v %v", v, err)
	}
	if v, err := f.svc.CharacterPanel(context.Background(), "camp", gm, "c2"); err != nil || v == nil {
		t.Fatalf("owner should still see it: %v %v", v, err)
	}
}
