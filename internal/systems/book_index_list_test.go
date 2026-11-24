package systems

import (
	"errors"
	"strings"
	"testing"
)

// The index tests' data with a list property, for list facets.
const testListData = `[
 {"slug":"b-strike","name":"Blade Strike","properties":{"class":"Fury","level":1,"type":"signature","keywords":["Melee","Strike"],"secret":"x","tier1_display":"2 damage"}},
 {"slug":"a-guard","name":"Aegis","properties":{"class":"Censor","level":2,"type":"heroic","keywords":["Magic"]}},
 {"slug":"c-rally","name":"Rally","properties":{"level":1}},
 {"slug":"d-dash","name":"dash","properties":{"class":"Fury","level":10,"keywords":[{"bad":1},"Move",""]}}
]`

const testListManifest = `{"id":"demo","name":"Demo","categories":[
 {"slug":"abilities","name":"Abilities","fields":[
  {"key":"class","label":"Class","type":"string"},
  {"key":"level","label":"Level","type":"number"},
  {"key":"keywords_display","label":"Keywords","type":"string"},
  {"key":"tier1_display","label":"Tier 1 (11 or lower)","type":"string"},
  {"key":"secret","label":"Secret","type":"string","gm_only":true}]}]}`

func listTestBook(t *testing.T, byClass string) (*Book, string, *SystemManifest) {
	t.Helper()
	dir, _ := indexTestBook(t, map[string]string{
		"data/abilities.json":         testListData,
		"book/chapters/by-class.yaml": byClass,
	})
	m := mustManifest(t, testListManifest)
	b, err := LoadBook(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	return b, dir, m
}

const byClassWithList = "title: By class\nindex:\n  category: abilities\n  group: class\n  other: Everyone\n" +
	"  filters: [level, type, keywords]\n  sections: level\n  tiers: [tier1_display]\n"

func TestBookIndexList(t *testing.T) {
	b, _, _ := listTestBook(t, byClassWithList)
	ch := chapterByID(b, "by-class")
	if !ch.Generated {
		t.Fatalf("chapter not generated: %+v", ch.Pages[0].Blocks[0])
	}
	l := ch.Pages[0].Blocks[0].List
	if l == nil {
		t.Fatal("index block has no list")
	}
	var labels []string
	for _, f := range l.Filters {
		labels = append(labels, f.Key+"="+f.Label)
	}
	// level has a manifest label, keywords only a display twin's, type none.
	if got := strings.Join(labels, ","); got != "level=Level,type=Type,keywords=Keywords" {
		t.Errorf("filters: %s", got)
	}
	if l.Sections != "level" || l.SectionLabel != "Level" || strings.Join(l.Tiers, ",") != "tier1_display" {
		t.Errorf("list: %+v", l)
	}
	for _, p := range ch.Pages {
		if p.Blocks[0].List != l {
			t.Errorf("page %q has another list", p.Title)
		}
	}
	if chapterByID(b, "by-level").Pages[0].Blocks[0].List != nil {
		t.Error("a chapter naming no filters has no list")
	}
}

func TestBookIndexListProblems(t *testing.T) {
	head := "title: X\nindex:\n  category: abilities\n  group: class\n"
	tests := []struct{ name, extra, want string }{
		{"bad key", "  filters: [Level]\n", "property name"},
		{"gm only filter", "  filters: [secret]\n", "Director-only"},
		{"gm only sections", "  sections: secret\n", "Director-only"},
		{"gm only tier", "  tiers: [secret]\n", "Director-only"},
		{"named twice", "  filters: [level, level]\n", "named twice"},
		{"too many filters", "  filters: [a, b, c, d, e, f, g]\n", "at most 6"},
		{"too many tiers", "  tiers: [a, b, c, d, e]\n", "at most 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _, _ := listTestBook(t, head+tt.extra)
			blk := chapterByID(b, "by-class").Pages[0].Blocks[0]
			if blk.Type != "problem" || !strings.Contains(blk.Text, tt.want) {
				t.Errorf("want a problem naming %q, got %+v", tt.want, blk)
			}
		})
	}
}

