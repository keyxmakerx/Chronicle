package encounters

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// memRunRepo is an in-memory RunRepository with the same version rules as
// SQL.
type memRunRepo struct{ rows map[string]RunRow }

func newMemRunRepo() *memRunRepo { return &memRunRepo{rows: map[string]RunRow{}} }

func (m *memRunRepo) GetRun(_ context.Context, campaignID string) (RunRow, bool, error) {
	r, ok := m.rows[campaignID]
	return r, ok, nil
}

func (m *memRunRepo) CreateRun(_ context.Context, campaignID string, data []byte, _ string) (bool, error) {
	if _, ok := m.rows[campaignID]; ok {
		return false, nil
	}
	m.rows[campaignID] = RunRow{Data: data, Version: 1}
	return true, nil
}

func (m *memRunRepo) UpdateRun(_ context.Context, campaignID string, data []byte, expected int, _ string) (bool, error) {
	r, ok := m.rows[campaignID]
	if !ok || r.Version != expected {
		return false, nil
	}
	m.rows[campaignID] = RunRow{Data: data, Version: r.Version + 1}
	return true, nil
}

func (m *memRunRepo) EndRun(_ context.Context, campaignID string) (RunRow, bool, error) {
	r, ok := m.rows[campaignID]
	delete(m.rows, campaignID)
	return r, ok, nil
}

func newRunTest(t *testing.T) (RunService, Service, *fakePages, string) {
	t.Helper()
	enc, _, pages := newTestService()
	in := ramparts()
	in.Ready = true
	e, err := enc.Create(context.Background(), "c1", "page1", dm, in)
	if err != nil {
		t.Fatalf("create encounter: %v", err)
	}
	return NewRunService(newMemRunRepo(), enc, pages), enc, pages, e.ID
}

func TestRunRefusesAllButTheDM(t *testing.T) {
	runs, _, _, id := newRunTest(t)
	ctx := context.Background()
	for _, v := range []Viewer{scribe, player} {
		if _, err := runs.Current(ctx, "c1", v); statusOf(err) != 403 {
			t.Errorf("current as %d: %v", v.VisibilityRole, err)
		}
		if _, err := runs.Start(ctx, "c1", v, RunStart{EncounterID: id}); statusOf(err) != 403 {
			t.Errorf("start as %d: %v", v.VisibilityRole, err)
		}
		if _, err := runs.Save(ctx, "c1", v, RunSave{Version: 1, Run: &Run{Round: 1}}); statusOf(err) != 403 {
			t.Errorf("save as %d: %v", v.VisibilityRole, err)
		}
		if _, err := runs.End(ctx, "c1", v); statusOf(err) != 403 {
			t.Errorf("end as %d: %v", v.VisibilityRole, err)
		}
	}
}

func TestRunStartLaysOutTheEncounter(t *testing.T) {
	runs, _, _, id := newRunTest(t)
	ctx := context.Background()
	before, err := runs.Current(ctx, "c1", dm)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if before.Run != nil || len(before.Ready) != 1 || before.Ready[0].ID != id {
		t.Fatalf("before the fight: %+v", before)
	}
	view, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	r := view.Run
	if r == nil || r.Name != "Ramparts fight" || r.Round != 1 || r.Turn != "" || view.Version != 1 {
		t.Fatalf("run: %+v", view)
	}
	want := []struct {
		id, kind, entity string
		boxes            int
		waiting          bool
	}{
		{"h-hero1", LineHero, "hero1", 1, false},
		{"h-hero2", LineHero, "hero2", 1, false},
		{"r1", LineCreature, "gob", 1, false},
		{"r2", LineCreature, "hob", 2, false},
		{"r3", LineCreature, "", 4, true},
	}
	if len(r.Lines) != len(want) {
		t.Fatalf("lines: %+v", r.Lines)
	}
	for i, w := range want {
		l := r.Lines[i]
		if l.ID != w.id || l.Kind != w.kind || l.EntityID != w.entity || len(l.HP) != w.boxes || l.Waiting != w.waiting {
			t.Errorf("line %d = %+v, want %+v", i, l, w)
		}
	}
	if len(view.Ready) != 0 {
		t.Errorf("a running fight lists encounters to start: %+v", view.Ready)
	}
	if _, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id}); statusOf(err) != 409 {
		t.Errorf("second start: %v", err)
	}
}

