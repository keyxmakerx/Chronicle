package app

import (
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

// homeBlocksOff names the home block types whose own route cannot answer for
// this campaign, so the home draws nothing for them. A block loads its
// fragment on page load; an off addon (or a degraded plugin whose routes were
// never registered) answers 404, which HTMX leaves unswapped, so the block
// would stay a grey bar and raise an error toast for every viewer, on exactly
// the campaigns that have not turned the feature on yet.
//
// The sessions routes sit behind both the calendar and the game nights
// addons (see sessions.RegisterRoutes), so either being off takes the five
// blocks that read them. The value is the feature's name, for the owner's
// note.
func homeBlocksOff(addonOn map[string]bool, calendarUp, sessionsUp bool) map[string]string {
	off := map[string]string{}
	calendarOn := addonOn[calendar.PluginSlug]
	if !calendarOn || !calendarUp {
		off[campaigns.BlockTodayInWorld] = "The calendar"
	}
	nights := ""
	switch {
	case !calendarOn:
		nights = "The calendar"
	case !addonOn[sessions.SessionsAddonSlug] || !sessionsUp:
		nights = "Game nights"
	}
	if nights != "" {
		for _, bt := range []string{
			campaigns.BlockWhenWePlay, campaigns.BlockLastTime, campaigns.BlockGettingStarted,
			campaigns.BlockWhenWeCouldPlay, campaigns.BlockNextGame,
		} {
			off[bt] = nights
		}
	}
	if !addonOn[timeline.PluginSlug] {
		off[campaigns.BlockTimelinePreview] = "Timelines"
		off[campaigns.BlockTimelineFull] = "Timelines"
	}
	if !addonOn[maps.PluginSlug] {
		off[campaigns.BlockMapFull] = "Maps"
	}
	return off
}
