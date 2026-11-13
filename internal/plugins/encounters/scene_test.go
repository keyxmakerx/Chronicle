package encounters

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeScenes is a SceneDirectory over campaign c1: two players, and pages
// the players may or may not see. Reveal un-hides a page.
type fakeScenes struct {
	players  []Player
	hidden   map[string]bool
	revealed []string
	names    map[string]string
}

func newFakeScenes(pages *fakePages) *fakeScenes {
	return &fakeScenes{
		players: []Player{{UserID: "u-player", Name: "Ash (Sam)"}, {UserID: "u-two", Name: "Bryn"}},
		hidden:  map[string]bool{"gob": true},
		names:   pages.names,
	}
}

func (f *fakeScenes) Players(_ context.Context, _ string) ([]Player, error) { return f.players, nil }

func (f *fakeScenes) PlayerCanView(_ context.Context, _ string, entityID, _ string) (bool, error) {
	return !f.hidden[entityID], nil
}

func (f *fakeScenes) Reveal(_ context.Context, _ string, entityID string) (string, error) {
	delete(f.hidden, entityID)
	f.revealed = append(f.revealed, entityID)
	return f.names[entityID], nil
}

// memShown is an in-memory ShownRepository.
type memShown struct{ rows []ShownItem }

func (m *memShown) AddShown(_ context.Context, _ string, it ShownItem, _ string) error {
	m.rows = append(m.rows, it)
	return nil
}

