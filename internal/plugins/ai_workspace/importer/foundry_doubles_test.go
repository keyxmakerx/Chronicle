package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// A new creature page named like a monster Foundry has is warned about; an
// update, another category or a name Foundry lacks is not.
func TestClassify_FoundryDoubles(t *testing.T) {
	types := append(creatureTypes(), entities.EntityType{ID: 9, Name: "Note", Slug: "note", Enabled: true})
	lookup := &stubLookup{types: types, bySlug: map[string]*entities.Entity{
		"orc-brute": {ID: "ent-9", Name: "Orc Brute", Slug: "orc-brute", CampaignID: "camp-1", EntityTypeID: 7},
	}}
	update := creaturePage("Orc Brute", nil)
	update.FrontMatter.Action = ActionUpdate
	note := creaturePage("Stalker", nil)
	note.FrontMatter.Type = "note"
	pages := []ParsedPage{creaturePage("Stalker", nil), update, note, creaturePage("Ashen Hound", nil)}
	has := func(name string) string {
		if strings.EqualFold(name, "stalker") || strings.EqualFold(name, "orc brute") {
			return "in the Monsters compendium"
		}
		return ""
	}
	if _, err := NewClassifier(lookup, "camp-1").WithFoundryMonsters(has).ClassifyAll(context.Background(), pages); err != nil {
		t.Fatal(err)
	}
	want := "Foundry already has a monster called Stalker (in the Monsters compendium)"
	if w := strings.Join(pages[0].Warnings, "|"); !strings.Contains(w, want) {
		t.Errorf("new creature page: warnings %q", pages[0].Warnings)
	}
	for i := 1; i < len(pages); i++ {
		for _, w := range pages[i].Warnings {
			if strings.Contains(w, "Foundry already has") {
				t.Errorf("page %d (%s) warned: %q", i, pages[i].Name, w)
			}
		}
	}
}
