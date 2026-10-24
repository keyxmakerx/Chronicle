package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type fakeAddonSwitch struct {
	on  map[string]bool
	err error
}

func (f fakeAddonSwitch) IsEnabledForCampaign(_ context.Context, _, slug string) (bool, error) {
	return f.on[slug], f.err
}

func TestAIFeatureGate(t *testing.T) {
	cases := []struct {
		name string
		sw   fakeAddonSwitch
		want map[string]string
	}{
		{"all on", fakeAddonSwitch{on: map[string]bool{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true, "maps": true}}, map[string]string{}},
		{"new campaign", fakeAddonSwitch{on: map[string]bool{}}, map[string]string{
			records.KindCalendar: "Calendar", "event": "Calendar", "weather": "Calendar", "roll-night": "Calendar", "moon": "Calendar",
			"game-night": "Game nights", "map": "Maps", "pin": "Maps",
		}},
		{"calendar off, game nights on", fakeAddonSwitch{on: map[string]bool{sessions.SessionsAddonSlug: true, "maps": true}}, map[string]string{
			records.KindCalendar: "Calendar", "event": "Calendar", "weather": "Calendar", "roll-night": "Calendar", "moon": "Calendar",
			"game-night": "Calendar",
		}},
		{"lookup fails", fakeAddonSwitch{err: errors.New("db down")}, map[string]string{}},
	}
	for _, c := range cases {
		got := aiFeatureGate(c.sw)(context.Background(), "camp")
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %s = %q, want %q", c.name, k, got[k], v)
			}
		}
	}
}
