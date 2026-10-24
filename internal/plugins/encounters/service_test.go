package encounters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	dm     = Viewer{UserID: "u-dm", VisibilityRole: permissions.RoleOwner}
	scribe = Viewer{UserID: "u-scribe", VisibilityRole: permissions.RoleScribe}
	player = Viewer{UserID: "u-player", VisibilityRole: permissions.RolePlayer}
)

func newTestService() (Service, *memRepo, *fakePages) {
	svc, repo, pages, _ := newVersionService()
	return svc, repo, pages
}

func newVersionService() (Service, *memRepo, *fakePages, *fakeResults) {
	repo, pages, results := newMemRepo(), newFakePages(), newFakeResults()
	return NewService(repo, pages, results), repo, pages, results
}

func intp(n int) *int { return &n }

func statusOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func ramparts() SaveInput {
	return SaveInput{
		Name:   "Ramparts fight",
		Heroes: []string{"hero1", "hero2", "hero1"},
		Creatures: []Creature{
			{ID: "r1", EntityID: "gob", Name: "Goblin War Chief", Count: 1, Group: "a", Start: StartHidden},
			{ID: "r2", EntityID: "hob", Name: "Hobgoblin Soldier", Count: 2, Group: "B"},
			{ID: "r3", Name: "Skeleton Archer", Count: 4, Group: "C", Start: StartLater},
		},
	}
}

func TestCreateAndRead(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, ramparts())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if e.Version != 1 || e.Page.Name != "The Siege of Harrowmere" {
		t.Fatalf("unexpected encounter: %+v", e)
	}
	if len(e.Heroes) != 2 {
		t.Errorf("duplicate hero kept: %+v", e.Heroes)
	}
	if c := e.Creatures[0]; c.Group != "A" || c.Start != StartHidden {
		t.Errorf("row not cleaned: %+v", c)
	}
	if c := e.Creatures[1]; c.Start != StartSeen {
		t.Errorf("missing start should read as seen: %+v", c)
	}
	list, err := svc.ListForPage(ctx, "c1", "page1", dm)
	if err != nil || len(list.Encounters) != 1 || !list.CanEdit {
		t.Fatalf("list: %+v %v", list, err)
	}
	got, err := svc.Get(ctx, "c1", e.ID, dm)
	if err != nil || got.Name != "Ramparts fight" {
		t.Fatalf("get: %+v %v", got, err)
	}
}

