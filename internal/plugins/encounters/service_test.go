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
	repo, pages := newMemRepo(), newFakePages()
	return NewService(repo, pages), repo, pages
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
