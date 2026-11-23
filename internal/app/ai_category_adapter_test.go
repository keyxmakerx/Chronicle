package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type stubCategoryTypes struct {
	infos   []entities.CustomizeCategoryInfo
	deleted []int
}

func (s *stubCategoryTypes) CustomizeCategories(context.Context, string) ([]entities.CustomizeCategoryInfo, error) {
	return s.infos, nil
}
func (s *stubCategoryTypes) DeleteEntityType(_ context.Context, id int) error {
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubCategoryTypes) MergeEntityType(context.Context, string, int, int) (int, error) {
	return 0, nil
}

func categoryStubTypes() *stubCategoryTypes {
	adventures := entities.PresetCategoryAdventures
	parent := 2
	return &stubCategoryTypes{infos: []entities.CustomizeCategoryInfo{
		{Type: entities.EntityType{ID: 1, Name: "Adventure", NamePlural: "Adventures", Slug: "adventures", PresetCategory: &adventures}, Pages: 11},
		{Type: entities.EntityType{ID: 2, Name: "Location", NamePlural: "Places", Slug: "location"}, Pages: 3, Trashed: 1},
		{Type: entities.EntityType{ID: 3, Name: "City", Slug: "city", ParentTypeID: &parent}},
	}}
}

// The kind reads parents, kinds and who sees a category from the list the
// Customizer draws, so its review row counts what the Customizer counts.
func TestAICategoryAdapter_Categories(t *testing.T) {
	got, err := aiCategoryAdapter{types: categoryStubTypes()}.Categories(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	want := []records.CategoryInfo{
		{ID: 1, Name: "Adventure", Plural: "Adventures", Slug: "adventures", DMOnly: true, Pages: 11},
		{ID: 2, Name: "Location", Plural: "Places", Slug: "location", Kinds: []string{"City"}, Pages: 3, Trashed: 1},
		{ID: 3, Name: "City", Slug: "city", Parent: "Places"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("categories\n%+v\nwant\n%+v", got, want)
	}
}

// The service's delete takes an id alone, so the adapter refuses one that is
// not the campaign's.
func TestAICategoryAdapter_DeleteChecksCampaign(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		deleted []int
		wantErr bool
	}{
		{"this campaign's category", 3, []int{3}, false},
		{"another campaign's category", 99, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := categoryStubTypes()
			err := aiCategoryAdapter{types: s}.DeleteCategory(context.Background(), "c", tc.id)
			if (err != nil) != tc.wantErr || !reflect.DeepEqual(s.deleted, tc.deleted) {
				t.Errorf("err %v, deleted %v; want error %v, deleted %v", err, s.deleted, tc.wantErr, tc.deleted)
			}
		})
	}
}
