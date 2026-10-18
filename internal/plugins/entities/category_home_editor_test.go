package entities

import (
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCategoryHomeEditorState(t *testing.T) {
	saved := `{"rows":[{"id":"own","columns":[{"id":"c","width":12,"blocks":[{"id":"b","type":"entity_grid"}]}]}]}`
	desc := "Every place in the world"
	tests := []struct {
		name        string
		layout      *string
		desc        *string
		wantUnsaved bool
		wantRow     string
		wantDesc    string
	}{
		{name: "nothing saved starts from the built-in home", wantUnsaved: true, wantRow: "row-header"},
		{name: "a saved home", layout: &saved, desc: &desc, wantRow: "own", wantDesc: desc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}}
			et := &EntityType{ID: 7, CampaignID: "c1", Name: "Place", NamePlural: "Places", DashboardLayout: tt.layout, Description: tt.desc}
			var st struct {
				Scope       string                    `json:"scope"`
				Unsaved     bool                      `json:"unsaved"`
				Key         string                    `json:"key"`
				Description string                    `json:"description"`
				Only        []string                  `json:"only"`
				Layout      campaigns.DashboardLayout `json:"layout"`
				Preview     string                    `json:"preview"`
			}
			if err := json.Unmarshal([]byte(categoryHomeEditorState(cc, et)), &st); err != nil {
				t.Fatal(err)
			}
			if st.Scope != "cathome" || st.Key != "7" || st.Unsaved != tt.wantUnsaved || st.Description != tt.wantDesc ||
				st.Preview != "/campaigns/c1/entity-types/7/home-preview" {
				t.Errorf("state = %+v", st)
			}
			if len(st.Layout.Rows) == 0 || st.Layout.Rows[0].ID != tt.wantRow {
				t.Errorf("layout starts with %+v, want row %q", st.Layout.Rows, tt.wantRow)
			}
			only := map[string]bool{}
			for _, ty := range st.Only {
				only[ty] = true
			}
			for _, ty := range []string{"category_header", "entity_grid", "search_bar"} {
				if !only[ty] {
					t.Errorf("palette leaves out %s", ty)
				}
			}
			if only["welcome_banner"] {
				t.Error("palette offers the campaign home's welcome banner")
			}
		})
	}
}

// Every widget the category home offers must be one a save accepts.
func TestCategoryHomeTypesAreSaveable(t *testing.T) {
	for _, ty := range categoryHomeTypes {
		if !campaigns.ValidBlockTypes[ty] {
			t.Errorf("%s is offered but a save refuses it", ty)
		}
	}
}
