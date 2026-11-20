package rolls

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The Customize hook: a "Dice" tab whose body is the widget the rolls plugin
// registers, pointed at this campaign.
func TestCustomizeTabFactory(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	tab := CustomizeTabFactory()(cc)
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
