package systems

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const draftTableRules = `intro: How we play at this table.
pages:
  - title: Fumbles
    blocks:
      - text: A couple of rules we added.
      - type: callout
        title: Critical fumbles
        text: A natural 2 drops your weapon.
      - type: roll
        dice: 1d6
        label: Heroic surge
        bands:
          - max: 2
            label: Nothing
          - label: Surge
  - title: Camping
    director: true
    blocks:
      - type: flaps
        items:
          - title: Short rest
            text: Ten minutes.
      - type: note
        text: Roll the night encounter secretly.
`

func TestParseHouseChapterDraft(t *testing.T) {
	pkg := testPackage(t, nil)
	tests := []struct {
		name    string
		raw     string
		pages   bool
		blocks  string
		wantErr string
	}{
		{name: "whole chapter", raw: draftTableRules, pages: true, blocks: "1 highlighted box, 1 dice roll, 1 set of flaps, 1 Director's note"},
		{name: "intro only keeps the pages", raw: "intro: Just a line.\n", pages: false},
		{name: "nothing at all", raw: "", pages: false},
		{name: "empty page list", raw: "pages: []\n", wantErr: "at least one page"},
		{name: "unknown key", raw: "intro: x\ncolour: red\n", wantErr: `no setting called "colour"`},
		{name: "page without blocks", raw: "pages:\n  - title: Empty\n", wantErr: "page 1: the page has no blocks"},
		{name: "bad block", raw: "pages:\n  - title: A\n    blocks:\n      - type: roll\n        dice: lots\n", wantErr: "page 1, block 1"},
		{name: "unknown widget", raw: "pages:\n  - title: A\n    blocks:\n      - type: widget\n        widget: nope\n", wantErr: "page 1, block 1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := ParseHouseChapterDraft([]byte(tc.raw), pkg)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || apperror.SafeCode(err) != 422 {
					t.Fatalf("err = %v, want 422 containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.HasPages() != tc.pages {
				t.Errorf("HasPages = %v", d.HasPages())
			}
			if d.Blocks() != tc.blocks {
				t.Errorf("Blocks = %q, want %q", d.Blocks(), tc.blocks)
			}
		})
	}
}

