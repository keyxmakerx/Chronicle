package entities

import (
	"reflect"
	"testing"
)

// A category made now places tags and relations under the title, as the
// no-layout fallback draws them, so a new page can carry both. The layout an
// empty stored layout falls back to is unchanged, so existing categories are
// never rewritten to match.
func TestNewCategoryLayout_PlacesTagsAndRelationsUnderTheTitle(t *testing.T) {
	tests := []struct {
		name   string
		layout EntityTypeLayout
		want   []string
	}{
		{"new category", NewCategoryLayout(), []string{"title", "tags", "relations", "entry", "image", "attributes", "details"}},
		{"fallback default is untouched", DefaultLayout(), []string{"title", "entry", "image", "attributes", "details"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blockTypeOrder(tt.layout); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("blocks = %v, want %v", got, tt.want)
			}
		})
	}
}

// Building the new layout must not alter the shared default.
func TestNewCategoryLayout_DoesNotShareBlocksWithDefault(t *testing.T) {
	_ = NewCategoryLayout()
	if got := blockTypeOrder(DefaultLayout()); len(got) != 5 {
		t.Errorf("DefaultLayout changed: %v", got)
	}
}
