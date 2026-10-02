package systems

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// writeBook lays out a package folder with the given files (path -> body).
func writeBook(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const testBookIndex = `title: Rulebook
mark: DS
turn: slide
glossary: rules-glossary.json
theme:
  accent: "#a78bfa"
  heading-font: "Inter, system-ui, sans-serif"
terms:
  Edge: A bonus on a roll.
parts:
  - title: Player's book
    chapters: [basics, broken, secret-in-player-part]
  - title: Director's book
    director: true
    chapters: [monsters]
`

const testBasics = `title: The Basics
intro: One roll settles most things.
pages:
  - title: The power roll
    blocks:
      - text: Roll [[edge]] and see.
      - type: roll
        dice: 2d10
        label: Power roll
        modifier: true
        bands:
          - {max: 11, label: Tier 1}
          - {max: 16, label: Tier 2}
          - {label: Tier 3}
      - type: note
        text: SECRET-NOTE director tip
  - title: SECRET-PAGE
    director: true
    blocks:
      - text: hidden page
`

const testMonsters = `title: SECRET-CHAPTER Monsters
pages:
  - blocks:
      - text: director only part
`

const testSecretChapter = `title: SECRET-DIRECTOR-CHAPTER
director: true
pages:
  - title: Lair
    blocks:
      - type: creature
        name: Goblin
        look: Small and quick.
        notice: [It keeps to the shadows.]
        stats:
          - {label: Stamina, value: SECRET-STAT-15}
        note: SECRET-CREATURE-NOTE
`

func testBookFiles() map[string]string {
	return map[string]string{
		"book/book.yaml":                           testBookIndex,
		"book/chapters/basics.yaml":                testBasics,
		"book/chapters/broken.yaml":                "title: Broken\npages:\n  - blocks:\n      - type: rol\n",
		"book/chapters/secret-in-player-part.yaml": testSecretChapter,
		"book/chapters/monsters.yaml":              testMonsters,
		"data/rules-glossary.json":                 `[{"name":"Bleeding","summary":"Lose {@resource stamina|Stamina} each turn."},{"name":"Edge","summary":"from glossary"}]`,
	}
}

func TestLoadBook_Valid(t *testing.T) {
	dir := writeBook(t, testBookFiles())
	if !HasBook(dir) {
		t.Fatal("HasBook = false, want true")
	}
	b, err := LoadBook(dir, &SystemManifest{Name: "Draw Steel"})
	if err != nil {
		t.Fatalf("LoadBook: %v", err)
	}
	if b.Turn != "slide" || b.Mark != "DS" || b.SystemName != "Draw Steel" {
		t.Errorf("cover = %q/%q/%q", b.Turn, b.Mark, b.SystemName)
	}
	if b.Theme["accent"] != "#a78bfa" {
		t.Errorf("theme accent = %q", b.Theme["accent"])
	}
	if got := b.Terms["bleeding"].Text; got != "Lose Stamina each turn." {
		t.Errorf("glossary markup not flattened: %q", got)
	}
	if got := b.Terms["edge"].Text; got != "A bonus on a roll." {
		t.Errorf("book terms should override glossary: %q", got)
	}
	if len(b.Parts) != 2 || len(b.Parts[0].Chapters) != 3 {
		t.Fatalf("parts/chapters = %+v", b.Parts)
	}
	roll := b.Parts[0].Chapters[0].Pages[0].Blocks[1]
	if roll.Dice == nil || roll.Dice.Count != 2 || roll.Dice.Sides != 10 || len(roll.Bands) != 3 || roll.Bands[2].Max != nil {
		t.Errorf("roll = %+v", roll)
	}
	if !b.Parts[0].Chapters[0].Pages[0].Blocks[2].Director {
		t.Error("note block must always be Director-only")
	}
	broken := b.Parts[0].Chapters[1]
	if broken.Pages[0].Blocks[0].Type != "problem" || !strings.Contains(broken.Pages[0].Blocks[0].Text, "page 1, block 1") {
		t.Errorf("broken chapter = %+v", broken)
	}
}

func TestHasBook_Absent(t *testing.T) {
	for _, dir := range []string{"", t.TempDir()} {
		if HasBook(dir) {
			t.Errorf("HasBook(%q) = true, want false", dir)
		}
	}
}

// TestFilterBook_PlayerNeverReceivesDirectorContent marshals the player's
// copy and checks no Director-only string survives anywhere in it.
func TestFilterBook_PlayerNeverReceivesDirectorContent(t *testing.T) {
	dir := writeBook(t, testBookFiles())
	b, err := LoadBook(dir, &SystemManifest{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		director bool
		want     []string
		wantNot  []string
	}{
		{
			name:     "player",
			director: false,
			want:     []string{"The Basics", "Power roll"},
			wantNot:  []string{"SECRET", "Director's book", `"problem"`, "block 1"},
		},
		{
			name:     "director",
			director: true,
			want:     []string{"SECRET-NOTE", "SECRET-PAGE", "SECRET-CHAPTER", "SECRET-DIRECTOR-CHAPTER", "SECRET-STAT-15", "SECRET-CREATURE-NOTE", `"problem"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := FilterBook(b, tt.director)
			if out.IsDirector != tt.director {
				t.Errorf("IsDirector = %v", out.IsDirector)
			}
			raw, _ := json.Marshal(out)
			s := string(raw)
			for _, w := range tt.want {
				if !strings.Contains(s, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(s, w) {
					t.Errorf("player copy leaks %q", w)
				}
			}
		})
	}
	if len(FilterBook(b, true).Parts) != 2 {
		t.Error("filtering for a player must not change the loaded book")
	}
}

func TestFilterBook_CreatureStatsHiddenFromPlayers(t *testing.T) {
	b := &Book{Parts: []BookPart{{Title: "P", Chapters: []BookChapter{{ID: "c", Title: "C", Pages: []BookPage{{Blocks: []BookBlock{{
		Type: "creature", Name: "Goblin", Look: "small",
		Stats: []BookStat{{Label: "Stamina", Value: "15"}}, Note: "flank them", HiddenStats: true,
	}}}}}}}}}
	got := FilterBook(b, false).Parts[0].Chapters[0].Pages[0].Blocks[0]
	if got.Stats != nil || got.Note != "" || !got.HiddenStats || got.Look != "small" {
		t.Errorf("player creature = %+v", got)
	}
	if b.Parts[0].Chapters[0].Pages[0].Blocks[0].Stats == nil {
		t.Error("filtering mutated the source book")
	}
}

func TestLoadBook_CoverErrors(t *testing.T) {
	ok := "parts:\n  - title: P\n    chapters: []\n"
	tests := []struct {
		name  string
		index string
		want  string
	}{
		{"unknown key", "titel: x\n" + ok, "titel"},
		{"bad turn", "turn: spin\n" + ok, "turn"},
		{"unknown theme key", "theme:\n  background: \"#fff\"\n" + ok, "background"},
		{"css injection in colour", "theme:\n  accent: \"red;background:url(x)\"\n" + ok, "not allowed"},
		{"css injection in font", "theme:\n  body-font: \"x;}body{display:none\"\n" + ok, "not allowed"},
		{"mark too long", "mark: ABCD\n" + ok, "mark"},
		{"no parts", "title: x\n", "no parts"},
		{"part without title", "parts:\n  - chapters: []\n", "no title"},
		{"glossary path escape", "glossary: ../secret.json\n" + ok, "glossary"},
		{"empty file", "", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeBook(t, map[string]string{"book/book.yaml": tt.index})
			_, err := LoadBook(dir, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestBuildBookBlock(t *testing.T) {
	widgets := map[string]bool{"rulebook-frontpage": true}
	max := func(n int) *int { return &n }
	type band = struct {
		Max   *int   `yaml:"max"`
		Label string `yaml:"label"`
		Text  string `yaml:"text"`
	}
	tests := []struct {
		name    string
		block   bookBlockYAML
		wantErr string
	}{
		{"text shorthand", bookBlockYAML{Text: "hi"}, ""},
		{"empty text", bookBlockYAML{Type: "text"}, "needs text"},
		{"unknown type", bookBlockYAML{Type: "rol"}, `no block type "rol"`},
		{"widget declared", bookBlockYAML{Type: "widget", Widget: "rulebook-frontpage"}, ""},
		{"widget not declared", bookBlockYAML{Type: "widget", Widget: "evil"}, "not one of"},
		{"bad dice", bookBlockYAML{Type: "roll", Dice: "2d1000", Bands: []band{{Label: "x"}}}, "dice"},
		{"bands out of order", bookBlockYAML{Type: "roll", Dice: "2d10", Bands: []band{{Max: max(16), Label: "a"}, {Max: max(11), Label: "b"}, {Label: "c"}}}, "lowest"},
		{"last band with max", bookBlockYAML{Type: "roll", Dice: "2d10", Bands: []band{{Max: max(11), Label: "a"}}}, "last band"},
		{"middle band without max", bookBlockYAML{Type: "roll", Dice: "2d10", Bands: []band{{Label: "a"}, {Label: "b"}}}, "needs max"},
		{"cards without items", bookBlockYAML{Type: "cards"}, "needs items"},
		{"example without steps", bookBlockYAML{Type: "example"}, "needs steps"},
		{"creature without name", bookBlockYAML{Type: "creature"}, "needs name"},
		{"too long", bookBlockYAML{Text: strings.Repeat("x", maxBookString+1)}, "longer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildBookBlock(tt.block, widgets)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestBookViewerIsDirector(t *testing.T) {
	tests := []struct {
		name string
		cc   campaigns.CampaignContext
		want bool
	}{
		{"owner", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}, true},
		{"player granted Director visibility", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsDmGranted: true}, true},
		{"scribe", campaigns.CampaignContext{MemberRole: campaigns.RoleScribe}, false},
		{"player", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer}, false},
		{"not a member", campaigns.CampaignContext{MemberRole: campaigns.RoleNone}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bookViewerIsDirector(&tt.cc); got != tt.want {
				t.Errorf("bookViewerIsDirector = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSystemIndexContent_BookMount: a system with a book mounts the core
// rulebook widget, full-bleed, instead of its front page widget.
func TestSystemIndexContent_BookMount(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test Campaign"}}
	m := &SystemManifest{ID: "drawsteel", Name: "Draw Steel", Widgets: []WidgetDef{{Slug: "rulebook-frontpage"}}}
	var buf bytes.Buffer
	if err := SystemIndexContent(cc, m, nil, true).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{`data-widget="rulebook"`, `data-book-url="/campaigns/camp-1/systems/drawsteel/book"`, "data-rulebook-fullbleed", "rulebook.css"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
	// The script comes from the layout's body-script registry: htmx strips
	// a page fragment's script tags on boosted navigation.
	if strings.Contains(html, "rulebook.js") {
		t.Errorf("rulebook.js must not be loaded from the page fragment:\n%s", html)
	}
	if strings.Contains(html, `data-widget="rulebook-frontpage"`) || strings.Contains(html, `aria-label="Breadcrumb"`) {
		t.Errorf("book page should mount only the book:\n%s", html)
	}
}