// Encounters are the DM's plans: nobody below Owner visibility reads or
// writes one, whatever the page allows.
func TestOnlyTheDM(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, ramparts())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []Viewer{scribe, player, {}} {
		if _, err := svc.ListForPage(ctx, "c1", "page1", v); statusOf(err) != 403 {
			t.Errorf("list as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Get(ctx, "c1", e.ID, v); statusOf(err) != 403 {
			t.Errorf("get as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Create(ctx, "c1", "page1", v, ramparts()); statusOf(err) != 403 {
			t.Errorf("create as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, err := svc.Update(ctx, "c1", e.ID, v, SaveInput{Name: "x", Version: intp(1)}); statusOf(err) != 403 {
			t.Errorf("update as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if err := svc.Delete(ctx, "c1", e.ID, v); statusOf(err) != 403 {
			t.Errorf("delete as %d: want 403, got %v", v.VisibilityRole, err)
		}
		if _, _, err := svc.Search(ctx, "c1", "", 0, v); statusOf(err) != 403 {
			t.Errorf("search as %d: want 403, got %v", v.VisibilityRole, err)
		}
	}
}

func TestPagesMustBeInTheCampaign(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	tests := []struct {
		name   string
		page   string
		mutate func(*SaveInput)
		want   int
	}{
		{"unknown page", "nope", nil, 404},
		{"hidden page reads as missing", "secret", nil, 404},
		{"hero from elsewhere", "page1", func(in *SaveInput) { in.Heroes = []string{"other-campaign-hero"} }, 422},
		{"creature from elsewhere", "page1", func(in *SaveInput) { in.Creatures[0].EntityID = "other" }, 422},
		{"hidden creature page", "page1", func(in *SaveInput) { in.Creatures[0].EntityID = "secret" }, 422},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := ramparts()
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			_, err := svc.Create(ctx, "c1", tc.page, dm, in)
			if got := statusOf(err); got != tc.want {
				t.Fatalf("want %d, got %d (%v)", tc.want, got, err)
			}
		})
	}
	if _, err := svc.Create(ctx, "c2", "page1", dm, ramparts()); statusOf(err) != 404 {
		t.Fatalf("a page of another campaign must read as missing, got %v", err)
	}
}

func TestValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*SaveInput)
	}{
		{"blank name", func(in *SaveInput) { in.Name = "   " }},
		{"long name", func(in *SaveInput) { in.Name = strings.Repeat("x", MaxNameChars+1) }},
		{"control character", func(in *SaveInput) { in.Name = "a\x00b" }},
		{"zero count", func(in *SaveInput) { in.Creatures[0].Count = 0 }},
		{"too many", func(in *SaveInput) { in.Creatures[0].Count = MaxCreatureCount + 1 }},
		{"bad group", func(in *SaveInput) { in.Creatures[0].Group = "Z" }},
		{"bad start", func(in *SaveInput) { in.Creatures[0].Start = "revealed" }},
		{"duplicate row id", func(in *SaveInput) { in.Creatures[1].ID = "r1" }},
		{"row id with markup", func(in *SaveInput) { in.Creatures[0].ID = "<b>" }},
		{"nameless row", func(in *SaveInput) { in.Creatures[0].Name = "" }},
		{"too many rows", func(in *SaveInput) {
			in.Creatures = nil
			for i := 0; i <= MaxCreatureRows; i++ {
				in.Creatures = append(in.Creatures, Creature{ID: "r" + strings.Repeat("x", i+1), Name: "Rat", Count: 1})
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := ramparts()
			tc.mutate(&in)
			if _, err := svc.Create(ctx, "c1", "page1", dm, in); statusOf(err) != 422 {
				t.Fatalf("want 422, got %v", err)
			}
		})
	}
}

func TestUpdateVersionAndAccess(t *testing.T) {
	svc, _, pages := newTestService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, ramparts())
	if err != nil {
		t.Fatal(err)
	}
	in := ramparts()
	in.Ready = true
	if _, err := svc.Update(ctx, "c1", e.ID, dm, in); statusOf(err) != 422 {
		t.Fatalf("missing version: want 422, got %v", err)
	}
	in.Version = intp(5)
	if _, err := svc.Update(ctx, "c1", e.ID, dm, in); statusOf(err) != 409 {
		t.Fatalf("stale version: want 409, got %v", err)
	}
	in.Version = intp(1)
	got, err := svc.Update(ctx, "c1", e.ID, dm, in)
	if err != nil || !got.Ready || got.Version != 2 {
		t.Fatalf("update: %+v %v", got, err)
	}
	if _, err := svc.Update(ctx, "c2", e.ID, dm, in); statusOf(err) != 404 {
		t.Fatalf("another campaign's id must read as missing, got %v", err)
	}
	pages.readOnly["page1"] = true
	in.Version = intp(2)
	if _, err := svc.Update(ctx, "c1", e.ID, dm, in); statusOf(err) != 403 {
		t.Fatalf("read-only page: want 403, got %v", err)
	}
	pages.readOnly["page1"] = false
	pages.hidden["page1"] = true
	if _, err := svc.Get(ctx, "c1", e.ID, dm); statusOf(err) != 404 {
		t.Fatalf("an encounter on a hidden page reads as missing, got %v", err)
	}
}

// A hero or creature page removed after the save is dropped from the heroes
// and unlinked from its row, which keeps its name.
func TestGonePagesOnRead(t *testing.T) {
	svc, _, pages := newTestService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, ramparts())
	if err != nil {
		t.Fatal(err)
	}
	delete(pages.names, "hero2")
	delete(pages.names, "gob")
	got, err := svc.Get(ctx, "c1", e.ID, dm)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Heroes) != 1 || got.Heroes[0].ID != "hero1" {
		t.Errorf("heroes: %+v", got.Heroes)
	}
	if c := got.Creatures[0]; c.EntityID != "" || c.Name != "Goblin War Chief" {
		t.Errorf("row: %+v", c)
	}
}

