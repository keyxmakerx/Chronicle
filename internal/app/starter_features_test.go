package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingEnabler struct {
	on     []string
	failOn string
}

func (r *recordingEnabler) EnableForCampaignBySlug(_ context.Context, _, slug, _ string) error {
	if slug == r.failOn {
		return errors.New("boom")
	}
	r.on = append(r.on, slug)
	return nil
}

// A new campaign gets the usual eleven features, the calendar before Game
// nights, with Armory, Crafting, Transformations and the Sync API left off; one
// feature that fails is skipped without stopping the rest.
func TestStarterFeatures(t *testing.T) {
	tests := []struct {
		name   string
		failOn string
		want   []string
	}{
		{"all eleven in order", "", starterFeatureSlugs},
		{"one failure skips only that feature", "maps", []string{
			"calendar", "sessions", "notes", "jot-notes", "timeline",
			"media-gallery", "npcs", "player-character-claiming", "attributes", "player-notes",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recordingEnabler{failOn: tt.failOn}
			starterFeatures{addons: r}.EnableStarterFeatures(context.Background(), "c1", "u1")
			if !reflect.DeepEqual(r.on, tt.want) {
				t.Errorf("turned on %v, want %v", r.on, tt.want)
			}
		})
	}
	if len(starterFeatureSlugs) != 11 {
		t.Errorf("the usual set is %d features, want 11", len(starterFeatureSlugs))
	}
	for _, off := range []string{"armory", "crafting", "transformations", "sync-api"} {
		for _, on := range starterFeatureSlugs {
			if on == off {
				t.Errorf("%s must start off", off)
			}
		}
	}
}
