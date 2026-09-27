// Package systems — operator_diag_calendar.go is the calendar plugin's own
// operator diagnostic: "is the calendar plugin actually live for THIS
// campaign, and is it up to date?" — a narrower question than campaign.config
// (which addons are enabled) or campaign.surfaces (which page a URL renders),
// and one campaign.config's own CALV5-PLACEHOLDER notes explicitly deferred
// to this file once the calendar plugin had counts worth reading again
// (calendar-v5 seams, #778).
//
// COUNTS ONLY. Never event text, never member names, never an answer to "what
// does the calendar say" — this is a state check, not a content export (that
// is aiexport's CategoryCalendarEvents, gated Director-only). RunDiagnostic's
// redactSecrets pass still runs over the output as defense in depth.
package systems

import (
	"context"
	"fmt"
	"strings"
)

// CalendarStatsFacts is what `calendar.stats` reads.
type CalendarStatsFacts struct {
	Found        bool
	CampaignID   string
	CampaignName string

	AddonEnabled *bool
	AddonNote    string

	CalendarCount  int
	EventCount     int
	MoonCount      int
	EraCount       int
	EventKindCount int

	// PluginHealthy/MigrationVersion/MigrationLatest report the calendar
	// plugin's OWN schema migration state — the same registry host.plugins
	// reads — distinct from whether the addon is enabled for this campaign:
	// a healthy plugin with the addon disabled and a campaign with the
	// addon enabled but an unhealthy plugin are different findings.
	PluginHealthy    bool
	MigrationVersion int
	MigrationLatest  int

	// FoundrySyncState is reported honestly rather than inferred: the sync
	// API's own calendar routes still answer a structured 503
	// (`calendar_rebuilding`) regardless of how live the calendar plugin
	// itself now is — syncapi is untouched, deliberately later work (#778's
	// own scope excludes it). See internal/plugins/syncapi/calendar_api_handler.go.
	FoundrySyncState string

	Notes []string
}

// CalendarDiagProvider is the injected read-only window into per-campaign
// calendar state. Implemented by the app layer (dependency inversion —
// systems must not import the calendar/campaigns/addons plugins), wired once
// at startup by SetCalendarDiagProvider.
type CalendarDiagProvider interface {
	CalendarStats(ctx context.Context, campaignID string) (CalendarStatsFacts, error)
}

var calendarDiagProvider CalendarDiagProvider

// SetCalendarDiagProvider wires the per-campaign read window for calendar.stats.
func SetCalendarDiagProvider(p CalendarDiagProvider) { calendarDiagProvider = p }

// calendarStatsDiagnostic is the catalog entry for calendar.stats.
func calendarStatsDiagnostic() Diagnostic {
	return Diagnostic{
		Name:    "calendar.stats",
		Title:   "Calendar plugin state for one campaign — counts only, never content",
		Desc:    "How many calendars/events/moons/eras/event-kinds a campaign has, the calendar plugin's own migration version (distinct from whether the addon is enabled), and the Foundry sync state. Redacted: counts and settings only, never event text or member names. THE check for 'is the calendar plugin actually live here, and current'.",
		ArgHint: "<campaignId>",
		Run:     renderCalendarStats,
	}
}

// providerNotWiredCalendar mirrors providerNotWired's "nobody answered, not
// an empty result" shape for calendar.stats's own provider.
const providerNotWiredCalendar = "_Calendar provider not wired — the app layer did not inject it at startup, so NOTHING WAS READ. This is NOT \"this campaign has no calendars\", it is \"nobody was asked\". Fix the wiring in RegisterRoutes before drawing any conclusion._\n"

// renderCalendarStats prints calendar.stats.
func renderCalendarStats(arg string) string {
	var b strings.Builder
	b.WriteString("## calendar.stats\n\n")
	campaignID := strings.TrimSpace(arg)
	if campaignID == "" {
		b.WriteString("_Usage: `<campaignId>` (run `campaigns.list` first)._\n")
		return b.String()
	}
	if calendarDiagProvider == nil {
		b.WriteString(providerNotWiredCalendar)
		return b.String()
	}
	f, err := calendarDiagProvider.CalendarStats(context.Background(), campaignID)
	if err != nil {
		fmt.Fprintf(&b, "- Error: %v\n", err)
		return b.String()
	}
	if !f.Found {
		fmt.Fprintf(&b, "_No campaign `%s` (check the id with `campaigns.list`)._\n", campaignID)
		return b.String()
	}
	fmt.Fprintf(&b, "campaign **%s** (`%s`)\n\n", f.CampaignName, f.CampaignID)

	switch {
	case f.AddonEnabled == nil:
		fmt.Fprintf(&b, "> calendar addon: **UNKNOWN** — %s\n\n", fallback(f.AddonNote, "the addons service could not be read"))
	case !*f.AddonEnabled:
		b.WriteString("> calendar addon: **disabled** for this campaign — its JSON API and the dashboard/category \"Upcoming Events\" cards are unreachable or blank.\n\n")
	default:
		b.WriteString("> calendar addon: **enabled**.\n\n")
	}

	b.WriteString("### Counts\n\n")
	fmt.Fprintf(&b, "- calendars: **%d**\n", f.CalendarCount)
	fmt.Fprintf(&b, "- events: **%d**\n", f.EventCount)
	fmt.Fprintf(&b, "- moons: **%d**\n", f.MoonCount)
	fmt.Fprintf(&b, "- eras: **%d**\n", f.EraCount)
	fmt.Fprintf(&b, "- event kinds: **%d**\n\n", f.EventKindCount)

	b.WriteString("### Plugin state\n\n")
	if f.PluginHealthy {
		fmt.Fprintf(&b, "- schema: **healthy**, migration `%d` applied (latest on disk: `%d`)\n", f.MigrationVersion, f.MigrationLatest)
	} else {
		fmt.Fprintf(&b, "- schema: **UNHEALTHY** — migration `%d` applied, `%d` on disk. The calendar plugin's routes may be degraded or unregistered; cross-check `host.plugins`.\n", f.MigrationVersion, f.MigrationLatest)
	}
	fmt.Fprintf(&b, "- Foundry sync: %s\n", fallback(f.FoundrySyncState, "unknown"))

	writeNotes(&b, f.Notes)
	return b.String()
}
