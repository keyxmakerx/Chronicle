// settings_plugin_hub_motion_test.go pins #702: the legacy plugin-hub
// card (rendered by PluginHubListContent, still reached via the
// /plugins/fragment HTMX route) transitions only the property that
// actually changes on hover (the accent ring's box-shadow), not every
// property, closing the last transition-all site in campaigns/*.templ.

package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPluginHubListContent_NoTransitionAll(t *testing.T) {
	cc := &CampaignContext{Campaign: &Campaign{ID: "c-9"}}
	addons := []PluginHubAddon{
		{AddonID: 1, Slug: "loot-tracker", Name: "Loot Tracker", Installed: true, Enabled: true},
	}
	var buf bytes.Buffer
	if err := PluginHubListContent(cc, addons, true, "csrf").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "transition-all") {
		t.Errorf("plugin hub card must not use transition-all (animates every property); got:\n%s", html)
	}
	if !strings.Contains(html, "transition-shadow") {
		t.Errorf("plugin hub card should scope its hover transition to box-shadow; got:\n%s", html)
	}
}