func TestRunStartRefusesAnEncounterTheDMCannotSee(t *testing.T) {
	runs, _, pages, id := newRunTest(t)
	pages.hidden["page1"] = true
	if _, err := runs.Start(context.Background(), "c1", dm, RunStart{EncounterID: id}); statusOf(err) != 404 {
		t.Errorf("start on a hidden page: %v", err)
	}
	if _, err := runs.Start(context.Background(), "c1", dm, RunStart{EncounterID: "nope"}); statusOf(err) != 404 {
		t.Errorf("start of a missing encounter: %v", err)
	}
}

func TestRunSaveKeepsWhatTheFightIs(t *testing.T) {
	runs, _, _, id := newRunTest(t)
	ctx := context.Background()
	view, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	r := *view.Run
	r.Name, r.EncounterID = "Renamed", "other"
	r.Lines = append([]RunLine{}, r.Lines...)
	r.Lines[2].EntityID = "secret"
	r.Lines[2].Init, r.Lines[2].Max = intp(14), intp(30)
	r.Lines[2].HP = []*int{intp(-3)}
	r.Lines[2].Conds = []string{"Grappled", "Grappled", " "}
	r.Lines = append(r.Lines, RunLine{ID: "x1", Kind: LineCreature, Name: "A rat", EntityID: "hob", HP: []*int{nil}})
	r.Turn, r.Round = "r1", 2
	saved, err := runs.Save(ctx, "c1", dm, RunSave{Version: view.Version, Run: &r})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	got := saved.Run
	if got.Name != "Ramparts fight" || got.EncounterID != id {
		t.Errorf("the save renamed the fight: %+v", got)
	}
	if got.Lines[2].EntityID != "gob" || got.Lines[5].EntityID != "" {
		t.Errorf("a save set a line's page: %+v / %+v", got.Lines[2], got.Lines[5])
	}
	if l := got.Lines[2]; *l.Init != 14 || *l.Max != 30 || *l.HP[0] != -3 || len(l.Conds) != 1 {
		t.Errorf("line not kept: %+v", l)
	}
	if saved.Version != 2 || got.Turn != "r1" || got.Round != 2 {
		t.Errorf("saved: %+v", saved)
	}
	if _, err := runs.Save(ctx, "c1", dm, RunSave{Version: view.Version, Run: &r}); statusOf(err) != 409 {
		t.Errorf("a stale save: %v", err)
	}
	if _, err := runs.End(ctx, "c1", dm); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, err := runs.Save(ctx, "c1", dm, RunSave{Version: 2, Run: &r}); statusOf(err) != 409 {
		t.Errorf("a save after the end: %v", err)
	}
	after, err := runs.Current(ctx, "c1", dm)
	if err != nil || after.Run != nil || len(after.Ready) != 1 {
		t.Errorf("after the end: %+v, %v", after, err)
	}
}

// fakeListener is an app that hears about fights.
type fakeListener struct {
	ended  []EndedFight
	leaves []string
	fail   bool
}

func (f *fakeListener) Leaves(_ context.Context, _ string, _ Viewer, ids []string) (map[string]string, error) {
	f.leaves = append(f.leaves, ids...)
	return map[string]string{"hob": "Has a drop list"}, nil
}

func (f *fakeListener) FightEnded(_ context.Context, _ string, _ Viewer, fight EndedFight) (*EndReport, error) {
	f.ended = append(f.ended, fight)
	if f.fail {
		return nil, errors.New("tray is down")
	}
	return &EndReport{Lines: []string{"Hobgoblin Soldier left spoils."}, Link: "/somewhere", LinkText: "Open it"}, nil
}

