// recurrence_integration_test.go: DB-backed checks for repeat rules and
// per-occurrence overrides — the parts whose correctness depends on real
// MariaDB: migration 022 re-applying and rolling back cleanly, the JSON rule
// column round-tripping through the repository, the override upsert and its
// FK cascade, and a month read surfacing a skip to the Director. Skips when
// no test DB answers (see openTestDB).
package calendar

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
)

// migrationStatements reads one of this plugin's migration files and splits
// it into statements, dropping comment lines first so a semicolon in prose
// cannot split a statement.
func migrationStatements(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile("migrations/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var kept []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	var out []string
	for _, stmt := range strings.Split(strings.Join(kept, "\n"), ";") {
		if s := strings.TrimSpace(stmt); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func execMigration(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	for _, stmt := range migrationStatements(t, name) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, stmt)
		}
	}
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&n); err != nil {
		t.Fatalf("information_schema: %v", err)
	}
	return n > 0
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&n); err != nil {
		t.Fatalf("information_schema: %v", err)
	}
	return n > 0
}

func TestRecurrenceMigration_Integration_IdempotentAndReversible(t *testing.T) {
	db := openTestDB(t)

	steps := []struct {
		name    string
		file    string
		present bool
	}{
		{"re-apply on top of itself", "022_event_recurrence_rules.up.sql", true},
		{"roll back", "022_event_recurrence_rules.down.sql", false},
		{"roll back twice", "022_event_recurrence_rules.down.sql", false},
		{"apply again", "022_event_recurrence_rules.up.sql", true},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			execMigration(t, db, st.file)
			if got := columnExists(t, db, "calendar_events", "recurrence_rule"); got != st.present {
				t.Errorf("recurrence_rule column present = %v, want %v", got, st.present)
			}
			if got := tableExists(t, db, "calendar_event_overrides"); got != st.present {
				t.Errorf("calendar_event_overrides present = %v, want %v", got, st.present)
			}
		})
	}
}

// seedRuleCalendar makes a two-month, seven-weekday calendar so a weekday
// rule has a known, small set of matching days.
func seedRuleCalendar(t *testing.T, db *sql.DB, campaignID string) *Calendar {
	t.Helper()
	ctx := context.Background()
	calRepo := NewCalendarRepository(db)
	cal := &Calendar{ID: testUUID(t), CampaignID: campaignID, Mode: ModeFantasy, Name: "Rule Calendar",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone"}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	if err := calRepo.SetMonths(ctx, cal.ID, []MonthInput{
		{Name: "Alpha", Days: 28, SortOrder: 0},
		{Name: "Beta", Days: 28, SortOrder: 1},
	}); err != nil {
		t.Fatalf("seed months: %v", err)
	}
	weekdays := make([]WeekdayInput, 7)
	for i := range weekdays {
		weekdays[i] = WeekdayInput{Name: fmt.Sprintf("Day%d", i+1), SortOrder: i}
	}
	if err := calRepo.SetWeekdays(ctx, cal.ID, weekdays); err != nil {
		t.Fatalf("seed weekdays: %v", err)
	}
	return cal
}

