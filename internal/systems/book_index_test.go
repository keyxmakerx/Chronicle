package systems

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const testIndexData = `[
 {"slug":"b-strike","name":"Blade Strike","summary":"A cut.","description":"A cut that leaves the foe {@condition bleeding}.","properties":{"class":"Fury","level":1,"effect":"The target is {@condition slowed|slowed down}.","effect_display":"The target is slowed down.","secret":"hidden note"}},
 {"slug":"a-guard","name":"Aegis","summary":"A guard.","properties":{"class":"Censor","level":2}},
 {"slug":"c-rally","name":"Rally","summary":"Everyone up.","properties":{"level":1}},
 {"slug":"d-dash","name":"dash","summary":"Go fast.","properties":{"class":"Fury","level":10}}
]`

const testIndexManifest = `{"id":"demo","name":"Demo","categories":[
 {"slug":"abilities","name":"Abilities","fields":[
  {"key":"class","label":"Class","type":"string"},
  {"key":"level","label":"Level","type":"number"},
  {"key":"effect_display","label":"Effect","type":"string"},
  {"key":"secret","label":"Secret","type":"string","gm_only":true}]}]}`

func indexTestBook(t *testing.T, extra map[string]string) (string, *SystemManifest) {
	t.Helper()
	files := map[string]string{
		"data/abilities.json": testIndexData,
		"book/book.yaml": `title: Rulebook
parts:
  - title: Rules index
    chapters: [by-class, by-level, all, all-paged]
  - title: Director's index
    director: true
    chapters: [secret-index]
`,
		"book/chapters/by-class.yaml":     "title: By class\nindex:\n  category: abilities\n  group: class\n  other: Everyone\n",
		"book/chapters/by-level.yaml":     "title: By level\nindex:\n  category: abilities\n  group: level\n",
		"book/chapters/all.yaml":          "title: All abilities\nindex:\n  category: abilities\n",
		"book/chapters/all-paged.yaml":    "title: Paged\nindex:\n  category: abilities\n  per-page: 3\n",
		"book/chapters/secret-index.yaml": "title: Secret\ndirector: true\nindex:\n  category: abilities\n  group: class\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	dir := writeBook(t, files)
	m := mustManifest(t, testIndexManifest)
	return dir, m
}

func mustManifest(t *testing.T, raw string) *SystemManifest {
	t.Helper()
	var m SystemManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

func chapterByID(b *Book, id string) *BookChapter {
	for pi := range b.Parts {
		for ci := range b.Parts[pi].Chapters {
			if b.Parts[pi].Chapters[ci].ID == id {
				return &b.Parts[pi].Chapters[ci]
			}
		}
	}
	return nil
}

func indexPageTitles(ch *BookChapter) string {
	var out []string
	for _, p := range ch.Pages {
		out = append(out, p.Title)
	}
	return strings.Join(out, ",")
}

func TestBookIndexChapters(t *testing.T) {
	dir, m := indexTestBook(t, nil)
	b, err := LoadBook(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		chapter, titles string
	}{
		{"by-class", "Censor,Fury,Everyone"},
		{"by-level", "Level 1,Level 2,Level 10"},
		{"all", "All abilities"},
		{"all-paged", "A–D,R"},
	}
	for _, tt := range tests {
		ch := chapterByID(b, tt.chapter)
		if ch == nil || !ch.Generated {
			t.Fatalf("%s: missing or not generated: %+v", tt.chapter, ch)
		}
		if got := indexPageTitles(ch); got != tt.titles {
			t.Errorf("%s: pages %q, want %q", tt.chapter, got, tt.titles)
		}
	}
	fury := chapterByID(b, "by-class").Pages[1].Blocks[0]
	if fury.Type != "index" || fury.Key != "class" || fury.Value != "Fury" || fury.Count != 2 {
		t.Errorf("Fury page block: %+v", fury)
	}
}

func TestBookIndexChapterProblems(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"unknown category", "title: X\nindex:\n  category: spells\n", "no category"},
		{"bad group", "title: X\nindex:\n  category: abilities\n  group: Class Name\n", "property name"},
		{"index and pages", "title: X\nindex:\n  category: abilities\npages:\n  - blocks:\n      - text: hi\n", "not both"},
		{"bad per-page", "title: X\nindex:\n  category: abilities\n  per-page: 500\n", "per-page"},
		{"no category", "title: X\nindex:\n  group: class\n", "needs a category"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, m := indexTestBook(t, map[string]string{"book/chapters/by-class.yaml": tt.body})
			b, err := LoadBook(dir, m)
			if err != nil {
				t.Fatal(err)
			}
			ch := chapterByID(b, "by-class")
			if ch.Generated || ch.Pages[0].Blocks[0].Type != "problem" || !strings.Contains(ch.Pages[0].Blocks[0].Text, tt.want) {
				t.Errorf("want a problem naming %q, got %+v", tt.want, ch)
			}
		})
	}
}