func TestRunEndTellsTheListenerWhoWasDown(t *testing.T) {
	runs, _, _, id := newRunTest(t)
	ctx := context.Background()
	l := &fakeListener{}
	runs.(*runService).SetFightListener(l)
	view, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	l.leaves = nil
	if view, err = runs.Current(ctx, "c1", dm); err != nil || view.Leaves["hob"] != "Has a drop list" {
		t.Fatalf("leaves: %+v, %v", view, err)
	}
	if strings.Join(l.leaves, ",") != "gob,hob" {
		t.Errorf("asked about %v; only creatures with a page", l.leaves)
	}
	r := *view.Run
	r.Lines = append([]RunLine{}, r.Lines...)
	// One hobgoblin at 0 and one at full, the waiting skeletons at -1, a
	// hero at 0: only the hobgoblin counts as down. The war chief's full
	// hit points were typed as 0, so the tab draws it down, and it is.
	r.Lines[0].HP = []*int{intp(0)}
	r.Lines[3].HP = []*int{intp(0), nil}
	r.Lines[4].HP = []*int{intp(-1), nil, nil, nil}
	for i := range r.Lines {
		if r.Lines[i].EntityID == "gob" {
			r.Lines[i].Max = intp(0)
		}
	}
	if _, err := runs.Save(ctx, "c1", dm, RunSave{Version: view.Version, Run: &r}); err != nil {
		t.Fatalf("save: %v", err)
	}
	rep, err := runs.End(ctx, "c1", dm)
	if err != nil || rep.Name != "Ramparts fight" || len(rep.Lines) != 1 || rep.Link != "/somewhere" {
		t.Fatalf("report: %+v, %v", rep, err)
	}
	want := []EndedCreature{
		{EntityID: "gob", Name: "Goblin War Chief", Copies: 1, Down: 1},
		{EntityID: "hob", Name: "Hobgoblin Soldier", Copies: 2, Down: 1},
		{Name: "Skeleton Archer", Copies: 4},
	}
	if len(l.ended) != 1 || len(l.ended[0].Creatures) != len(want) {
		t.Fatalf("heard %+v", l.ended)
	}
	for i, w := range want {
		if got := l.ended[0].Creatures[i]; got != w {
			t.Errorf("creature %d = %+v, want %+v", i, got, w)
		}
	}
	// A second end finds no fight, so nothing drops twice.
	if rep, err := runs.End(ctx, "c1", dm); err != nil || len(rep.Lines) != 0 || len(l.ended) != 1 {
		t.Errorf("second end: %+v, %v, heard %d", rep, err, len(l.ended))
	}
}

func TestRunEndIsOverWhenTheListenerFails(t *testing.T) {
	runs, _, _, id := newRunTest(t)
	ctx := context.Background()
	runs.(*runService).SetFightListener(&fakeListener{fail: true})
	if _, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id}); err != nil {
		t.Fatalf("start: %v", err)
	}
	rep, err := runs.End(ctx, "c1", dm)
	if err != nil || len(rep.Lines) != 1 || !strings.Contains(rep.Lines[0], "Check the tray") {
		t.Fatalf("report: %+v, %v", rep, err)
	}
	if after, _ := runs.Current(ctx, "c1", dm); after.Run != nil {
		t.Error("the fight is still on")
	}
}

func TestRunCurrentUnlinksAGonePage(t *testing.T) {
	runs, _, pages, id := newRunTest(t)
	ctx := context.Background()
	if _, err := runs.Start(ctx, "c1", dm, RunStart{EncounterID: id}); err != nil {
		t.Fatalf("start: %v", err)
	}
	pages.hidden["gob"] = true
	view, err := runs.Current(ctx, "c1", dm)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if l := view.Run.Lines[2]; l.EntityID != "" || l.Name != "Goblin War Chief" {
		t.Errorf("gone page still linked: %+v", l)
	}
}