func TestParseHouseChapterDraft_CountsHiddenParts(t *testing.T) {
	d, err := ParseHouseChapterDraft([]byte(draftTableRules), testPackage(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	// The Director-only page counts once, its note inside it not again.
	if got := d.HiddenParts(); got != 1 {
		t.Errorf("HiddenParts = %d, want 1", got)
	}
	if got := d.PageTitles(); strings.Join(got, "|") != "Fumbles|Camping" {
		t.Errorf("PageTitles = %v", got)
	}
}

func TestHouseChapterPreview(t *testing.T) {
	pkg := testPackage(t, nil)
	d, err := ParseHouseChapterDraft([]byte(draftTableRules), pkg)
	if err != nil {
		t.Fatal(err)
	}
	b := HouseChapterPreview(pkg, " Table rules ", "", false, d)
	if b == nil {
		t.Fatal("no preview")
	}
	if len(b.Parts) != 1 || len(b.Parts[0].Chapters) != 1 {
		t.Fatalf("parts = %+v", b.Parts)
	}
	ch := b.Parts[0].Chapters[0]
	if ch.Title != "Table rules" || ch.Intro != "How we play at this table." || b.Start != ch.ID || b.CanEdit || !b.IsDirector {
		t.Errorf("preview = %+v / %+v", b, ch)
	}
	if len(ch.Pages) != 2 || ch.Pages[0].Blocks[1].Type != "callout" || ch.Pages[0].Blocks[2].Dice == nil {
		t.Errorf("pages = %+v", ch.Pages)
	}
	// The package's own look travels with it, so the reader draws it alike.
	if b.Title != pkg.Book.Title || b.Look != pkg.Book.Look || len(b.Theme) != len(pkg.Book.Theme) {
		t.Errorf("look not kept: %+v", b)
	}
	if HouseChapterPreview(pkg, "x", "", false, &HouseChapterDraft{}) != nil {
		t.Error("a draft without pages has no preview")
	}
}

func TestBookEditService_WriteHouseChapter(t *testing.T) {
	ctx := context.Background()
	pkg := testPackage(t, nil)
	repo := &fakeBookRepo{}
	svc := newTestService(repo)
	draft := func(raw string) *HouseChapterDraft {
		t.Helper()
		d, err := ParseHouseChapterDraft([]byte(raw), pkg)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	ch, err := svc.WriteHouseChapter(ctx, testCampaign, "u1", pkg, "", "Table rules", draft(draftTableRules))
	if err != nil {
		t.Fatal(err)
	}
	if !ch.House || ch.Title != "Table rules" || ch.Intro != "How we play at this table." || len(ch.Pages) != 2 {
		t.Fatalf("created = %+v", ch)
	}
	if ch.Pages[0].Page.Title != "Fumbles" || ch.Pages[1].Page.Title != "Camping" || ch.Pages[0].State != PageStateMine {
		t.Errorf("pages = %+v", ch.Pages)
	}

	// A replacement swaps every page at once and keeps the intro it leaves out.
	got, err := svc.WriteHouseChapter(ctx, testCampaign, "u1", pkg, ch.ID, "", draft("pages:\n  - title: Only\n    blocks:\n      - text: One page now.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Table rules" || got.Intro != "How we play at this table." || len(got.Pages) != 1 || got.Pages[0].Page.Title != "Only" {
		t.Errorf("replaced = %+v", got)
	}
	if n := len(repo.pages); n != 1 {
		t.Errorf("stored pages = %d, want 1", n)
	}

	// Without pages only the chapter's own lines change.
	got, err = svc.WriteHouseChapter(ctx, testCampaign, "u1", pkg, ch.ID, "Renamed", draft("intro: New line.\ndirector: true\n"))
	if err != nil || got.Title != "Renamed" || got.Intro != "New line." || !got.Director || len(got.Pages) != 1 {
		t.Errorf("lines only = %+v, %v", got, err)
	}

	refusals := []struct {
		name, chapter, title string
		d                    *HouseChapterDraft
		code                 int
	}{
		{"package chapter", "basics", "", draft(draftTableRules), 422},
		{"missing chapter", "house_nope", "", draft(draftTableRules), 404},
		{"new without pages", "", "Empty", draft("intro: x\n"), 422},
		{"new without title", "", " ", draft(draftTableRules), 422},
	}
	for _, r := range refusals {
		if _, err := svc.WriteHouseChapter(ctx, testCampaign, "u1", pkg, r.chapter, r.title, r.d); apperror.SafeCode(err) != r.code {
			t.Errorf("%s: err = %v, want %d", r.name, err, r.code)
		}
	}
}

func TestHouseChapterYAML(t *testing.T) {
	ctx := context.Background()
	pkg := testPackage(t, nil)
	svc := newTestService(&fakeBookRepo{})
	d, err := ParseHouseChapterDraft([]byte(draftTableRules+"  - title: Monsters\n    blocks:\n      - type: creature\n        name: Troll\n        stats:\n          - label: Stamina\n            value: \"40\"\n        note: Regenerates.\n"+
		"  - title: See also\n    blocks:\n      - type: links\n        items:\n          - {title: The basics, chapter: basics}\n          - {title: Secret lair, chapter: lair}\n"+
		"      - type: links\n        items:\n          - {title: Only the lair, chapter: lair}\n"), pkg)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := svc.WriteHouseChapter(ctx, testCampaign, "u1", pkg, "", "Table rules", d)
	if err != nil {
		t.Fatal(err)
	}

	all, hidden, err := HouseChapterYAML(*ch, true, nil)
	if err != nil || hidden != 0 {
		t.Fatalf("director: hidden %d, %v", hidden, err)
	}
	for _, want := range []string{"intro: How we play", "type: callout", "Camping", "Roll the night", "Stamina", "Regenerates."} {
		if !strings.Contains(string(all), want) {
			t.Errorf("director export lacks %q:\n%s", want, all)
		}
	}
	// It reads back as the same chapter.
	back, err := ParseHouseChapterDraft(all, pkg)
	if err != nil || len(back.PageTitles()) != 4 {
		t.Errorf("round trip: %v, %v", back, err)
	}

	players, hidden, err := HouseChapterYAML(*ch, false, map[string]bool{"basics": true})
	if err != nil {
		t.Fatal(err)
	}
	if hidden != 1 {
		t.Errorf("player hidden = %d, want 1 (the Director-only page)", hidden)
	}
	// A link to a chapter players can't open goes, as the reader drops it.
	for _, gone := range []string{"Camping", "Roll the night", "Stamina", "Regenerates.", "Secret lair", "Only the lair"} {
		if strings.Contains(string(players), gone) {
			t.Errorf("player export has %q:\n%s", gone, players)
		}
	}
	if !strings.Contains(string(players), "The basics") {
		t.Errorf("player export lost a link players can follow:\n%s", players)
	}
	if !strings.Contains(string(all), "Secret lair") {
		t.Errorf("director export lost a link:\n%s", all)
	}
	if !strings.Contains(string(players), "name: Troll") {
		t.Errorf("player export lost the creature itself:\n%s", players)
	}
}
