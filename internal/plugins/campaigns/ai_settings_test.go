package campaigns

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

type fakeAICampaigns struct{ c Campaign }

func (f fakeAICampaigns) GetByID(context.Context, string) (*Campaign, error) { return &f.c, nil }
func (f fakeAICampaigns) GetEventTierDefinitions(context.Context, string) ([]TierDefinition, error) {
	return []TierDefinition{{Name: "Major"}, {Name: "Detail"}}, nil
}

type fakeAIAddons []PluginHubAddon

func (f fakeAIAddons) ListForPluginHub(context.Context, string) ([]PluginHubAddon, error) {
	return f, nil
}

func aiSettingsFixture() *AISettings {
	d := "A broken kingdom."
	// Secrets sit in the settings and the join code to prove none reach the answer.
	code := "JOINSECRET"
	camp := Campaign{Name: "Therin", Description: &d, JoinCode: &code,
		Settings: `{"system_id":"drawsteel","guest_code":"GUESTSECRET"}`}
	return NewAISettings(fakeAICampaigns{camp}, fakeAIAddons{
		{Slug: "almanac", Name: "Calendar", Enabled: true, Installed: true},
		{Slug: "atlas", Name: "Maps", Installed: true},
		{Slug: "soon", Name: "Coming Soon Thing"},
		{Slug: "drawsteel", Name: "Draw Steel", Category: "system", Enabled: true, Installed: true},
	})
}

func TestAISettings_Answer(t *testing.T) {
	tests := []struct {
		name     string
		actor    aiplug.Actor
		has, not []string
	}{
		{"owner gets everything", aiplug.Actor{UserID: "o", Role: 3},
			[]string{"Name: Therin", "Description: A broken kingdom.", "Game system: Draw Steel", "Add-ons on: Calendar", "Add-ons off: Maps", "Event importance levels: Major, Detail"},
			[]string{"JOINSECRET", "GUESTSECRET", "Coming Soon", "Draw Steel, "}},
		{"player-safe mode and a player's key get what a player sees", aiplug.Actor{UserID: "p", Role: 1},
			[]string{"Name: Therin", "Game system: Draw Steel"},
			[]string{"Add-ons", "Calendar", "Maps", "importance", "JOINSECRET", "GUESTSECRET"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, text, err := aiSettingsFixture().Answer(context.Background(), "c", tt.actor, aiplug.Record{})
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.has {
				if !strings.Contains(text, w) {
					t.Errorf("answer lacks %q:\n%s", w, text)
				}
			}
			for _, w := range tt.not {
				if strings.Contains(text, w) {
					t.Errorf("answer holds %q:\n%s", w, text)
				}
			}
		})
	}
}

func TestAISettings_ToolIsNotOwnerOnly(t *testing.T) {
	in := (&AISettings{}).LookupInfo()
	if in.What != "settings" || in.Tool.Name != "campaign_settings" || in.Tool.Owner || in.Tool.PlayerDesc == "" {
		t.Errorf("info = %+v", in)
	}
}

func TestGameSystemName(t *testing.T) {
	hub := []PluginHubAddon{{Slug: "dnd5e", Name: "D&D 5e", Category: "system", Enabled: true}}
	for id, want := range map[string]string{"dnd5e": "D&D 5e", "": "D&D 5e", "custom:https://x.test/a": "a custom game system", "other": "other"} {
		if got := gameSystemName(id, hub); got != want {
			t.Errorf("gameSystemName(%q) = %q, want %q", id, got, want)
		}
	}
}