func TestValidateRun(t *testing.T) {
	line := func(mod func(*RunLine)) *Run {
		l := RunLine{ID: "a", Kind: LineCreature, Name: "Rat", HP: []*int{nil}}
		mod(&l)
		return &Run{Round: 1, Lines: []RunLine{l}}
	}
	many := make([]RunLine, MaxRunLines+1)
	for i := range many {
		many[i] = RunLine{ID: "l" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Kind: LineHero, Name: "H", HP: []*int{nil}}
	}
	tests := []struct {
		name string
		run  *Run
		ok   bool
	}{
		{"a plain line", line(func(*RunLine) {}), true},
		{"no run", nil, false},
		{"round 0", &Run{Round: 0}, false},
		{"bad id", line(func(l *RunLine) { l.ID = "a b" }), false},
		{"bad kind", line(func(l *RunLine) { l.Kind = "boss" }), false},
		{"no name", line(func(l *RunLine) { l.Name = " " }), false},
		{"bad group", line(func(l *RunLine) { l.Group = "z" }), false},
		{"lower group", line(func(l *RunLine) { l.Group = "b" }), true},
		{"initiative too high", line(func(l *RunLine) { l.Init = intp(MaxInit + 1) }), false},
		{"negative full hit points", line(func(l *RunLine) { l.Max = intp(-1) }), false},
		{"no boxes", line(func(l *RunLine) { l.HP = nil }), false},
		{"too many boxes", line(func(l *RunLine) { l.HP = make([]*int, MaxCreatureCount+1) }), false},
		{"a hero with two boxes", line(func(l *RunLine) { l.Kind = LineHero; l.HP = []*int{nil, nil} }), false},
		{"below zero hit points", line(func(l *RunLine) { l.HP = []*int{intp(-12)} }), true},
		{"too many conditions", line(func(l *RunLine) { l.Conds = make([]string, MaxRunConds+1) }), false},
		{"turn names no line", &Run{Round: 1, Turn: "zz"}, false},
		{"too many lines", &Run{Round: 1, Lines: many}, false},
		{"two lines share an id", &Run{Round: 1, Lines: []RunLine{
			{ID: "a", Kind: LineHero, Name: "A", HP: []*int{nil}}, {ID: "a", Kind: LineHero, Name: "B", HP: []*int{nil}},
		}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateRun(tt.run)
			if (err == nil) != tt.ok {
				t.Errorf("validateRun ok = %v, want %v (%v)", err == nil, tt.ok, err)
			}
		})
	}
}

func TestPlayedRowsFollowsThePickedVersion(t *testing.T) {
	e := &Encounter{
		Creatures: []Creature{{ID: "r1", Name: "Goblin", Count: 2}, {ID: "r2", Name: "Hobgoblin", Count: 1}},
		Versions: []Version{{
			ID: "v1", Remove: []string{"r2"}, Counts: []CountChange{{Row: "r1", Count: 5}},
			Add: []Creature{{ID: "a1", Name: "Ogre", Count: 1}},
		}},
	}
	if rows := playedRows(e); len(rows) != 2 || rows[0].Count != 2 {
		t.Errorf("no pick plays the base rows: %+v", rows)
	}
	e.Picked = &Picked{Version: "v1"}
	rows := playedRows(e)
	if len(rows) != 2 || rows[0].ID != "r1" || rows[0].Count != 5 || rows[1].ID != "a1" {
		t.Errorf("picked version: %+v", rows)
	}
	if e.Creatures[0].Count != 2 {
		t.Error("playedRows changed the encounter")
	}
	e.Picked = &Picked{Version: "gone"}
	if rows := playedRows(e); len(rows) != 2 || rows[1].ID != "r2" {
		t.Errorf("a missing version plays the base rows: %+v", rows)
	}
}

func TestRunMountCarriesOnlyConditionNames(t *testing.T) {
	var buf bytes.Buffer
	if err := RunMount("c1", []string{`Grappled`, `<b>"x"</b>`}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	for _, want := range []string{`data-widget="encounter-run"`, `data-campaign-id="c1"`, `data-conditions="[&#34;Grappled&#34;,`, "/static/css/encounters.css"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, "<b>") {
		t.Errorf("a condition name went in as markup: %s", html)
	}
	buf.Reset()
	if err := RunMount("c1", nil).Render(context.Background(), &buf); err != nil || !strings.Contains(buf.String(), `data-conditions="[]"`) {
		t.Errorf("no conditions: %s, %v", buf.String(), err)
	}
}