func TestBookIndexFacets(t *testing.T) {
	_, dir, m := listTestBook(t, byClassWithList)
	ie, err := loadIndexEntries(dir, m, "abilities")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ReferenceItem{}
	for _, it := range ie.items {
		byID[it.ID] = it
	}
	show := func(fs []BookIndexFacet) string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Key+"="+strings.Join(f.Values, "|"))
		}
		return strings.Join(out, ";")
	}
	list := &BookIndexList{Filters: []BookIndexFilter{{Key: "level"}, {Key: "type"}, {Key: "keywords"}}, Sections: "level"}
	gm := &BookIndexList{Filters: []BookIndexFilter{{Key: "secret"}, {Key: "level"}}}
	tests := []struct {
		name, id string
		list     *BookIndexList
		director bool
		want     string
	}{
		{"scalars, lists, capitals", "b-strike", list, false, "level=1;type=Signature;keywords=Melee|Strike"},
		{"missing values left out", "c-rally", list, false, "level=1"},
		{"non-scalar and empty list values dropped", "d-dash", list, false, "level=10;keywords=Move"},
		{"gm only hidden from players", "b-strike", gm, false, "level=1"},
		{"gm only shown to directors", "b-strike", gm, true, "secret=X;level=1"},
		{"no list", "b-strike", nil, true, ""},
	}
	for _, tt := range tests {
		if got := show(ie.facets(byID[tt.id], tt.list, tt.director)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestBookIndexSliceList(t *testing.T) {
	b, _, _ := listTestBook(t, byClassWithList)
	l, ok := sliceList(b, "abilities", bookIndexSlice{Key: "class", Value: "Fury"})
	if !ok || l == nil || len(l.Filters) != 3 {
		t.Errorf("Fury page: %v %+v", ok, l)
	}
	if _, ok := sliceList(b, "abilities", bookIndexSlice{Key: "class", Value: "Nobody"}); ok {
		t.Error("a slice no page lists is refused")
	}
	if l, ok := sliceList(b, "abilities", bookIndexSlice{Key: "level", Value: "1"}); !ok || l != nil {
		t.Errorf("a page without filters is listed with no layout: %v %+v", ok, l)
	}
}

func TestFindBookEntry(t *testing.T) {
	b, dir, m := listTestBook(t, byClassWithList)
	loads := 0
	load := func() (*indexEntries, error) { loads++; return loadIndexEntries(dir, m, "abilities") }
	player, director := FilterBook(b, false), FilterBook(b, true)

	_, hit, err := findBookEntry(player, "abilities", "d-dash", load)
	if err != nil || hit == nil {
		t.Fatalf("dash: %v %+v", err, hit)
	}
	// The first page listing it: the by-class chapter's Fury page.
	if hit.Chapter != "by-class" || hit.Page != 1 || hit.PageTitle != "Fury" || hit.List == nil || hit.Item.Name != "dash" {
		t.Errorf("dash hit: %+v", hit)
	}
	if loads != 1 {
		t.Errorf("entries read %d times, want once", loads)
	}

	tests := []struct {
		name, cat, id string
		book          *Book
		found         bool
	}{
		{"unknown id", "abilities", "nope", player, false},
		{"unknown category", "spells", "d-dash", player, false},
		{"director book", "abilities", "a-guard", director, true},
	}
	for _, tt := range tests {
		_, hit, err := findBookEntry(tt.book, tt.cat, tt.id, load)
		if err != nil || (hit != nil) != tt.found {
			t.Errorf("%s: hit %+v err %v", tt.name, hit, err)
		}
	}

	// A book whose only index of a category is Director-only lists none of
	// it to a player.
	only := "title: By class\npages:\n  - blocks:\n      - text: Nothing here.\n"
	ob, _, _ := listTestBook(t, only)
	for _, id := range []string{"a-guard", "d-dash"} {
		pl := FilterBook(ob, false)
		pl.Parts = keepParts(pl.Parts, "secret-index", "by-class")
		if _, hit, _ := findBookEntry(pl, "abilities", id, load); hit != nil {
			t.Errorf("%s: a player's book without an index of it found %+v", id, hit)
		}
	}
	if _, hit, _ := findBookEntry(FilterBook(ob, true), "abilities", "a-guard", load); hit == nil || hit.Chapter == "by-class" {
		t.Errorf("a Director finds it in the Director's index: %+v", hit)
	}

	failing := func() (*indexEntries, error) { return nil, errors.New("unreadable") }
	if _, _, err := findBookEntry(player, "abilities", "d-dash", failing); err == nil {
		t.Error("a failed read is an error")
	}
}

// keepParts keeps only the parts' chapters with the given ids.
func keepParts(parts []BookPart, ids ...string) []BookPart {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []BookPart
	for _, p := range parts {
		var chs []BookChapter
		for _, c := range p.Chapters {
			if want[c.ID] {
				chs = append(chs, c)
			}
		}
		if len(chs) > 0 {
			p.Chapters = chs
			out = append(out, p)
		}
	}
	return out
}
