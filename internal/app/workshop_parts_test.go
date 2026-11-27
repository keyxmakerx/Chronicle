package app

import (
	"reflect"
	"sort"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
)

// TestRegisterWorkshopParts pins that each feature's own init joins the
// Workshop, and that a degraded plugin's parts stay out while the rest join.
func TestRegisterWorkshopParts(t *testing.T) {
	var labels []string
	for _, s := range workshopSources {
		labels = append(labels, s.Label)
	}
	sort.Strings(labels)
	if want := []string{"ancestries", "ask the director", "card decks", "crafting recipes", "creature variants", "drawer buttons", "field rules", "recipe sharing", "respite", "the calendar", "trackers", "transformations"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("workshop sources = %v, want %v", labels, want)
	}

	tests := []struct {
		name      string
		trackers  bool
		fieldRule bool
		// transformations is the layer store's health.
		transformations bool
		want            []string
	}{
		{name: "all degraded", want: []string{"ancestry"}},
		{name: "trackers healthy", trackers: true, want: []string{"ancestry", "clock", "tracker"}},
		{name: "field rules healthy", fieldRule: true, want: []string{"ancestry", "field", "item", "words"}},
		{name: "transformations healthy", transformations: true, want: []string{"ancestry", "transformation"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := database.NewPluginHealthRegistry()
			health.Register(trackers.PluginSlug, tt.trackers, nil, 1, 1)
			health.Register(transformations.PluginSlug, tt.transformations, nil, 1, 1)
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

// Card events and deck actions join the Workshop only while the decks plugin
// is healthy and its service is bound.
func TestCardDecksJoinTheWorkshopOnlyWhileHealthy(t *testing.T) {
	for _, tt := range []struct {
		name    string
		healthy bool
		bound   bool
		want    bool
	}{
		{"degraded", false, true, false},
		{"healthy but unbound", true, false, false},
		{"healthy", true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			health := database.NewPluginHealthRegistry()
			health.Register(decks.PluginSlug, tt.healthy, nil, 1, 1)
			d := workshopDeps{Health: health}
			if tt.bound {
				d.Decks = newDecksStory(nil)
			}
			reg := homebrew.NewRegistry()
			registerWorkshopParts(reg, d)
			joined := false
			for _, e := range reg.Catalog().Events {
				joined = joined || e.Name == cardPlayedEvent || e.Name == cardDrawnEvent
			}
			if joined != tt.want {
				t.Errorf("card events joined = %v, want %v", joined, tt.want)
			}
		})
	}
}