func TestDelete(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, ramparts())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "c2", e.ID, dm); statusOf(err) != 404 {
		t.Fatalf("another campaign: want 404, got %v", err)
	}
	if err := svc.Delete(ctx, "c1", e.ID, dm); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 0 {
		t.Fatal("row not deleted")
	}
}

func TestPerPageLimit(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	for i := 0; i < MaxPerPage; i++ {
		if _, err := svc.Create(ctx, "c1", "page1", dm, SaveInput{Name: "Fight"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Create(ctx, "c1", "page1", dm, SaveInput{Name: "One more"}); statusOf(err) != 422 {
		t.Fatalf("want 422, got %v", err)
	}
	if _, err := svc.Create(ctx, "c1", "page2", dm, SaveInput{Name: "Elsewhere"}); err != nil {
		t.Fatalf("another page still takes one: %v", err)
	}
}

func TestSearchReadyFirstAndHiddenPages(t *testing.T) {
	svc, _, pages := newTestService()
	ctx := context.Background()
	a, _ := svc.Create(ctx, "c1", "page1", dm, SaveInput{Name: "Old ready", Ready: true})
	_, _ = svc.Create(ctx, "c1", "page1", dm, ramparts())
	_, _ = svc.Create(ctx, "c1", "page2", dm, SaveInput{Name: "Bandits"})

	items, total, err := svc.Search(ctx, "c1", "", 0, dm)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("search: %d %d %v", total, len(items), err)
	}
	if items[0].ID != a.ID || !items[0].Ready {
		t.Errorf("ready first: %+v", items[0])
	}
	for _, it := range items {
		if it.Name == "Ramparts fight" && it.CreatureCount != 7 {
			t.Errorf("creature count: %+v", it)
		}
	}
	items, total, _ = svc.Search(ctx, "c1", "band", 0, dm)
	if total != 1 || items[0].Name != "Bandits" {
		t.Errorf("q: %+v", items)
	}
	pages.hidden["page2"] = true
	_, total, _ = svc.Search(ctx, "c1", "", 0, dm)
	if total != 2 {
		t.Errorf("a hidden page's encounters are left out, total %d", total)
	}
	if _, _, err := svc.Search(ctx, "c1", "a\x00", 0, dm); statusOf(err) != 422 {
		t.Errorf("bad search text: %v", err)
	}
}

// queenStage is the maintainer's example: at half Stamina the Chief takes a
// form and the later row arrives.
func queenStage() Stage {
	return Stage{
		ID:     "s1",
		When:   StageWhen{Kind: WhenStamina, Row: "r1", Percent: 50},
		Forms:  []StageForm{{Row: "r1", Form: "unmasked", FormName: "Unmasked"}},
		Arrive: []string{"r3"},
	}
}

func withStages(st ...Stage) SaveInput {
	in := ramparts()
	in.Stages = st
	return in
}

func TestStageValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*Stage)
		extra  func(*SaveInput)
	}{
		{"unknown kind", func(s *Stage) { s.When.Kind = "daily" }, nil},
		{"empty kind", func(s *Stage) { s.When.Kind = "" }, nil},
		{"stamina row missing", func(s *Stage) { s.When.Row = "nope" }, nil},
		{"stamina row empty", func(s *Stage) { s.When.Row = "" }, nil},
		{"stamina percent 0", func(s *Stage) { s.When.Percent = 0 }, nil},
		{"stamina percent 100", func(s *Stage) { s.When.Percent = 100 }, nil},
		{"round 1", func(s *Stage) { s.When = StageWhen{Kind: WhenRound, Round: 1} }, nil},
		{"round 100", func(s *Stage) { s.When = StageWhen{Kind: WhenRound, Round: 100} }, nil},
		{"bad stage id", func(s *Stage) { s.ID = "<b>" }, nil},
		{"form row missing", func(s *Stage) { s.Forms[0].Row = "nope" }, nil},
		{"form row has no page", func(s *Stage) { s.Forms[0].Row = "r3" }, nil},
		{"form id upper case", func(s *Stage) { s.Forms[0].Form = "Unmasked" }, nil},
		{"form id too long", func(s *Stage) { s.Forms[0].Form = strings.Repeat("a", 25) }, nil},
		{"form id with markup", func(s *Stage) { s.Forms[0].Form = "a<b" }, nil},
		{"form name too long", func(s *Stage) { s.Forms[0].FormName = strings.Repeat("x", MaxFormNameChars+1) }, nil},
		{"form name control char", func(s *Stage) { s.Forms[0].FormName = "a\x00" }, nil},
		{"too many forms", func(s *Stage) {
			s.Forms = nil
			for i := 0; i <= MaxStageForms; i++ {
				s.Forms = append(s.Forms, StageForm{Row: "r1", Form: "a"})
			}
		}, nil},
		{"arrive row missing", func(s *Stage) { s.Arrive = []string{"nope"} }, nil},
		{"arrive duplicate", func(s *Stage) { s.Arrive = []string{"r3", "r3"} }, nil},
		{"too many arrivals", func(s *Stage) {
			s.Arrive = nil
			for i := 0; i <= MaxStageArrive; i++ {
				s.Arrive = append(s.Arrive, "r3")
			}
		}, nil},
		{"bad how", func(s *Stage) { s.How = "loudly" }, nil},
		{"note too long", func(s *Stage) { s.Note = strings.Repeat("x", MaxNoteChars+1) }, nil},
		{"note control char", func(s *Stage) { s.Note = "a\x07b" }, nil},
		{"does nothing", func(s *Stage) { s.Forms, s.Arrive = nil, nil }, nil},
		{"duplicate stage id", nil, func(in *SaveInput) { in.Stages = append(in.Stages, in.Stages[0]) }},
		{"too many stages", nil, func(in *SaveInput) {
			in.Stages = nil
			for i := 0; i <= MaxStages; i++ {
				st := queenStage()
				st.ID = "s" + strings.Repeat("x", i+1)
				in.Stages = append(in.Stages, st)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := queenStage()
			if tc.mutate != nil {
				tc.mutate(&st)
			}
			in := withStages(st)
			if tc.extra != nil {
				tc.extra(&in)
			}
			if _, err := svc.Create(ctx, "c1", "page1", dm, in); statusOf(err) != 422 {
				t.Fatalf("want 422, got %v", err)
			}
		})
	}
}

