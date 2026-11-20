package rolls

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The Customize hook: a "Dice" tab whose body is the widget the rolls plugin
// registers, pointed at this campaign.
func TestCustomizeTabFactory(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	tab := CustomizeTabFactory(nil)(cc)
	tests := []struct {
		name string
		ok   bool
	}{
		{"has an id, a label, an icon and a body", tab.ID == "dice" && tab.Label == "Dice" && strings.Contains(tab.Icon, "fa-dice") && tab.Content != nil},
	}
	for _, tt := range tests {
		if !tt.ok {
			t.Fatalf("%s: tab = %+v", tt.name, tab)
		}
	}
	var buf bytes.Buffer
	if err := tab.Content.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-widget="dice-customize"`, `data-campaign-id="camp-1"`, "dice_customize.css"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("tab body is missing %s:\n%s", want, buf.String())
		}
	}
}

type stubDice struct {
	s   *Settings
	err error
}

func (d stubDice) Settings(context.Context, string) (*Settings, error) { return d.s, d.err }

// The Dice row says the campaign's die and whether players pick their own,
// from the rolls plugin's own settings; the engine's defaults stand for an
// empty pick, and a failed read gives no words rather than wrong ones.
func TestCustomizeTabSummary(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	tests := []struct {
		name string
		dice DiceSettingsReader
		want campaigns.CustomizeSummary
	}{
		{"defaults", stubDice{s: &Settings{PlayersPick: true}}, campaigns.CustomizeSummary{Text: "Jewelled, onyx", Sub: "players pick their own"}},
		{"a chosen set", stubDice{s: &Settings{DiceSet: "confetti", DiceColour: "sea"}}, campaigns.CustomizeSummary{Text: "Confetti glass, sea", Sub: "same for everyone"}},
		{"unread", stubDice{err: context.Canceled}, campaigns.CustomizeSummary{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CustomizeTabFactory(tt.dice)(cc).Summary(context.Background()); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	if CustomizeTabFactory(nil)(cc).Summary != nil {
		t.Error("a tab without a reader has no summary")
	}
}

// Each set's name is the one the dice engine shows in its pickers.
func TestDiceSetNamesMatchEngine(t *testing.T) {
	var all strings.Builder
	for _, dir := range []string{"../../../static/js/dice", "../../../static/js/dice/sets"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".js") {
				b, _ := os.ReadFile(dir + "/" + e.Name())
				all.Write(b)
			}
		}
	}
	src := all.String()
	for _, key := range DiceSetKeys {
		name := diceSetNames[key]
		if name == "" {
			t.Errorf("%s has no name", key)
			continue
		}
		single := "key: '" + key + "', name: '" + name + "'"
		double := "key: '" + key + "', name: \"" + name + "\""
		if !strings.Contains(src, single) && !strings.Contains(src, double) {
			t.Errorf("the engine does not call %s %q", key, name)
		}
	}
}