func (m *memShown) ListShown(_ context.Context, _ string, since time.Time, limit int) ([]ShownItem, error) {
	var out []ShownItem
	for _, it := range m.rows {
		if !it.ShownAt.Before(since) {
			out = append(out, it)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// fakePub records what went to the players' connections.
type fakePub struct{ sent []ShownItem }

func (p *fakePub) Shown(_ string, it ShownItem) { p.sent = append(p.sent, it) }

func tavern() SaveInput {
	return SaveInput{
		Name: "The Gilded Flagon",
		Kind: KindScene,
		Shows: []Show{
			{ID: "s1", Kind: ShowPage, EntityID: "page2"},
			{ID: "s2", Kind: ShowNote, Label: "Hessa slips you a vial", Text: "It smells of pine.", To: "u-player"},
			{ID: "s3", Kind: ShowReveal, EntityID: "gob", Label: "The chief"},
			{ID: "s4", Kind: ShowPage, EntityID: "hero1"},
		},
	}
}

type sceneRig struct {
	runs   RunService
	enc    Service
	scenes *fakeScenes
	shown  *memShown
	pub    *fakePub
	id     string
}

func newSceneRig(t *testing.T) sceneRig {
	t.Helper()
	enc, _, pages := newTestService()
	scenes := newFakeScenes(pages)
	enc.(interface{ SetSceneDirectory(SceneDirectory) }).SetSceneDirectory(scenes)
	in := tavern()
	in.Ready = true
	e, err := enc.Create(context.Background(), "c1", "page1", dm, in)
	if err != nil {
		t.Fatalf("create scene: %v", err)
	}
	runs := NewRunService(newMemRunRepo(), enc, pages)
	shown, pub := &memShown{}, &fakePub{}
	runs.(interface {
		SetShowing(SceneDirectory, ShownRepository, ShowPublisher)
	}).SetShowing(scenes, shown, pub)
	return sceneRig{runs: runs, enc: enc, scenes: scenes, shown: shown, pub: pub, id: e.ID}
}

func TestValidateScene(t *testing.T) {
	long := strings.Repeat("x", MaxShowText+1)
	cases := []struct {
		name string
		edit func(*SaveInput)
		want string
	}{
		{"a scene as built", func(*SaveInput) {}, ""},
		{"a scene may not hold creatures", func(in *SaveInput) {
			in.Creatures = []Creature{{ID: "r1", Name: "Goblin", Count: 1, Group: "A"}}
		}, "scene"},
		{"a fight may not hold things to show", func(in *SaveInput) { in.Kind = KindFight }, "show"},
		{"a page row needs its page", func(in *SaveInput) { in.Shows[0].EntityID = "" }, "page"},
		{"a note needs its words", func(in *SaveInput) { in.Shows[1].Text = "  " }, "note"},
		{"a note's words have a limit", func(in *SaveInput) { in.Shows[1].Text = long }, ""},
		{"a reveal can't go to just one player", func(in *SaveInput) { in.Shows[2].To = "u-player" }, "everyone"},
		{"rows need their own ids", func(in *SaveInput) { in.Shows[1].ID = "s1" }, ""},
		{"an unknown kind is refused", func(in *SaveInput) { in.Shows[0].Kind = "song" }, ""},
		{"too many rows", func(in *SaveInput) {
			for len(in.Shows) <= MaxShows {
				in.Shows = append(in.Shows, Show{ID: fmt.Sprintf("n%d", len(in.Shows)), Kind: ShowNote, Label: "n", Text: "n"})
			}
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tavern()
			tc.edit(&in)
			_, _, err := validateInput(in)
			if tc.name == "a scene as built" {
				if err != nil {
					t.Fatalf("want it saved, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want a refusal")
			}
			if tc.want != "" && !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("refusal %q doesn't mention %q", err, tc.want)
			}
		})
	}
}

func TestSceneSaveChecksPlayersAndPages(t *testing.T) {
	enc, _, pages := newTestService()
	ctx := context.Background()

	// Without a scene directory only notes to everyone can be saved.
	in := tavern()
	if _, err := enc.Create(ctx, "c1", "page1", dm, in); statusOf(err) != 422 {
		t.Fatalf("no directory: want 422, got %v", err)
	}
	notes := SaveInput{Name: "Notes", Kind: KindScene, Shows: []Show{{ID: "n1", Kind: ShowNote, Label: "A", Text: "B"}}}
	if _, err := enc.Create(ctx, "c1", "page1", dm, notes); err != nil {
		t.Fatalf("notes to everyone: %v", err)
	}

	enc.(interface{ SetSceneDirectory(SceneDirectory) }).SetSceneDirectory(newFakeScenes(pages))
	stranger := tavern()
	stranger.Shows[1].To = "u-nobody"
	if _, err := enc.Create(ctx, "c1", "page1", dm, stranger); statusOf(err) != 422 {
		t.Errorf("a stranger as To: want 422, got %v", err)
	}
	secret := tavern()
	secret.Shows[0].EntityID = "secret"
	if _, err := enc.Create(ctx, "c1", "page1", dm, secret); err == nil {
		t.Errorf("a page the DM can't see should be refused")
	}
	e, err := enc.Create(ctx, "c1", "page1", dm, tavern())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if e.Kind != KindScene || len(e.Shows) != 4 {
		t.Fatalf("got kind %q with %d rows", e.Kind, len(e.Shows))
	}
	if e.Shows[0].Label != "Lastwell" {
		t.Errorf("an empty label takes the page's name, got %q", e.Shows[0].Label)
	}
}

func TestSceneRunShowsToPlayers(t *testing.T) {
	r := newSceneRig(t)
	ctx := context.Background()
	view, err := r.runs.Start(ctx, "c1", dm, RunStart{EncounterID: r.id})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if view.Run.Kind != KindScene || len(view.Run.Shows) != 4 || len(view.Run.Lines) != 0 {
		t.Fatalf("a scene runs with its rows and no lines: %+v", view.Run)
	}

	// A page row goes to everyone with the page's link and name.
	view, err = r.runs.Show(ctx, "c1", dm, RunShowInput{ShowID: "s1"})
	if err != nil {
		t.Fatalf("show page: %v", err)
	}
	if len(r.pub.sent) != 1 || r.pub.sent[0].EntityID != "page2" || r.pub.sent[0].Label != "Lastwell" || r.pub.sent[0].To != "" {
		t.Fatalf("page sent as %+v", r.pub.sent)
	}
	if view.Run.Shows[0].ShownAt == nil {
		t.Errorf("the row should say when it was shown")
	}

	// A note keeps its words and its one player.
	if _, err := r.runs.Show(ctx, "c1", dm, RunShowInput{ShowID: "s2"}); err != nil {
		t.Fatalf("show note: %v", err)
	}
	if it := r.pub.sent[1]; it.To != "u-player" || it.Text != "It smells of pine." || it.EntityID != "" {
		t.Errorf("note sent as %+v", it)
	}

	// A reveal un-hides first, then shows.
	if _, err := r.runs.Show(ctx, "c1", dm, RunShowInput{ShowID: "s3"}); err != nil {
		t.Fatalf("show reveal: %v", err)
	}
	if len(r.scenes.revealed) != 1 || r.scenes.revealed[0] != "gob" {
		t.Errorf("reveal should un-hide gob, got %v", r.scenes.revealed)
	}

	// A page hidden from players is refused with a reason, and nothing is sent.
	r.scenes.hidden["hero1"] = true
	before := len(r.pub.sent)
	if _, err := r.runs.Show(ctx, "c1", dm, RunShowInput{ShowID: "s4"}); statusOf(err) != 422 || !strings.Contains(err.Error(), "hidden from the players") {
		t.Errorf("hidden page: want 422 saying it's hidden, got %v", err)
	}
	if len(r.pub.sent) != before || len(r.shown.rows) != before {
		t.Errorf("a refused show must not be sent or logged")
	}

	if _, err := r.runs.Show(ctx, "c1", dm, RunShowInput{ShowID: "nope"}); statusOf(err) != 404 {
		t.Errorf("unknown row: want 404, got %v", err)
	}
	for _, v := range []Viewer{scribe, player} {
		if _, err := r.runs.Show(ctx, "c1", v, RunShowInput{ShowID: "s1"}); statusOf(err) != 403 {
			t.Errorf("show as %d: want 403, got %v", v.VisibilityRole, err)
		}
	}
}

func TestShownListsOnlyWhatTheViewerWasShown(t *testing.T) {
	r := newSceneRig(t)
	ctx := context.Background()
	if _, err := r.runs.Start(ctx, "c1", dm, RunStart{EncounterID: r.id}); err != nil {
		t.Fatalf("start: %v", err)
	}
	for _, id := range []string{"s1", "s2"} {
		if _, err := r.runs.Show(ctx, "c1", dm, RunShowInput{ShowID: id}); err != nil {
			t.Fatalf("show %s: %v", id, err)
		}
	}
	since := time.Now().Add(-time.Hour)
	cases := []struct {
		name string
		v    Viewer
		want int
	}{
		{"the DM sees both", dm, 2},
		{"the named player sees both", player, 2},
		{"another player sees only the one for everyone", Viewer{UserID: "u-two", VisibilityRole: player.VisibilityRole}, 1},
	}
	for _, tc := range cases {
		got, err := r.runs.Shown(ctx, "c1", tc.v, since)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: got %d items", tc.name, len(got))
		}
	}
	if got, _ := r.runs.Shown(ctx, "c1", dm, time.Now().Add(time.Hour)); len(got) != 0 {
		t.Errorf("nothing is newer than the window, got %d", len(got))
	}
}

func TestShowRefusedForAFight(t *testing.T) {
	runs, _, _, id := newRunTest(t)
	runs.(interface {
		SetShowing(SceneDirectory, ShownRepository, ShowPublisher)
	}).SetShowing(nil, &memShown{}, &fakePub{})
	ctx := context.Background()
	if _, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := runs.Show(ctx, "c1", dm, RunShowInput{ShowID: "s1"}); statusOf(err) != 409 {
		t.Errorf("show during a fight: want 409, got %v", err)
	}
}

func TestSceneUpdateWithoutKindStaysAScene(t *testing.T) {
	r := newSceneRig(t)
	ctx := context.Background()
	e, err := r.enc.Get(ctx, "c1", r.id, dm)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	in := SaveInput{Name: e.Name, Shows: []Show{}, Version: &e.Version}
	got, err := r.enc.Update(ctx, "c1", r.id, dm, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Kind != KindScene {
		t.Errorf("a save without kind turned the scene into %q", got.Kind)
	}
}
