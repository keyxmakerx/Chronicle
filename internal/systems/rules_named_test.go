package systems

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestNamedAddress_PerReader(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}); err != nil {
		t.Fatal(err)
	}
	// A name at the cap in a script of three bytes a character.
	long := strings.Repeat("竜", maxEntryName)
	if _, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: long}); err != nil {
		t.Fatal(err)
	}
	const base = "/campaigns/c1/systems/sys/ancestries/"
	tests := []struct {
		name, kind, entry string
		reader            EntryActor
		want              string // "" = not found
	}{
		{"a printed name leads to the rewrite", "ancestry", "Elf", playerActor, base + "sea-elf"},
		{"the category slug works as the kind", "ancestries", "elf", playerActor, base + "sea-elf"},
		{"the campaign's Dwarf holds the plain address", "ancestry", "Dwarf", playerActor, base + "dwarf"},
		{"a left-out name answers at its own address", "ancestry", "Polder", playerActor, base + "polder"},
		{"a Directors-only entry for a player", "ancestry", "Tideling", playerActor, ""},
		{"a Directors-only entry for the Director", "ancestry", "Tideling", dirActor, base + "tideling"},
		{"an entry kept to another's hero", "ancestry", "Fairy", playerActor, ""},
		{"an entry kept to the reader's hero", "ancestry", "Fairy", tyneActor, base + "fairy"},
		{"a Directors-only category", "monsters", "Goblin", playerActor, ""},
		{"a field that is not a kind", "level", "3", playerActor, ""},
		{"no such kind", "nonsense", "Elf", playerActor, ""},
		{"a name too long", "ancestry", strings.Repeat("a", maxNamedLen+1), playerActor, ""},
		{"a name at the cap counts characters, not bytes", "ancestry", long, playerActor, base + "entry"},
		{"a name past the cap in characters", "ancestry", long + "竜", playerActor, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.NamedAddress(ctx, sys, tt.kind, tt.entry, reader(tt.reader))
			if tt.want == "" {
				if !isNotFound(err) {
					t.Fatalf("want not found, got %q %v", got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("address = %q %v, want %q", got, err, tt.want)
			}
		})
	}
	// The peek by name is the entry's own peek: a left-out people gives its
	// one line and its traits, marked, to the player whose hero is one.
	v, err := p.PeekNamed(ctx, sys, "ancestry", "Polder", "pip", reader(tyneActor))
	if err != nil || !v.LeftOut || v.LeftOutLine != "Polder is not a people of Therin. The halflings drowned." {
		t.Fatalf("Polder peek = %+v %v", v, err)
	}
	if v.Hero == nil || len(v.Hero.Traits) != 1 || !v.Hero.Traits[0].NoLongerOffered {
		t.Fatalf("Pip's traits = %+v", v.Hero)
	}
	// The Director opening it from Pip's sheet reads Pip's traits too; the
	// same peek without a hero of it draws only the line.
	v, err = p.PeekNamed(ctx, sys, "ancestry", "Polder", "pip", reader(dirActor))
	if err != nil || v.Hero == nil || len(v.Hero.Traits) != 1 || !v.Hero.Traits[0].NoLongerOffered {
		t.Fatalf("the Director's peek from Pip's sheet = %+v %v", v.Hero, err)
	}
	if v, err := p.PeekNamed(ctx, sys, "ancestry", "Polder", "gimli", reader(dirActor)); err != nil || v.Hero != nil || len(v.Table) != 0 {
		t.Fatalf("a hero of another people draws no traits: %+v %v", v, err)
	}
	if v, err := p.PeekNamed(ctx, sys, "ancestry", "Elf", "", reader(playerActor)); err != nil || v.Entry.Name != "Sea Elf" {
		t.Fatalf("Elf peek = %+v %v", v, err)
	}
}

func TestTold_PerReader(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	for _, name := range []string{"Ashborn", "Tideling"} {
		if _, err := svc.AddNote(ctx, "c1", dirActor, entryByName(t, repo, name).ID, "Changed."); err != nil {
			t.Fatal(err)
		}
	}
	refs := []NamedRef{{"ancestry", "Ashborn"}, {"ancestry", "Tideling"}, {"ancestry", "Dwarf"}, {"ancestry", "Nobody"}, {"monsters", "Goblin"}}
	tests := []struct {
		name   string
		reader EntryActor
		want   string
	}{
		{"a player is told of what they may read", playerActor, "ancestry:Ashborn"},
		{"the Director of both", dirActor, "ancestry:Ashborn,ancestry:Tideling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Told(ctx, sys, refs, reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			var words []string
			for _, r := range got {
				words = append(words, r.Kind+":"+r.Name)
			}
			if strings.Join(words, ",") != tt.want {
				t.Fatalf("told = %v, want %s", words, tt.want)
			}
		})
	}
}

// A sheet's question about many names reads the campaign's entries a fixed
// number of times: the names of a kind are resolved against one reading.
func TestTold_ReadsDoNotGrowWithNames(t *testing.T) {
	names := []string{"Ashborn", "Dwarf", "Elf", "Polder", "Devil", "Fairy", "Tideling", "Gnome", "Orc", "Emberkin"}
	tests := []struct {
		name string
		refs int
	}{
		{"one name", 1},
		{"twenty names", 20},
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
			e, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Ashborn"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AddNote(ctx, "c1", dirActor, e.ID, "Changed."); err != nil {
				t.Fatal(err)
			}
			p := NewRulesPages(svc, func(context.Context, string) *SystemManifest { return m }, therinPeople())
			var refs []NamedRef
			for n := 0; n < tt.refs; n++ {
				refs = append(refs, NamedRef{Kind: "ancestry", Name: fmt.Sprint(names[n%len(names)], strings.Repeat(" ", n/len(names)))})
			}
			repo.reads = 0
			got, err := p.Told(ctx, fakeSystem{m: m, data: data}, refs, reader(playerActor))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 || got[0].Name != "Ashborn" {
				t.Fatalf("told = %+v", got)
			}
			reads[i] = repo.reads
		})
	}
	if reads[1] != reads[0] {
		t.Fatalf("reads grew with names: %d for one, %d for twenty", reads[0], reads[1])
	}
}
