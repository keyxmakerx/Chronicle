package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// sightWorld has one page of each kind of hiding: private, shared person by
// person, in a DM-only category, with a GM-only passage and sheet field, and
// an open one.
func sightWorld() *Lookups {
	ghost := page("ghost", "The Ghost", "Character", true)
	ghost.EntityTypeID = 1
	ghost.EntryHTML = strp(`<p>Haunts the inn. <span data-secret="true">She is the duke's sister.</span></p>`)
	ghost.FieldsData = map[string]any{"motive": "revenge", "backstory": "drowned", "age": 30}
	letter := page("letter", "Sealed Letter", "Item", false)
	letter.Visibility = entities.VisibilityCustom
	heist := page("heist", "The Heist", "Adventure", false)
	heist.TypeDMOnly = true
	inn := page("inn", "The Rusty Anchor", "Location", false)
	return &Lookups{Pages: &fakePages{
		all: []entities.Entity{ghost, letter, heist, inn},
		types: []entities.EntityType{{ID: 1, Name: "Character", Fields: []entities.FieldDefinition{
			{Key: "age", Label: "Age", Type: "number"},
			{Key: "motive", Label: "Motive", Type: "text", GMOnly: true},
			{Key: "backstory", Label: "Backstory", Type: "text", OwnerOnly: true},
		}}},
	}}
}

// A key reading as the owner is told which pages, passages and fields
// players never see, so its AI knows what not to repeat to them. A player's
// view carries no marks: it never reads what they would mark.
func TestPageSight_Marks(t *testing.T) {
	ctx := context.Background()
	answer := func(a Actor, r Record) string {
		ans := sightWorld().Answer(ctx, camp, a, []Record{r})
		if ans[0].Error != "" {
			t.Fatalf("%s: %s", r.Str("what"), ans[0].Error)
		}
		return ans[0].Text
	}
	cases := []struct {
		name    string
		got     func() string
		want    []string
		notWant []string
	}{
		{"outline, owner", func() string { return sightWorld().PageIndex(ctx, camp, owner) },
			[]string{sightLegend, "The Ghost (GM only)", "Sealed Letter (shared with some)", "**Adventures** (1) (GM only): The Heist\n", "**Locations** (1): The Rusty Anchor\n"},
			[]string{"The Heist (GM only)", "Rusty Anchor ("}},
		{"outline, player", func() string { return sightWorld().PageIndex(ctx, camp, player) },
			[]string{"Sealed Letter"}, []string{"(GM only)", "(shared with some)", sightLegend}},
		{"list_pages, owner", func() string { return answer(owner, lookup("pages", nil)) },
			[]string{sightLegend, "- The Ghost (GM only)\n", "- Sealed Letter (shared with some)\n", "### Adventures (1) (GM only)\n", "- The Rusty Anchor\n"}, nil},
		{"list_pages, player", func() string { return answer(player, lookup("pages", nil)) },
			[]string{"- Sealed Letter\n"}, []string{"(GM only)", "(shared with some)", sightLegend}},
		{"read_page, owner, hidden page", func() string { return answer(owner, lookup("page", map[string]any{"name": "The Ghost"})) },
			[]string{"## Page: The Ghost\n\n**For the GM only:** players never see this page. Text between {gm{ and }gm} is hidden from players. Sheet fields hidden from players: `motive`. Sheet fields only its player and the GM see: `backstory`. Don't repeat these to players.\n\n",
				"{gm{She is the duke's sister.}gm}"}, nil},
		{"read_page, owner, shared page", func() string { return answer(owner, lookup("page", map[string]any{"name": "Sealed Letter"})) },
			[]string{"**For the GM only:** only the people it is shared with see this page. Don't repeat"}, nil},
		{"read_page, owner, DM-only category", func() string { return answer(owner, lookup("page", map[string]any{"name": "The Heist"})) },
			[]string{"its category is for the DM team only"}, nil},
		{"read_page, owner, open page", func() string { return answer(owner, lookup("page", map[string]any{"name": "The Rusty Anchor"})) },
			nil, []string{"For the GM only"}},
		{"read_page, player", func() string { return answer(player, lookup("page", map[string]any{"name": "Sealed Letter"})) },
			nil, []string{"For the GM only", "{gm{"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.got()
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
		})
	}
}
