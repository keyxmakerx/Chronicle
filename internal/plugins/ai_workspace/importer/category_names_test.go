package importer

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// TestCategoryNames pins that a category the AI names in the plural is not
// pluralized a second time ("races" must not become "Racess").
func TestCategoryNames(t *testing.T) {
	tests := []struct {
		slug, name, plural string
	}{
		{"race", "Race", "Races"},
		{"races", "Race", "Races"},
		{"ancestry", "Ancestry", "Ancestries"},
		{"ancestries", "Ancestry", "Ancestries"},
		{"class", "Class", "Classes"},
		{"classes", "Class", "Classes"},
		{"box", "Box", "Boxes"},
		{"day", "Day", "Days"},
		{"warrior", "Warrior", "Warriors"},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			name, plural := categoryNames(tt.slug)
			if name != tt.name || plural != tt.plural {
				t.Errorf("categoryNames(%q) = %q, %q; want %q, %q", tt.slug, name, plural, tt.name, tt.plural)
			}
		})
	}
}

// TestClassify_TypeMatchesPluralName pins that `type: races` finds a
// category whose slug is "race", so a second paste doesn't make a new one.
func TestClassify_TypeMatchesPluralName(t *testing.T) {
	tests := []struct {
		typ    string
		wantID int
	}{
		{"race", 1},
		{"races", 1},
		{"Races", 1},
		{"characters", 2},
		{"notes", 3}, // a real slug wins over another type's plural name
	}
	lookup := &stubLookup{types: []entities.EntityType{
		{ID: 1, Slug: "race", Name: "Race", NamePlural: "Races"},
		{ID: 2, Slug: "character", Name: "Character", NamePlural: "Characters"},
		{ID: 3, Slug: "notes", Name: "Notes", NamePlural: "Notes"},
		{ID: 4, Slug: "note", Name: "Note", NamePlural: "Notes"},
	}}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			clf := NewClassifier(lookup, "camp-1")
			cls, err := clf.ClassifyAll(context.Background(), []ParsedPage{{
				Name:        "Fairy",
				FrontMatter: FrontMatter{Name: "Fairy", Type: tt.typ, Action: ActionCreate},
				Status:      StatusNew,
			}})
			if err != nil {
				t.Fatalf("ClassifyAll: %v", err)
			}
			if cls[0].ExistingType == nil || cls[0].ExistingType.ID != tt.wantID {
				t.Fatalf("type %q matched %+v; want type %d", tt.typ, cls[0].ExistingType, tt.wantID)
			}
			if cls[0].IsNewCategory {
				t.Error("an existing category was offered as new")
			}
		})
	}
}