func TestBookIndexEntriesAndSlices(t *testing.T) {
	dir, m := indexTestBook(t, nil)
	ie, err := loadIndexEntries(dir, m, "abilities")
	if err != nil {
		t.Fatal(err)
	}
	names := func(items []ReferenceItem) string {
		var out []string
		for _, it := range items {
			out = append(out, it.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(ie.slice(bookIndexSlice{Key: "class", Value: "Fury"})); got != "Blade Strike,dash" {
		t.Errorf("Fury slice: %s", got)
	}
	if got := names(ie.slice(bookIndexSlice{Key: "class", Value: ""})); got != "Rally" {
		t.Errorf("other slice: %s", got)
	}
	if got := names(ie.slice(bookIndexSlice{From: 3})); got != "Rally" {
		t.Errorf("from 3: %s", got)
	}
	if got := names(ie.slice(bookIndexSlice{To: 2})); got != "Aegis,Blade Strike" {
		t.Errorf("to 2: %s", got)
	}

	strike := ie.slice(bookIndexSlice{Key: "class", Value: "Fury"})[0]
	player := ie.indexEntry(strike, false, "class")
	director := ie.indexEntry(strike, true, "class")
	labels := func(e BookIndexEntry) string {
		var out []string
		for _, f := range e.Fields {
			out = append(out, f.Label+"="+f.Value)
		}
		return strings.Join(out, ";")
	}
	if got := labels(player); got != "Level=1;Effect=The target is [[slowed|slowed down]]." {
		t.Errorf("player fields: %s", got)
	}
	if !strings.Contains(labels(director), "Secret=hidden note") {
		t.Errorf("director should see the gm_only field: %s", labels(director))
	}
	if player.Text != "A cut that leaves the foe [[bleeding|bleeding]]." {
		t.Errorf("text: %q", player.Text)
	}
}

func TestRefMarkupToTerms(t *testing.T) {
	tests := []struct{ in, want string }{
		{"{@condition taunted|taunts}", "[[taunted|taunts]]"},
		{"{@resource damage-weakness}", "[[damage weakness|damage-weakness]]"},
		{"no markup", "no markup"},
		{"[[already]] {@combat dying}", "[already] [[dying|dying]]"},
	}
	for _, tt := range tests {
		if got := refMarkupToTerms(tt.in); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBookIndexReaderScope(t *testing.T) {
	dir, m := indexTestBook(t, nil)
	b, err := LoadBook(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	player, director := FilterBook(b, false), FilterBook(b, true)
	fury := bookIndexSlice{Key: "class", Value: "Fury"}
	if !bookShowsSlice(player, "abilities", fury) {
		t.Error("a player's book shows the Fury page")
	}
	if bookShowsSlice(player, "abilities", bookIndexSlice{Key: "secret", Value: "x"}) {
		t.Error("a slice no page shows must be refused")
	}
	if bookShowsSlice(player, "abilities", bookIndexSlice{From: 0, To: 3}) != true {
		t.Error("the paged chapter's first page is shown")
	}

	for _, r := range findInIndex(dir, m, player, "a") {
		if r.Chapter == "secret-index" {
			t.Errorf("a player's search found a Director-only chapter: %+v", r)
		}
	}
	if chapterByID(director, "secret-index") == nil || chapterByID(player, "secret-index") != nil {
		t.Error("the Director's index is in a Director's book only")
	}
	got := findInIndex(dir, m, player, "dash")
	if len(got) == 0 || got[0].Name != "dash" || got[0].Where != "By class · Fury" {
		t.Errorf("find dash: %+v", got)
	}
}

func TestBookIndexGroupEdges(t *testing.T) {
	var big strings.Builder
	big.WriteString("[")
	for i := 0; i < maxBookIndexItems+20; i++ {
		if i > 0 {
			big.WriteString(",")
		}
		fmt.Fprintf(&big, `{"slug":"e%04d","name":"Entry %04d","properties":{"class":"Big"}}`, i, i)
	}
	big.WriteString(`,{"slug":"n1","name":"Null class","properties":{"class":null}},{"slug":"n2","name":"No class","properties":{}}]`)

	dir, m := indexTestBook(t, map[string]string{"data/abilities.json": big.String()})
	b, err := LoadBook(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	ch := chapterByID(b, "by-class")
	if got := indexPageTitles(ch); got != "Big (1),Big (2),Everyone" {
		t.Fatalf("pages %q", got)
	}
	if n := ch.Pages[0].Blocks[0].Count + ch.Pages[1].Blocks[0].Count; n != maxBookIndexItems+20 {
		t.Errorf("split pages hold %d entries", n)
	}
	if ch.Pages[2].Blocks[0].Count != 2 {
		t.Errorf("missing and null share the Everyone page: %+v", ch.Pages[2].Blocks[0])
	}
	ie, _ := loadIndexEntries(dir, m, "abilities")
	_, second, _ := pageSlice(ch.Pages[1])
	if got := len(ie.slice(second)); got != 20 {
		t.Errorf("second page lists %d", got)
	}

	listy := `[{"slug":"a","name":"A","properties":{"class":["x","y"]}}]`
	dir, m = indexTestBook(t, map[string]string{"data/abilities.json": listy})
	b, _ = LoadBook(dir, m)
	if ch := chapterByID(b, "by-class"); ch.Generated || !strings.Contains(ch.Pages[0].Blocks[0].Text, "plain value") {
		t.Errorf("a list-valued group must be refused: %+v", ch)
	}
}
