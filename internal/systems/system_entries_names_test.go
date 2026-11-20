package systems

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// A refusal over an old name must not name an entry the writer may not see:
// a player gets the plain "taken", whatever path they write by.
func TestRefuseOldName_NeverNamesAHiddenEntry(t *testing.T) {
	tests := []struct {
		name     string
		actor    EntryActor
		rename   bool // write by renaming a draft rather than creating
		taken    string
		wantSay  string
		wantHide string
	}{
		{"player creating over a hidden entry's old name", playerActor, false, "Tideling", "That name is already taken.", "Sea Folk"},
		{"player renaming onto a hidden entry's old name", playerActor, true, "Tideling", "That name is already taken.", "Sea Folk"},
		{"player creating over a visible entry's old name", playerActor, false, "Ashborn", "still leads there", ""},
		{"Director creating over a hidden entry's old name", dirActor, false, "Tideling", "Sea Folk", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := newReadingSvc(t)
			ctx := context.Background()
			for from, to := range map[string]string{"Tideling": "Sea Folk", "Ashborn": "Emberkin"} {
				e := entryByName(t, repo, from)
				if _, err := svc.Update(ctx, "c1", dirActor, e.ID, UpdateSystemEntryInput{Name: patch.Of(to)}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 3}); err != nil {
				t.Fatal(err)
			}
			var err error
			if tt.rename {
				d, cerr := svc.Create(ctx, "c1", tt.actor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Pixie"})
				if cerr != nil {
					t.Fatal(cerr)
				}
				_, err = svc.Update(ctx, "c1", tt.actor, d.ID, UpdateSystemEntryInput{Name: patch.Of(tt.taken)})
			} else {
				_, err = svc.Create(ctx, "c1", tt.actor, CreateSystemEntryInput{FieldKey: "ancestry", Name: tt.taken})
			}
			wantCode(t, err, 409)
			if !strings.Contains(err.Error(), tt.wantSay) {
				t.Fatalf("refusal %q does not say %q", err, tt.wantSay)
			}
			if tt.wantHide != "" && strings.Contains(err.Error(), tt.wantHide) {
				t.Fatalf("refusal %q names the hidden entry", err)
			}
		})
	}
}

// Only an accepted entry leaves its old name behind; a draft or sent one is
// nobody else's yet.
func TestRecordRename_OnlyAccepted(t *testing.T) {
	tests := []struct {
		name     string
		status   string // the entry's status before the rename
		accept   bool   // the Director accepts it in the same save
		renamer  EntryActor
		wantKept bool
	}{
		{"accepted entry renamed", EntryStatusAccepted, false, dirActor, true},
		{"draft renamed by its writer", EntryStatusDraft, false, playerActor, false},
		{"sent entry renamed by the Director", EntryStatusSent, false, dirActor, false},
		{"sent entry renamed as it is accepted", EntryStatusSent, true, dirActor, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := newReadingSvc(t)
			ctx := context.Background()
			if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 3}); err != nil {
				t.Fatal(err)
			}
			e, err := svc.Create(ctx, "c1", playerActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Pixie"})
			if err != nil {
				t.Fatal(err)
			}
			for i := range repo.rows {
				if repo.rows[i].ID == e.ID {
					repo.rows[i].Status = tt.status
				}
			}
			in := UpdateSystemEntryInput{Name: patch.Of("Sprite")}
			if tt.accept {
				in.Status = patch.Of(EntryStatusAccepted)
			}
			if _, err := svc.Update(ctx, "c1", tt.renamer, e.ID, in); err != nil {
				t.Fatal(err)
			}
			o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", "Pixie")
			if (o != nil) != tt.wantKept {
				t.Fatalf("old name kept = %v, want %v", o != nil, tt.wantKept)
			}
		})
	}
}

// A much-renamed entry keeps its last maxOldNamesPerEntry old names; the
// oldest are let go and free again.
func TestRecordRename_Cap(t *testing.T) {
	tests := []struct {
		renames  int
		wantKept int
	}{
		{maxOldNamesPerEntry, maxOldNamesPerEntry},
		{maxOldNamesPerEntry + 5, maxOldNamesPerEntry},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.renames), func(t *testing.T) {
			svc, repo, _ := newReadingSvc(t)
			ctx := context.Background()
			ash := entryByName(t, repo, "Ashborn")
			given := []string{"Ashborn"}
			for i := 1; i <= tt.renames; i++ {
				n := fmt.Sprintf("Ash %d", i)
				if _, err := svc.Update(ctx, "c1", dirActor, ash.ID, UpdateSystemEntryInput{Name: patch.Of(n)}); err != nil {
					t.Fatal(err)
				}
				if i < tt.renames {
					given = append(given, n)
				}
			}
			names, _ := svc.OldNames(ctx, "c1", "ancestry", dirActor)
			if len(names) != tt.wantKept {
				t.Fatalf("kept %d old names, want %d", len(names), tt.wantKept)
			}
			dropped := given[:len(given)-tt.wantKept]
			for _, n := range dropped {
				if o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", n); o != nil {
					t.Fatalf("%s should have been let go", n)
				}
			}
			for _, n := range given[len(given)-tt.wantKept:] {
				if o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", n); o == nil {
					t.Fatalf("%s should be kept", n)
				}
			}
		})
	}
}

