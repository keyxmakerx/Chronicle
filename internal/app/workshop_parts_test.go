package app

import (
	"reflect"
	"sort"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// TestRegisterWorkshopParts pins that each feature's own init joins the
// Workshop, and that a degraded plugin's parts stay out while the rest join.
func TestRegisterWorkshopParts(t *testing.T) {
	var labels []string
	for _, s := range workshopSources {
		labels = append(labels, s.Label)
	}
	sort.Strings(labels)
	if want := []string{"ancestries", "creature variants", "field rules", "trackers"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("workshop sources = %v, want %v", labels, want)
	}

	tests := []struct {
		name      string
		trackers  bool
		fieldRule bool
		want      []string
	}{
		{name: "all degraded", want: []string{"ancestry"}},
		{name: "trackers healthy", trackers: true, want: []string{"ancestry", "clock", "tracker"}},
		{name: "field rules healthy", fieldRule: true, want: []string{"ancestry", "field", "item", "words"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := database.NewPluginHealthRegistry()
			health.Register(trackers.PluginSlug, tt.trackers, nil, 1, 1)
			d := workshopDeps{Health: health}
			if tt.fieldRule {
				d.FieldRules = &fieldRulesWorld{}
			}
			reg := homebrew.NewRegistry()
			registerWorkshopParts(reg, d)
			var got []string
			for _, p := range reg.Catalog().Parts {
				got = append(got, p.Name)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parts = %v, want %v", got, tt.want)
			}
		})
	}
}