func TestRecurrenceRepository_Integration_RuleAndOverrides(t *testing.T) {
	db := openTestDB(t)
	fixture := newTestCampaign(t, db, "rule-repo")
	cal := seedRuleCalendar(t, db, fixture.CampaignID)
	eventRepo := NewEventRepository(db)
	svc := NewCalendarService(NewCalendarRepository(db), eventRepo, NewEventKindRepository(db), NewWeatherRepository(db))
	ctx := context.Background()

	wd := 2
	byRule := RecurrenceByRule
	rule := &RecurrenceRule{Match: []RuleCondition{{Kind: RuleWeekday, Weekday: &wd}}, Every: 2, OffsetDays: 1}
	evt := &Event{ID: testUUID(t), CalendarID: cal.ID, Name: "Market", Year: 1, Month: 1, Day: 1,
		Visibility: "everyone", IsRecurring: true, RecurrenceType: &byRule, RecurrenceRule: rule, CreatedBy: &fixture.UserID}
	if err := eventRepo.CreateEvent(ctx, evt); err != nil {
		t.Fatalf("create rule event: %v", err)
	}

	t.Run("rule round-trips through the JSON column", func(t *testing.T) {
		got, err := eventRepo.GetEvent(ctx, evt.ID)
		if err != nil {
			t.Fatalf("GetEvent: %v", err)
		}
		r := got.RecurrenceRule
		if r == nil || len(r.Match) != 1 || r.Match[0].Kind != RuleWeekday || r.Match[0].Weekday == nil ||
			*r.Match[0].Weekday != 2 || r.Every != 2 || r.OffsetDays != 1 {
			t.Fatalf("rule did not round-trip: %+v", r)
		}
		rules, err := eventRepo.ListRuleEvents(ctx, cal.ID)
		if err != nil || len(rules) != 1 || rules[0].ID != evt.ID {
			t.Errorf("ListRuleEvents = %v, %v; want the one rule event", rules, err)
		}
	})

	t.Run("an event without a rule reads back nil", func(t *testing.T) {
		plain := &Event{ID: testUUID(t), CalendarID: cal.ID, Name: "Plain", Year: 1, Month: 1, Day: 5,
			Visibility: "everyone", CreatedBy: &fixture.UserID}
		if err := eventRepo.CreateEvent(ctx, plain); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := eventRepo.GetEvent(ctx, plain.ID)
		if err != nil {
			t.Fatalf("GetEvent: %v", err)
		}
		if got.RecurrenceRule != nil {
			t.Errorf("rule = %+v, want nil", got.RecurrenceRule)
		}
	})

	// Find two natural occurrences through the service so the test does not
	// re-derive the weekday arithmetic.
	owner := ownerViewer(fixture.UserID)
	month, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 1, 1, owner)
	if err != nil {
		t.Fatalf("ListEventsForMonth: %v", err)
	}
	var occs []Occurrence
	for _, e := range month {
		if e.ID == evt.ID {
			occs = e.Occurrences
		}
	}
	if len(occs) < 2 {
		t.Fatalf("want at least two occurrences in the first month, got %+v", occs)
	}
	first := DayDate{Year: occs[0].Year, Month: occs[0].Month, Day: occs[0].Day}

	t.Run("override upserts on its occurrence key", func(t *testing.T) {
		if _, err := svc.SetOccurrenceOverride(ctx, evt.ID, cal.ID, fixture.CampaignID, first,
			OccurrenceOverrideInput{Action: OverrideMove, Year: 1, Month: 2, Day: 20}, owner); err != nil {
			t.Fatalf("move: %v", err)
		}
		if _, err := svc.SetOccurrenceOverride(ctx, evt.ID, cal.ID, fixture.CampaignID, first,
			OccurrenceOverrideInput{Action: OverrideSkip}, owner); err != nil {
			t.Fatalf("skip (replacing the move): %v", err)
		}
		got, err := eventRepo.ListOverridesForEvents(ctx, []string{evt.ID})
		if err != nil {
			t.Fatalf("ListOverridesForEvents: %v", err)
		}
		list := got[evt.ID]
		if len(list) != 1 || list[0].Action != OverrideSkip || list[0].NewYear != nil {
			t.Fatalf("want one skip row with no target, got %+v", list)
		}
	})

	t.Run("the Director sees the skip, a player does not", func(t *testing.T) {
		tests := []struct {
			name        string
			viewer      func() []Event
			wantSkipped bool
		}{
			{"owner", func() []Event {
				evs, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 1, 1, owner)
				if err != nil {
					t.Fatal(err)
				}
				return evs
			}, true},
			{"player", func() []Event {
				evs, err := svc.ListEventsForMonth(ctx, cal.ID, fixture.CampaignID, 1, 1, playerViewer("u-player"))
				if err != nil {
					t.Fatal(err)
				}
				return evs
			}, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var sawSkipped, sawFirst bool
				for _, e := range tt.viewer() {
					if e.ID != evt.ID {
						continue
					}
					for _, o := range e.Occurrences {
						if o.Year == first.Year && o.Month == first.Month && o.Day == first.Day {
							sawFirst = true
							sawSkipped = o.Skipped
						}
					}
				}
				if tt.wantSkipped && (!sawFirst || !sawSkipped) {
					t.Errorf("owner should see the occurrence flagged skipped")
				}
				if !tt.wantSkipped && sawFirst {
					t.Errorf("player must not see a skipped occurrence at all")
				}
			})
		}
	})

	t.Run("undo deletes the override; a second undo is NotFound", func(t *testing.T) {
		if err := svc.DeleteOccurrenceOverride(ctx, evt.ID, cal.ID, fixture.CampaignID, first, owner); err != nil {
			t.Fatalf("delete: %v", err)
		}
		err := svc.DeleteOccurrenceOverride(ctx, evt.ID, cal.ID, fixture.CampaignID, first, owner)
		assertIntegrationNotFound(t, err)
	})

	t.Run("deleting the event cascades its overrides", func(t *testing.T) {
		if _, err := svc.SetOccurrenceOverride(ctx, evt.ID, cal.ID, fixture.CampaignID, first,
			OccurrenceOverrideInput{Action: OverrideSkip}, owner); err != nil {
			t.Fatalf("skip: %v", err)
		}
		if err := eventRepo.DeleteEvent(ctx, evt.ID); err != nil {
			t.Fatalf("DeleteEvent: %v", err)
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM calendar_event_overrides WHERE event_id = ?`, evt.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%d override rows survived their event", n)
		}
	})
}
