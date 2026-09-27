package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// operator_diag_calendar_adapter.go injects the calendar plugin's own
// per-campaign read window into calendar.stats (calendar-v5 seams, #778) — a
// SEPARATE provider from campaignDiagAdapter in operator_diag_campaign_adapter.go,
// mirroring how entity.* has its own EntityDiagProvider/entityDiagAdapter.
//
// It lives here, not in internal/systems, for the same dependency-inversion
// reason campaignDiagAdapter does (that package must not import plugins).
//
// COUNTS ONLY, read as a declared SYSTEM caller (ADR-049): this is an
// operator health check, not a per-viewer content read, so it deliberately
// bypasses per-user visibility (a hidden moon or a dm_only event still
// counts) — see CalendarStatsFacts' own doc comment for why that is safe
// here specifically (never event text, never member names, never an answer).
//
// EVERY READ DEGRADES INDIVIDUALLY into a Note, never a zero value: "no
// calendars" and "the calendar service could not be read" must never render
// the same sentence.
type calendarDiagAdapter struct {
	campaigns    campaigns.CampaignService
	addons       addons.AddonService
	calendar     calendar.CalendarService
	pluginHealth *database.PluginHealthRegistry
}

// CalendarStats implements systems.CalendarDiagProvider.
func (a calendarDiagAdapter) CalendarStats(ctx context.Context, campaignID string) (systems.CalendarStatsFacts, error) {
	out := systems.CalendarStatsFacts{
		CampaignID: campaignID,
		// Reported as a fact of the current build, not a live read: syncapi's
		// calendar routes are untouched by this restoration (deliberately
		// deferred, #778's own scope) and still answer a structured 503.
		FoundrySyncState: "calendar_rebuilding — syncapi's calendar routes still answer a structured 503 (untouched by calendar-v5 seams, #778)",
	}
	camp, err := a.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return out, nil
		}
		return out, err
	}
	out.Found = true
	out.CampaignName = camp.Name

	if a.addons != nil {
		if enabled, aerr := a.addons.IsEnabledForCampaign(ctx, campaignID, calendar.PluginSlug); aerr != nil {
			out.AddonNote = aerr.Error()
		} else {
			out.AddonEnabled = &enabled
		}
	} else {
		out.AddonNote = "the addons service is not wired into this adapter"
	}

	if a.pluginHealth != nil {
		if h := a.pluginHealth.Get(calendar.PluginSlug); h != nil {
			out.PluginHealthy = h.Healthy
			out.MigrationVersion = h.Version
			out.MigrationLatest = h.LatestVersion
		} else {
			out.Notes = append(out.Notes, "no plugin-health entry registered for \"calendar\" — the plugin's schema migrations may not have run at startup")
		}
	} else {
		out.Notes = append(out.Notes, "the plugin-health registry is not wired into this adapter, so migration state was NOT read")
	}

	if a.calendar == nil {
		out.Notes = append(out.Notes, "the calendar service is not wired into this adapter, so counts were NOT read")
		return out, nil
	}

	// A declared SYSTEM caller (ADR-049): a health check walks every
	// calendar's own rows, with no per-request identity behind it, the same
	// stated trust the campaign export adapter uses.
	const ownerRole = 3
	systemViewer := permissions.SystemViewer(ownerRole)

	cals, err := a.calendar.ListCalendars(ctx, campaignID, systemViewer)
	if err != nil {
		out.Notes = append(out.Notes, "listing calendars failed: "+apperror.SafeMessage(err))
		return out, nil
	}
	out.CalendarCount = len(cals)

	for _, c := range cals {
		full, ferr := a.calendar.GetCalendarForViewer(ctx, c.ID, campaignID, systemViewer)
		if ferr != nil {
			out.Notes = append(out.Notes, "reading calendar "+c.ID+" failed: "+apperror.SafeMessage(ferr))
			continue
		}
		out.MoonCount += len(full.Moons)
		out.EraCount += len(full.Eras)

		events, eerr := a.calendar.ListAllEventsForCalendar(ctx, c.ID, campaignID, systemViewer)
		if eerr != nil {
			out.Notes = append(out.Notes, "listing events for calendar "+c.ID+" failed: "+apperror.SafeMessage(eerr))
			continue
		}
		out.EventCount += len(events)
	}

	// Event kinds are campaign-scoped (shared by every calendar in the
	// campaign — see calendar.EventKind's own doc comment), so this is one
	// read, not per-calendar.
	kinds, kerr := a.calendar.ListEventKinds(ctx, campaignID)
	if kerr != nil {
		out.Notes = append(out.Notes, "listing event kinds failed: "+apperror.SafeMessage(kerr))
	} else {
		out.EventKindCount = len(kinds)
	}

	return out, nil
}
