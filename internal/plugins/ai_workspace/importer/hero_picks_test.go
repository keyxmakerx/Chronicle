package importer

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// Only a hero row (a category of the character preset) that writes sheet
// values is handed to the hero check, with the sheet as it will stand and
// the keys written, and what the check says becomes the row's warnings.
func TestClassify_HeroPicks(t *testing.T) {
	character := "character"
	types := append(creatureTypes(),
		entities.EntityType{ID: 8, Name: "Hero", Slug: "hero", Enabled: true, PresetCategory: &character, Fields: creatureFields()},
	)
	lookup := &stubLookup{types: types}
	hero := creaturePage("Ash", map[string]any{"level": 2})
	hero.FrontMatter.Type = "hero"
	heroNoFields := creaturePage("Bryn", nil)
	heroNoFields.FrontMatter.Type = "hero"
	pages := []ParsedPage{hero, heroNoFields, creaturePage("Goblin", map[string]any{"level": 1})}

	var calls []map[string]any
	var written [][]string
	check := func(fields map[string]any, w []string) []string {
		calls = append(calls, fields)
		written = append(written, w)
		return []string{"Level: off the list."}
	}
	if _, err := NewClassifier(lookup, "camp-1").WithHeroPicks(check).ClassifyAll(context.Background(), pages); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], map[string]any{"level": 2}) || !reflect.DeepEqual(written[0], []string{"level"}) {
		t.Fatalf("check calls = %#v written %#v; want one call for the hero's level", calls, written)
	}
	if !reflect.DeepEqual(pages[0].Warnings, []string{"Level: off the list."}) {
		t.Errorf("hero warnings = %q", pages[0].Warnings)
	}
	for _, i := range []int{1, 2} {
		if len(pages[i].Warnings) != 0 {
			t.Errorf("%s warned: %q", pages[i].Name, pages[i].Warnings)
		}
	}
	// A warning never changes the row's status: the paste stays importable.
	cls, _ := NewClassifier(lookup, "camp-1").WithHeroPicks(check).ClassifyAll(context.Background(), []ParsedPage{hero})
	if cls[0].Status == StatusFieldProblem {
		t.Errorf("a pick warning blocked the row: %v", cls[0].Status)
	}
}
