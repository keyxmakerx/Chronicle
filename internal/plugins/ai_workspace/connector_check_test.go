package ai_workspace

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// typesNoPages answers the category list and fails the test on any page
// lookup, since what a connected AI hears must not say whether a page exists.
type typesNoPages struct{ t *testing.T }

func (s typesNoPages) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return []entities.EntityType{
		{ID: 1, Slug: "lore", Name: "Lore", NamePlural: "Lore"},
		{ID: 2, Slug: "places", Name: "Places", NamePlural: "Places"},
		{ID: 3, Slug: "race", Name: "Race", NamePlural: "Races"},
	}, nil
}
func (s typesNoPages) GetBySlug(context.Context, string, string) (*entities.Entity, error) {
	s.t.Error("the check looked up a page")
	return nil, nil
}
func (s typesNoPages) GetEntityTypeBySlug(context.Context, string, string) (*entities.EntityType, error) {
	return nil, nil
}
func (s typesNoPages) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	s.t.Error("the check looked up a page")
	return nil, nil
}

// foldersIn has a Races folder in Lore and a Docks folder in Places.
type foldersIn struct{}

func (foldersIn) FindFolder(_ context.Context, _ string, typeID int, name string) (*entities.SidebarNode, error) {
	for _, f := range []struct {
		typeID int
		name   string
	}{{1, "races"}, {2, "docks"}} {
		if f.typeID == typeID && strings.EqualFold(name, f.name) {
			return &entities.SidebarNode{ID: "f", Name: name, EntityTypeID: typeID}, nil
		}
	}
	return nil, nil
}
func (foldersIn) SetEntityFolder(context.Context, string, string, *string) error { return nil }

// A new page filed in a folder its category doesn't have is told to the AI,
// with where the folder is and the fix.
func TestConnectorCheck(t *testing.T) {
	khepri := func(typ, folder string) string {
		return "---\nname: The Khepri\ntype: " + typ + "\nfolder: " + folder + "\n---\n\n# The Khepri\n\nBeetle folk.\n"
	}
	tests := []struct {
		name    string
		changes string
		want    []string
	}{
		{"folder in another category", khepri("races", "Races"),
			[]string{`block 1 (The Khepri): the Race category has no folder "Races"; Lore has one. Use type: Lore, or leave the folder out`}},
		{"folder nowhere", khepri("race", "Desert folk"),
			[]string{`block 1 (The Khepri): the Race category has no folder "Desert folk". Make it first with a folder block in this request, or leave the folder out`}},
		{"new category", khepri("peoples", "Docks"),
			[]string{`block 1 (The Khepri): the category "peoples" is new, so it has no folder "Docks"; Places has one. Use type: Places, or leave the folder out`}},
		{"new category, folder nowhere", khepri("peoples", "Desert folk"),
			[]string{`block 1 (The Khepri): the category "peoples" is new, so it has no folder "Desert folk". Use an existing category, or leave the folder out`}},
		{"right category", khepri("lore", "Races"), nil},
		{"folder made in the request", "---\nkind: folder\nname: Desert folk\ncategory: Race\n---\n\n" + khepri("race", "Desert folk"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{importLookup: typesNoPages{t}, importFolders: foldersIn{}}
			got, err := h.ConnectorCheck(context.Background(), "camp-1", tt.changes)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}
