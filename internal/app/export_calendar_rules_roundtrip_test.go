// export_calendar_rules_roundtrip_test.go proves a campaign backup carries
// events that repeat by a rule and their "this one only" overrides, and that
// a rule's references to moons, seasons and other events are pointed at the
// re-created ones on import rather than at ids from the old campaign.
package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCalendarExportImport_RecurrenceRulesAndOverrides(t *testing.T) {
	byRule := calendar.RecurrenceByRule
	yearly := calendar.RecurrenceYearly
	two := 2
	newY, newM, newD := 1000, 2, 3
	src := &fakeCalendarService{
		cal: &calendar.Calendar{
			ID: "cal-1", CampaignID: "c1", Mode: calendar.ModeFantasy, Name: "Rules",
			HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
			Months:   []calendar.Month{{Name: "One", Days: 30}, {Name: "Two", Days: 30}},
			Weekdays: []calendar.Weekday{{Name: "A"}, {Name: "B"}},
			Moons:    []calendar.Moon{{ID: 7, Name: "Luna", CycleDays: 29.5, Color: "#ffffff"}},
			Seasons:  []calendar.Season{{ID: 9, Name: "Spring", StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30, Color: "#22c55e"}},
		},
		// The dependent event comes BEFORE its anchor, so the importer must
		// order its creates itself.
		events: []calendar.Event{
			{
				ID: "old-aftermath", Name: "Masks Aftermath", Year: 1000, Month: 1, Day: 1, Visibility: "everyone",
				IsRecurring: true, RecurrenceType: &byRule,
				RecurrenceRule: &calendar.RecurrenceRule{
					Match:      []calendar.RuleCondition{{Kind: calendar.RuleRelativeToEvent, EventID: "old-masks"}},
					OffsetDays: 2,
				},
			},
			{
				ID: "old-masks", Name: "Festival of Masks", Year: 1000, Month: 1, Day: 10, Visibility: "everyone",
				IsRecurring: true, RecurrenceType: &yearly,
			},
			{
				ID: "old-rite", Name: "Moon Rite", Year: 1000, Month: 1, Day: 1, Visibility: "dm_only",
				IsRecurring: true, RecurrenceType: &byRule,
				RecurrenceRule: &calendar.RecurrenceRule{
					Match: []calendar.RuleCondition{
						{Kind: calendar.RuleMoonPhase, MoonID: 7, Phase: "full"},
						{Kind: calendar.RuleAfterEvent, EventID: "old-masks", Days: &two},
					},
				},
			},
			{
				ID: "old-fair", Name: "Spring Fair", Year: 1000, Month: 1, Day: 1, Visibility: "everyone",
				IsRecurring: true, RecurrenceType: &byRule,
				RecurrenceRule: &calendar.RecurrenceRule{
					Match: []calendar.RuleCondition{{Kind: calendar.RuleSeasonStart, SeasonID: 9}},
				},
			},
		},
		overrides: map[string][]calendar.OccurrenceOverride{
			"old-masks": {
				{EventID: "old-masks", Year: 1001, Month: 1, Day: 10, Action: calendar.OverrideSkip},
				{EventID: "old-masks", Year: 1002, Month: 1, Day: 10, Action: calendar.OverrideMove, NewYear: &newY, NewMonth: &newM, NewDay: &newD},
			},
		},
	}

	calData, err := (&calendarExportAdapter{svc: src}).ExportCalendar(context.Background(), "c1", func(string) string { return "" })
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := json.Marshal(&campaigns.CampaignExport{Format: campaigns.ExportFormat, Version: campaigns.ExportVersion, Calendar: calData})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var reloaded campaigns.CampaignExport
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if reloaded.Calendar.Moons[0].Ref != 7 || reloaded.Calendar.Seasons[0].Ref != 9 {
		t.Fatalf("export lost moon/season refs: %+v %+v", reloaded.Calendar.Moons, reloaded.Calendar.Seasons)
	}

	dst := &fakeCalendarService{}
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: dst}).ImportCalendar(context.Background(), "c2", reloaded.Calendar, campaigns.NewIDMap("c2"), report); err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("import reported failures: %s", report.Summary())
	}

	order := map[string]int{}
	rules := map[string]*calendar.RecurrenceRule{}
	for i, in := range dst.createdEvents {
		order[in.Name] = i
		if len(in.RecurrenceRule) > 0 {
			r, err := calendar.ParseRecurrenceRule(in.RecurrenceRule)
			if err != nil {
				t.Fatalf("%s: imported rule does not parse: %v", in.Name, err)
			}
			rules[in.Name] = r
		}
	}
	if order["Festival of Masks"] > order["Masks Aftermath"] || order["Festival of Masks"] > order["Moon Rite"] {
		t.Errorf("anchor was created after an event that repeats relative to it: %v", order)
	}

	tests := []struct {
		event string
		check func(r *calendar.RecurrenceRule) bool
	}{
		{"Masks Aftermath", func(r *calendar.RecurrenceRule) bool {
			return r.Match[0].EventID == "evt-Festival of Masks" && r.OffsetDays == 2
		}},
		{"Moon Rite", func(r *calendar.RecurrenceRule) bool {
			return r.Match[0].MoonID == 200 && r.Match[1].EventID == "evt-Festival of Masks" && *r.Match[1].Days == 2
		}},
		{"Spring Fair", func(r *calendar.RecurrenceRule) bool { return r.Match[0].SeasonID == 300 }},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			r := rules[tt.event]
			if r == nil || !tt.check(r) {
				t.Errorf("rule not remapped to the imported ids: %+v", r)
			}
		})
	}

	if len(dst.setOverrides) != 2 {
		t.Fatalf("imported %d overrides, want 2", len(dst.setOverrides))
	}
	for _, o := range dst.setOverrides {
		if o.eventID != "evt-Festival of Masks" {
			t.Errorf("override attached to %q, want the re-created anchor", o.eventID)
		}
	}
	if mv := dst.setOverrides[1]; mv.input.Action != calendar.OverrideMove || mv.input.Year != 1000 || mv.input.Month != 2 || mv.input.Day != 3 || mv.occ.Year != 1002 {
		t.Errorf("move override lost its dates: %+v", mv)
	}
}