func TestStagesSaveAndClean(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	tests := []struct {
		name  string
		stage Stage
		check func(*testing.T, Stage)
	}{
		{"how defaults to reveal", queenStage(), func(t *testing.T, s Stage) {
			if s.How != HowReveal {
				t.Errorf("how: %q", s.How)
			}
		}},
		{"quiet is kept", func() Stage { s := queenStage(); s.How = HowQuiet; return s }(), func(t *testing.T, s Stage) {
			if s.How != HowQuiet {
				t.Errorf("how: %q", s.How)
			}
		}},
		{"first form is the empty id", func() Stage {
			s := queenStage()
			s.Forms = []StageForm{{Row: "r1", Form: "", FormName: "Its first form"}}
			return s
		}(), func(t *testing.T, s Stage) {
			if s.Forms[0].Form != "" {
				t.Errorf("forms: %+v", s.Forms)
			}
		}},
		{"manual drops stray fields", func() Stage {
			s := queenStage()
			s.When = StageWhen{Kind: WhenManual, Row: "r1", Percent: 50, Round: 3}
			return s
		}(), func(t *testing.T, s Stage) {
			if s.When != (StageWhen{Kind: WhenManual}) {
				t.Errorf("when: %+v", s.When)
			}
		}},
		{"round", func() Stage {
			s := queenStage()
			s.When = StageWhen{Kind: WhenRound, Round: 3, Row: "r1", Percent: 10}
			return s
		}(), func(t *testing.T, s Stage) {
			if s.When != (StageWhen{Kind: WhenRound, Round: 3}) {
				t.Errorf("when: %+v", s.When)
			}
		}},
		{"note is trimmed, arrival only is enough", func() Stage {
			s := queenStage()
			s.Forms, s.Note = nil, "  Bring the lights down  "
			return s
		}(), func(t *testing.T, s Stage) {
			if s.Note != "Bring the lights down" || len(s.Forms) != 0 {
				t.Errorf("stage: %+v", s)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, err := svc.Create(ctx, "c1", "page1", dm, withStages(tc.stage))
			if err != nil {
				t.Fatal(err)
			}
			if len(e.Stages) != 1 {
				t.Fatalf("stages: %+v", e.Stages)
			}
			tc.check(t, e.Stages[0])
		})
	}
}

// An encounter saved before stages existed has no "stages" member and reads
// as having none, as a list rather than null.
func TestOldDocumentReadsWithNoStages(t *testing.T) {
	svc, repo, _ := newTestService()
	ctx := context.Background()
	repo.rows["old"] = Row{ID: "old", CampaignID: "c1", EntityID: "page1", Name: "Old", Version: 1,
		Data: []byte(`{"heroes":[],"creatures":[{"id":"r1","entityId":"","name":"Rat","count":1,"group":"A","start":"seen"}]}`)}
	got, err := svc.Get(ctx, "c1", "old", dm)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stages == nil || len(got.Stages) != 0 {
		t.Fatalf("stages: %#v", got.Stages)
	}
}

func TestStagesOnReadDropGoneReferences(t *testing.T) {
	svc, repo, pages := newTestService()
	ctx := context.Background()
	st := queenStage()
	st.Forms = append(st.Forms, StageForm{Row: "r2", Form: "", FormName: "Its first form"})
	e, err := svc.Create(ctx, "c1", "page1", dm, withStages(st))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("an unlinked row loses its form change, keeps its arrival", func(t *testing.T) {
		delete(pages.names, "hob")
		got, err := svc.Get(ctx, "c1", e.ID, dm)
		if err != nil {
			t.Fatal(err)
		}
		s := got.Stages[0]
		if len(s.Forms) != 1 || s.Forms[0].Row != "r1" || len(s.Arrive) != 1 {
			t.Errorf("stage: %+v", s)
		}
		pages.names["hob"] = "Hobgoblin Soldier"
	})

	// Rows that vanish from the stored document stand in for any way a row
	// can be gone by the time the stage is read.
	t.Run("a missing row is dropped and a Stamina trigger on it becomes manual", func(t *testing.T) {
		row := repo.rows[e.ID]
		row.Data = []byte(`{"heroes":[],"creatures":[{"id":"r2","entityId":"hob","name":"Hob","count":1,"group":"A","start":"seen"}],` +
			`"stages":[{"id":"s1","when":{"kind":"stamina","row":"r1","percent":50},` +
			`"forms":[{"row":"r1","form":"x","formName":"X"}],"arrive":["r1","r2"],"how":"reveal","note":"n"}]}`)
		repo.rows[e.ID] = row
		got, err := svc.Get(ctx, "c1", e.ID, dm)
		if err != nil {
			t.Fatal(err)
		}
		s := got.Stages[0]
		if s.When.Kind != WhenManual || s.When.Row != "" || s.When.Percent != 0 {
			t.Errorf("when: %+v", s.When)
		}
		if len(s.Forms) != 0 || len(s.Arrive) != 1 || s.Arrive[0] != "r2" || s.Note != "n" {
			t.Errorf("stage: %+v", s)
		}
	})
}

// The page list carries the campaign's picker types; when they can't be read
// the list still loads, with empty pickers rather than an error.
func TestListForPagePickerTypes(t *testing.T) {
	cases := []struct {
		name          string
		pickerErr     error
		wantHeroes    int
		wantCreatures int
	}{
		{"types offered", nil, 2, 2},
		{"types unavailable", errors.New("types down"), 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, pages := newTestService()
			pages.pickerErr = tc.pickerErr
			list, err := svc.ListForPage(context.Background(), "c1", "page1", dm)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(list.HeroTypeIDs) != tc.wantHeroes || len(list.CreatureTypeIDs) != tc.wantCreatures {
				t.Errorf("got heroes %v creatures %v", list.HeroTypeIDs, list.CreatureTypeIDs)
			}
			if list.HeroTypeIDs == nil || list.CreatureTypeIDs == nil {
				t.Errorf("lists must encode as [], not null")
			}
		})
	}
}
