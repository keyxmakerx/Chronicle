package systems

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// readingBook is a book with a rules-index chapter of ancestries, as the
// package lays it out, and a written chapter whose cards name their entries.
func readingBook() *Book {
	return &Book{Parts: []BookPart{{Title: "Heroes", Chapters: []BookChapter{
		{ID: "peoples-index", Title: "Peoples", Generated: true, Pages: []BookPage{{Title: "Peoples",
			Blocks: []BookBlock{{Type: "index", Category: "ancestries", Count: 4}}}}},
		{ID: "peoples", Title: "Ancestries", Pages: []BookPage{
			{Title: "Part one", Blocks: []BookBlock{{Type: "cards", Items: []BookItem{
				{Title: "Devil", Summary: "Horned.", Entry: "ancestries/devil"},
				{Title: "Dwarf", Summary: "Stout.", Entry: "ancestries/dwarf"},
				{Title: "Elf", Summary: "Printed elf words.", Entry: "ancestries/elf"},
				{Title: "Polder", Summary: "Small.", Entry: "ancestries/polder"},
				{Title: "A card about nothing", Summary: "Stays."},
			}}}},
			{Title: "Part two", Blocks: []BookBlock{{Type: "text", Text: "More."}}},
		}},
	}}}}
}

func cardTitles(b *Book) string {
	var out []string
	for _, p := range b.Parts[0].Chapters[1].Pages {
		for _, blk := range p.Blocks {
			for _, it := range blk.Items {
				w := it.Title
				if strings.HasPrefix(it.Summary, rewrittenWord) {
					w += "[Rewritten]"
				}
				out = append(out, w)
			}
		}
	}
	return strings.Join(out, ",")
}

func TestReaderBook_PerReader(t *testing.T) {
	p, svc, _, sys := newRulesPages(t)
	ctx := context.Background()
	elf, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf", Name: "Sea Elf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, elf.ID, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []LeftOutInput{{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}, {FieldKey: "ancestry", Slug: "devil"}} {
		if _, err := svc.LeaveOut(ctx, "c1", dirActor, in); err != nil {
			t.Fatal(err)
		}
	}
	const polderLine = "Polder is not a people of Therin. The halflings drowned."
	tests := []struct {
		name   string
		reader EntryActor
		index  string // the index chapter's entries, in order
		cards  string // the written chapter's cards
	}{
		{"a player", playerActor, "Ashborn,Dwarf,Dwarf,Sea Elf[Rewritten]",
			"Dwarf,Sea Elf[Rewritten],A card about nothing,Ashborn,Dwarf"},
		{"Wren's player has the Fairy kept to Wren", tyneActor, "Ashborn,Dwarf,Dwarf,Fairy,Sea Elf[Rewritten]",
			"Dwarf,Sea Elf[Rewritten],A card about nothing,Ashborn,Dwarf,Fairy"},
		{"the Director has the Directors-only Tideling", dirActor, "Ashborn,Dwarf,Dwarf,Fairy,Sea Elf[Rewritten],Tideling",
			"Dwarf,Sea Elf[Rewritten],A card about nothing,Ashborn,Dwarf,Fairy,Tideling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := reader(tt.reader)
			kinds := func(cat string) (*CatalogList, error) { return p.BookKind(ctx, sys, cat, r) }
			b, err := readerBook(FilterBook(readingBook(), tt.reader.IsDirector), sys.Info(), kinds, "Therin")
			if err != nil {
				t.Fatal(err)
			}
			// The index: laid out again from the catalog and counted.
			ix := b.Parts[0].Chapters[0].Pages
			if len(ix) != 1 || ix[0].Blocks[0].Count != strings.Count(tt.index, ",")+1 {
				t.Fatalf("index pages = %+v", ix)
			}
			l, _ := kinds("ancestries")
			ie := catalogIndex(l, findCategory(sys.Info(), "ancestries"), "Therin")
			var names []string
			for _, it := range ie.slice(bookIndexSlice{}) {
				w := it.Name
				if strings.HasPrefix(it.Summary, rewrittenWord) {
					w += "[Rewritten]"
				}
				names = append(names, w)
			}
			if strings.Join(names, ",") != tt.index {
				t.Fatalf("index = %s\nwant    %s", strings.Join(names, ","), tt.index)
			}
			if len(ie.closing) != 1 || ie.closing[0].Summary != polderLine {
				t.Fatalf("closing = %+v", ie.closing)
			}
			// The cards: the printed Elf never reaches anyone who reads it
			// rewritten, left-out peoples are gone, the campaign's follow.
			if got := cardTitles(b); got != tt.cards {
				t.Fatalf("cards = %s\nwant    %s", got, tt.cards)
			}
			pages := b.Parts[0].Chapters[1].Pages
			end := pages[len(pages)-1].Blocks
			if end[len(end)-1].Type != "text" || end[len(end)-1].Text != polderLine {
				t.Fatalf("the chapter must end with the left-out line: %+v", end[len(end)-1])
			}
			var words strings.Builder
			for _, pg := range pages {
				for _, blk := range pg.Blocks {
					for _, it := range blk.Items {
						words.WriteString(it.Summary + it.Text)
					}
				}
			}
			if strings.Contains(words.String(), "Printed elf") {
				t.Fatal("printed words reached a reader who reads the Elf rewritten")
			}
			if strings.Contains(words.String(), "Cursed") != tt.reader.IsDirector {
				t.Fatal("a secret span is the Director's alone")
			}
			// The search finds the entries the reader's chapter lists and
			// answers a left-out name with its one line.
			idx := buildBookFindIndexWith(b, func(cat string) (*indexEntries, error) { return ie, nil })
			if hits := idx.search("elf"); len(hits) != 1 || hits[0].Name != "Sea Elf" {
				t.Fatalf("find elf = %+v", hits)
			}
			if hits := idx.search("polder"); len(hits) != 1 || hits[0].Summary != polderLine {
				t.Fatalf("find polder = %+v", hits)
			}
		})
	}
	// The loaded book is never changed by a reader's copy.
	orig := readingBook()
	r := reader(playerActor)
	if _, err := readerBook(FilterBook(orig, false), sys.Info(), func(cat string) (*CatalogList, error) { return p.BookKind(ctx, sys, cat, r) }, "Therin"); err != nil {
		t.Fatal(err)
	}
	if got := cardTitles(orig); got != "Devil,Dwarf,Elf,Polder,A card about nothing" {
		t.Fatalf("the package's book changed: %s", got)
	}
	// Another system's book reads as its package.
	other := fakeSystem{m: &SystemManifest{ID: "other", Categories: sys.Info().Categories, EntityPresets: sys.Info().EntityPresets}}
	if l, err := p.BookKind(ctx, other, "ancestries", r); l != nil || err != nil {
		t.Fatalf("another system: %v %v", l, err)
	}
}

func TestBookText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Plain words.", "Plain words."},
		{"<p>One &amp; two.</p><p>Three<br>four</p>", "One & two.\n\nThree\n\nfour"},
	}
	for _, tt := range tests {
		if got := bookText(tt.in); got != tt.want {
			t.Fatalf("bookText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
