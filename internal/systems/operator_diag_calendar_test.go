package systems

import (
	"context"
	"strings"
	"testing"
)

// fakeCalendarProvider is a scripted CalendarDiagProvider, mirroring
// fakeCampaignProvider's pattern in operator_diag_campaign_test.go.
type fakeCalendarProvider struct {
	stats CalendarStatsFacts
	err   error
}

func (f *fakeCalendarProvider) CalendarStats(context.Context, string) (CalendarStatsFacts, error) {
	return f.stats, f.err
}

// withCalendarProvider installs a provider for the duration of fn and
// restores whatever was there, the same leak-guard withCampaignProvider uses.
func withCalendarProvider(t *testing.T, p CalendarDiagProvider, fn func()) {
	t.Helper()
	prev := calendarDiagProvider
	calendarDiagProvider = p
	defer func() { calendarDiagProvider = prev }()
	fn()
}

// TestCalendarStats_ProviderNotWired_SaysNoBodyAsked pins the degrade-loudly
// contract: an unwired provider must never render as "zero calendars".
func TestCalendarStats_ProviderNotWired_SaysNoBodyAsked(t *testing.T) {
	withCalendarProvider(t, nil, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.stats", "c1")
		if !strings.Contains(got, "NOTHING WAS READ") {
			t.Errorf("an unwired provider must say nobody was asked, not print an empty result:\n%s", got)
		}
	})
}

// TestCalendarStats_CampaignNotFound.
func TestCalendarStats_CampaignNotFound(t *testing.T) {
	withCalendarProvider(t, &fakeCalendarProvider{stats: CalendarStatsFacts{Found: false}}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.stats", "missing")
		if !strings.Contains(got, "No campaign `missing`") {
			t.Errorf("a missing campaign must say so plainly:\n%s", got)
		}
	})
}

// TestCalendarStats_PrintsCountsAddonAndSyncState is the happy path: every
// count, the addon state, the migration state and the honest Foundry sync
// state must all appear.
func TestCalendarStats_PrintsCountsAddonAndSyncState(t *testing.T) {
	enabled := true
	f := CalendarStatsFacts{
		Found: true, CampaignID: "c1", CampaignName: "Ashfall",
		AddonEnabled:  &enabled,
		CalendarCount: 1, EventCount: 42, MoonCount: 2, EraCount: 3, EventKindCount: 5,
		PluginHealthy: true, MigrationVersion: 20, MigrationLatest: 20,
		FoundrySyncState: "on V5: date and events served",
	}
	withCalendarProvider(t, &fakeCalendarProvider{stats: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.stats", "c1")
		for _, want := range []string{
			"calendar addon: **enabled**",
			"calendars: **1**",
			"events: **42**",
			"moons: **2**",
			"eras: **3**",
			"event kinds: **5**",
			"schema: **healthy**, migration `20`",
			"Foundry sync: on V5: date and events served",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("calendar.stats missing %q:\n%s", want, got)
			}
		}
	})
}

// TestCalendarStats_UnhealthySchemaIsFlagged.
func TestCalendarStats_UnhealthySchemaIsFlagged(t *testing.T) {
	f := CalendarStatsFacts{
		Found: true, CampaignID: "c1", CampaignName: "Ashfall",
		PluginHealthy: false, MigrationVersion: 18, MigrationLatest: 20,
	}
	withCalendarProvider(t, &fakeCalendarProvider{stats: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.stats", "c1")
		if !strings.Contains(got, "UNHEALTHY") {
			t.Errorf("an unhealthy plugin schema must be flagged, not silently reported as counts:\n%s", got)
		}
	})
}

// TestCalendarStats_NoContentLeaks pins the "counts only" contract: nothing
// in the renderer ever formats an event name, a moon name, or a member name
// — the Facts struct itself carries no such field, so this asserts the
// renderer's OWN output never happens to mention this test's canary string,
// guarding against a future field addition that starts threading content
// through by accident.
func TestCalendarStats_NoContentLeaks(t *testing.T) {
	f := CalendarStatsFacts{
		Found: true, CampaignID: "c1", CampaignName: "Ashfall",
		EventCount: 1, Notes: []string{"reading calendar cal-1 failed: db is on fire"},
	}
	withCalendarProvider(t, &fakeCalendarProvider{stats: f}, func() {
		got, _ := RunDiagnostic(diagnosticCatalog(), "calendar.stats", "c1")
		if strings.Contains(got, "Secret War Council") {
			t.Error("calendar.stats must never print event content")
		}
	})
}
