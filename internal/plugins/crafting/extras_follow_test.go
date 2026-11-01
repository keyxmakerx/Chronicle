package crafting

import (
	"reflect"
	"testing"
)

func TestFollowMaterials(t *testing.T) {
	moss := Material{ItemID: "moss", Qty: 1}
	salt := Material{ItemID: "salt", Qty: 2}
	herb := Material{Tag: "herb", Qty: 1}
	subs := []Substitute{{For: 0, Use: "herb", Costs: "bane"}, {For: 2, Use: "liquid", Costs: "points"}}
	tests := []struct {
		name          string
		before, after []Material
		want          []Substitute
	}{
		{"unchanged", []Material{moss, salt, herb}, []Material{moss, salt, herb}, subs},
		{"reordered", []Material{moss, salt, herb}, []Material{herb, moss}, []Substitute{{For: 1, Use: "herb", Costs: "bane"}, {For: 0, Use: "liquid", Costs: "points"}}},
		{"material removed", []Material{moss, salt, herb}, []Material{salt, herb}, []Substitute{{For: 1, Use: "liquid", Costs: "points"}}},
		{"all removed", []Material{moss, salt, herb}, []Material{}, []Substitute{}},
		{"index out of range", []Material{moss}, []Material{moss}, []Substitute{{For: 0, Use: "herb", Costs: "bane"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := DefaultExtras()
			x.Substitutes = append([]Substitute(nil), subs...)
			got := FollowMaterials(x, tt.before, tt.after).Substitutes
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	if got := FollowMaterials(Extras{}, nil, nil).Substitutes; got == nil {
		t.Error("no substitutes should still be a list, not null")
	}
}
