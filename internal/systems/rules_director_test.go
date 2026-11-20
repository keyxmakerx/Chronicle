package systems

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// The Director's amber words on an entry's page, for each kind of entry, and
// never for a player.
func TestRulesPage_DirectorTools(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}); err != nil {
		t.Fatal(err)
	}
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		reader  EntryActor
		slug    string
		printed bool
		setup   func(t *testing.T)
		want    string // kind/flags, "" for no tools
	}{
		{name: "a player never gets them", reader: playerActor, slug: "polder"},
		{name: "a player on Therin's own", reader: playerActor, slug: "ashborn"},
		{name: "the package's Dwarf beside Therin's", reader: dirActor, slug: "dwarf", printed: true, want: "standard rewritable same=Dwarf"},
		{name: "an entry that would lose part of itself", reader: dirActor, slug: "devil", want: "standard"},
		{name: "a left-out entry", reader: dirActor, slug: "polder", want: "left_out sentence"},
		{name: "a rewritten draft", reader: dirActor, slug: "sea-elf", want: "rewritten pen"},
		{name: "a rewritten entry in play", reader: dirActor, slug: "sea-elf", setup: func(t *testing.T) {
			if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
				t.Fatal(err)
			}
		}, want: "rewritten pen aside who=Everyone in Therin reads this Sea Elf."},
		{name: "the printed original", reader: dirActor, slug: "elf", printed: true},
		{name: "Therin's own", reader: dirActor, slug: "ashborn", want: "campaign pen"},
		{name: "a category that is not a kind", reader: dirActor, slug: "strike"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			cat := "ancestries"
			if tt.slug == "strike" {
				cat = "abilities"
			}
			v, err := p.Page(ctx, sys, cat, tt.slug, tt.printed, reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if tl := v.Tools; tl != nil {
				w := []string{tl.Kind}
				if tl.Rewritable {
					w = append(w, "rewritable")
				}
				if tl.SameName != nil {
					w = append(w, "same="+tl.SameName.Name)
				}
				if tl.Sentence != "" {
					w = append(w, "sentence")
				}
				if tl.EditURL != "" {
					w = append(w, "pen")
					if !strings.Contains(tl.EditURL, "return=") {
						t.Fatalf("the pen has no way back: %s", tl.EditURL)
					}
				}
				if tl.CanSetAside {
					w = append(w, "aside")
				}
				if tl.WhoReads != "" {
					w = append(w, "who="+tl.WhoReads)
				}
				got = strings.Join(w, " ")
			}
			if got != tt.want {
				t.Fatalf("tools = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDirectorTools_HeroLines(t *testing.T) {
	tests := []struct {
		heroes []string
		more   int
		notIn  string
		aside  string
	}{
		{nil, 0, "No hero in Therin is an Elf.", "No hero is an Elf yet."},
		{[]string{"Wren"}, 0, "Wren is an Elf and keeps reading its traits, marked no longer offered.", "Wren goes back to the printed Elf."},
		{[]string{"Wren", "Pip"}, 0, "Wren and Pip are Elf and keep reading its traits, marked no longer offered.", "Wren and Pip go back to the printed Elf."},
		{[]string{"Wren"}, 2, "Wren and 2 more are Elf and keep reading its traits, marked no longer offered.", "Wren and 2 more go back to the printed Elf."},
	}
	for _, tt := range tests {
		tl := &directorTools{CampaignName: "Therin", Name: "Elf", PrintedName: "Elf", Heroes: tt.heroes, MoreHeroes: tt.more}
		if got := tl.NotInHeroes(); got != tt.notIn {
			t.Errorf("not in = %q, want %q", got, tt.notIn)
		}
		if got := tl.AsideHeroes(); got != tt.aside {
			t.Errorf("aside = %q, want %q", got, tt.aside)
		}
	}
}
