package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

type fakeAddons map[string]bool

func (f fakeAddons) IsEnabledForCampaign(_ context.Context, _, slug string) (bool, error) {
	return f[slug], nil
}

func lookupNames(d *aiDeps) string {
	var names []string
	for _, l := range d.coverageLookups() {
		names = append(names, l.LookupInfo().Tool.Name)
	}
	return strings.Join(names, " ")
}

// The settings lookup is always plugged; timelines only while the plugin is
// up, and neither tool is owner-only (the lookups give a player's key the
// player's part).
func TestCoverageLookups_Plugged(t *testing.T) {
	tests := []struct {
		name string
		deps aiDeps
		want string
	}{
		{"timeline degraded", aiDeps{}, "campaign_settings"},
		{"timeline up", aiDeps{Timelines: struct{ timeline.TimelineService }{}}, "campaign_settings timelines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lookupNames(&tt.deps); got != tt.want {
				t.Errorf("tools = %q, want %q", got, tt.want)
			}
			for _, l := range tt.deps.coverageLookups() {
				if l.LookupInfo().Tool.Owner {
					t.Errorf("%s is owner-only", l.LookupInfo().What)
				}
			}
		})
	}
}

func TestAddonOffReason(t *testing.T) {
	d := &aiDeps{Addons: fakeAddons{"timeline": false, "maps": true}}
	if why := d.addonOffReason("timeline", "Timelines")(context.Background(), "c"); !strings.Contains(why, "Timelines is switched off") {
		t.Errorf("off add-on said %q", why)
	}
	if why := d.addonOffReason("maps", "Maps")(context.Background(), "c"); why != "" {
		t.Errorf("on add-on refused: %q", why)
	}
}
