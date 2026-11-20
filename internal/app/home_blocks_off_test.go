package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

// A new campaign starts with the calendar off, and the players' and guests'
// homes place blocks that read the calendar and game nights routes. Each must
// be named off then, or it stays a loading bar and raises an error toast.
func TestHomeBlocksOff(t *testing.T) {
	nightsBlocks := []string{
		campaigns.BlockWhenWePlay, campaigns.BlockLastTime, campaigns.BlockGettingStarted,
		campaigns.BlockWhenWeCouldPlay, campaigns.BlockNextGame,
	}
	all := map[string]bool{
		calendar.PluginSlug: true, sessions.SessionsAddonSlug: true, timeline.PluginSlug: true, maps.PluginSlug: true,
	}
	without := func(slug string) map[string]bool {
		m := map[string]bool{}
		for k, v := range all {
			m[k] = v && k != slug
		}
		return m
	}
	tests := []struct {
		name       string
		addons     map[string]bool
		calendarUp bool
		sessionsUp bool
		wantOff    []string
		wantOn     []string
	}{
		{"everything on", all, true, true, nil,
			append([]string{campaigns.BlockTodayInWorld, campaigns.BlockTimelineFull, campaigns.BlockMapFull}, nightsBlocks...)},
		{"a new campaign: nothing enabled", map[string]bool{}, true, true,
			append([]string{campaigns.BlockTodayInWorld, campaigns.BlockTimelinePreview, campaigns.BlockTimelineFull, campaigns.BlockMapFull}, nightsBlocks...), nil},
		{"calendar off", without(calendar.PluginSlug), true, true,
			append([]string{campaigns.BlockTodayInWorld}, nightsBlocks...),
			[]string{campaigns.BlockTimelineFull, campaigns.BlockMapFull}},
		{"calendar on but degraded", all, false, true,
			[]string{campaigns.BlockTodayInWorld}, nightsBlocks},
		{"game nights off", without(sessions.SessionsAddonSlug), true, true,
			nightsBlocks, []string{campaigns.BlockTodayInWorld}},
		{"sessions plugin degraded", all, true, false,
			nightsBlocks, []string{campaigns.BlockTodayInWorld}},
		{"timelines off", without(timeline.PluginSlug), true, true,
			[]string{campaigns.BlockTimelinePreview, campaigns.BlockTimelineFull},
			append([]string{campaigns.BlockMapFull}, nightsBlocks...)},
		{"maps off", without(maps.PluginSlug), true, true,
			[]string{campaigns.BlockMapFull}, []string{campaigns.BlockTimelineFull}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := homeBlocksOff(tt.addons, tt.calendarUp, tt.sessionsUp)
			for _, bt := range tt.wantOff {
				if got[bt] == "" {
					t.Errorf("%s should be off, is not", bt)
				}
			}
			for _, bt := range tt.wantOn {
				if _, off := got[bt]; off {
					t.Errorf("%s should still draw, is off (%q)", bt, got[bt])
				}
			}
		})
	}
}
