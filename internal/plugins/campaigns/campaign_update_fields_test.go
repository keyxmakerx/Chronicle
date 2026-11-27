package campaigns

import (
	"reflect"
	"testing"
)

// The activity log names which settings an edit changed, never their values,
// and a field sent unchanged is not named.
func TestChangedCampaignFields(t *testing.T) {
	desc := "Old words"
	was := &Campaign{Name: "Therin", Description: &desc, IsPublic: false}
	s := func(v string) *string { return &v }
	b := func(v bool) *bool { return &v }
	tests := []struct {
		name string
		req  UpdateCampaignRequest
		want []string
	}{
		{"nothing sent", UpdateCampaignRequest{}, []string{}},
		{"sent unchanged", UpdateCampaignRequest{Name: s("Therin"), Description: s("Old words"), IsPublic: b(false)}, []string{}},
		{"name", UpdateCampaignRequest{Name: s("Therin Reborn")}, []string{"name"}},
		{"all three", UpdateCampaignRequest{Name: s("X"), Description: s("New"), IsPublic: b(true)}, []string{"name", "description", "is_public"}},
		{"description cleared", UpdateCampaignRequest{Description: s("")}, []string{"description"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changedCampaignFields(was, tt.req); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("changedCampaignFields = %v, want %v", got, tt.want)
			}
		})
	}
	// A campaign with no description treats an empty one as unchanged.
	if got := changedCampaignFields(&Campaign{Name: "A"}, UpdateCampaignRequest{Description: s("")}); len(got) != 0 {
		t.Errorf("empty description on a campaign with none = %v, want none", got)
	}
}