// A renamed campaign entry keeps the heroes made under its old name even
// when the package prints an entry of that name, for readers who may see it.
func TestResolveName_OldNameBeatsPackage(t *testing.T) {
	svc, repo, _ := newReadingSvc(t)
	ctx := context.Background()
	d := entryByName(t, repo, "Dwarf")
	if _, err := svc.Update(ctx, "c1", dirActor, d.ID, UpdateSystemEntryInput{Name: patch.Of("Stoneborn")}); err != nil {
		t.Fatal(err)
	}
	pol, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Polder", Visibility: EntryVisibilityDirectors})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, pol.ID, UpdateSystemEntryInput{Name: patch.Of("Hillfolk")}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		reader     EntryActor
		query      string
		wantKind   string
		wantVia    string
		wantOrigin string
	}{
		{"a player follows the old name", playerActor, "Dwarf", ResolvedRedirect, ViaOldName, OriginCampaign},
		{"a stranger follows the old name", stranger, "dwarf", ResolvedRedirect, ViaOldName, OriginCampaign},
		{"a hidden entry's old name gives the package's", playerActor, "Polder", ResolvedEntry, "", OriginStandard},
		{"the Director follows the hidden entry's old name", dirActor, "Polder", ResolvedRedirect, ViaOldName, OriginCampaign},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := svc.ResolveName(ctx, "c1", "ancestries", tt.query, tt.reader)
			if err != nil {
				t.Fatal(err)
			}
			if r.Kind != tt.wantKind || r.Via != tt.wantVia || r.Entry == nil || r.Entry.Origin != tt.wantOrigin {
				t.Fatalf("got %+v", r)
			}
		})
	}
	// Freeing the old name gives the name back to the package.
	o, _ := repo.FindOldName(ctx, "c1", "sys", "ancestry", "Dwarf")
	if err := svc.FreeOldName(ctx, "c1", dirActor, o.ID); err != nil {
		t.Fatal(err)
	}
	r, _ := svc.ResolveName(ctx, "c1", "ancestries", "Dwarf", playerActor)
	if r.Kind != ResolvedEntry || r.Entry.Origin != OriginStandard {
		t.Fatalf("after freeing: %+v", r)
	}
}

// countingRepo counts the reads a page makes, so a test can hold them flat
// however many characters the page lists.
type countingRepo struct {
	*fakeEntryRepo
	reads int
}

func (c *countingRepo) List(ctx context.Context, camp, sys, field string, dirs bool) ([]SystemEntry, error) {
	c.reads++
	return c.fakeEntryRepo.List(ctx, camp, sys, field, dirs)
}

func (c *countingRepo) ListOldNames(ctx context.Context, camp, sys, field string) ([]EntryOldName, error) {
	c.reads++
	return c.fakeEntryRepo.ListOldNames(ctx, camp, sys, field)
}

func (c *countingRepo) ListLeftOut(ctx context.Context, camp, sys, field string) ([]LeftOut, error) {
	c.reads++
	return c.fakeEntryRepo.ListLeftOut(ctx, camp, sys, field)
}

func (c *countingRepo) FindOldName(ctx context.Context, camp, sys, field, name string) (*EntryOldName, error) {
	c.reads++
	return c.fakeEntryRepo.FindOldName(ctx, camp, sys, field, name)
}

// The entry page reads the campaign's entries a fixed number of times, not
// once per character: names are resolved against one reading.
func TestRulesPage_ReadsDoNotGrowWithCharacters(t *testing.T) {
	values := []string{"Ashborn", "Dwarf", "Elf", "Polder", "Devil", "Fairy", "Tideling", "Gnome", "Orc", "Emberkin"}
	tests := []struct {
		name     string
		people   int
		distinct int
	}{
		{"one character", 1, 1},
		{"fifty characters of ten stored values", 50, 10},
	}
	reads := make([]int, len(tests))
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := readingManifest()
			data := readingData()
			repo := &countingRepo{fakeEntryRepo: &fakeEntryRepo{}}
			svc := NewSystemEntryService(repo, func(context.Context, string) *SystemManifest { return m },
				WithEntryCharacters(testCharacters),
				WithStandardSystem(func(context.Context, string) System { return fakeSystem{m: m, data: data} }),
			).(*systemEntryService)
			ctx := context.Background()
			if _, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Ashborn"}); err != nil {
				t.Fatal(err)
			}
			var people fakePeople
			for n := 0; n < tt.people; n++ {
				people.people = append(people.people, fakePerson{
					EntryPerson: EntryPerson{ID: fmt.Sprint("p", n), Name: fmt.Sprint("Person ", n), Value: values[n%tt.distinct]},
					field:       "ancestry"})
			}
			p := NewRulesPages(svc, func(context.Context, string) *SystemManifest { return m }, people)
			repo.reads = 0
			v, err := p.Page(ctx, fakeSystem{m: m, data: data}, "ancestries", "ashborn", false, reader(playerActor))
			if err != nil {
				t.Fatal(err)
			}
			if v.InCampaign == nil {
				t.Fatal("no In the campaign")
			}
			reads[i] = repo.reads
			t.Logf("%d characters, %d stored values: %d reads", tt.people, tt.distinct, repo.reads)
		})
	}
	if reads[1] != reads[0] {
		t.Fatalf("reads grew with characters: %d for one, %d for fifty", reads[0], reads[1])
	}
}
