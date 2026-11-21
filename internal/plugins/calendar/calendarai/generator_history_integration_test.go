// generator_history_integration_test.go runs a history row's write against
// the real calendar service and a scratch MariaDB schema: the eras and
// events go in, hidden history never reaches a player's reads, and a rerun
// adds nothing. Skips when no test DB server answers (`make test-db-up`).
package calendarai

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestGeneratorKind_HistoryIntegration(t *testing.T) {
	db := openCalendarTestDB(t)
	ctx := context.Background()
	user, campaign := "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, []any{user, "aihist@example.test", "AI Hist", "x"}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, []any{campaign, "AI Hist", "ai-hist", user}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	svc := calendar.NewCalendarService(calendar.NewCalendarRepository(db), calendar.NewEventRepository(db),
		calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	a := Actor{UserID: user, Role: 3}

	fields := newCalendarFields()
	delete(fields, "eras")
	cr := rec(KindCalendar, ActionCreate, "Harptos", fields, "")
	if err := (CalendarKind{Svc: svc}).Apply(ctx, campaign, a, cr); err != nil {
		t.Fatalf("calendar: %v", err)
	}

	k := GeneratorKind{Cal: svc, Eras: svc}
	r := histRec(nil)
	if p := k.Plan(ctx, campaign, a, r); p.Error != "" || p.Client == "" {
		t.Fatalf("plan: %+v", p)
	}
	r.Generated = `{"eras":[` +
		`{"name":"The Founding","start_year":1,"end_year":1000,"description":"Walls and wells.","color":"#bc6449"},` +
		`{"name":"The Present Age","start_year":1001,"end_year":null,"description":"Now.","color":"oklch(0.6 0.1 40)"}],` +
		`"events":[` +
		`{"name":"Harrow founded","year":12,"month":1,"day":3,"description":"The first charter is signed.","visibility":"everyone"},` +
		`{"name":"The king is poisoned","year":1003,"month":2,"day":4,"description":"Nobody saw the cup.","visibility":"dm_only"},` +
		`{"name":"Treaty of Ford","year":1490,"month":1,"day":9,"visibility":"everyone"}]}`
	if err := k.Apply(ctx, campaign, a, r); err != nil {
		t.Fatalf("apply: %v", err)
	}

	cal, err := svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	if err != nil {
		t.Fatal(err)
	}
	if len(cal.Eras) != 2 || cal.Eras[0].Color != "#bc6449" || cal.Eras[1].EndYear != nil || cal.Eras[1].Color != "" {
		t.Fatalf("eras %+v", cal.Eras)
	}
	all, _ := svc.ListEventsForCalendar(ctx, campaign, cal.ID, 3)
	if len(all) != 3 {
		t.Fatalf("owner sees %d events, want 3", len(all))
	}
	players, _ := svc.ListEventsForCalendar(ctx, campaign, cal.ID, 1)
	month, _ := svc.ListEventsForMonth(ctx, cal.ID, campaign, 1003, 2, permissions.RequestViewer(1, "pl"))
	for _, e := range append(players, month...) {
		if e.Name == "The king is poisoned" {
			t.Fatal("hidden history reached a player")
		}
	}
	if len(players) != 2 {
		t.Fatalf("player sees %d events, want 2", len(players))
	}

	// The calendar now has its eras, so a rerun is events only, and every
	// event is already there.
	if err := k.Apply(ctx, campaign, a, r); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	cal, _ = svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	all, _ = svc.ListEventsForCalendar(ctx, campaign, cal.ID, 3)
	if len(cal.Eras) != 2 || len(all) != 3 {
		t.Fatalf("after rerun: %d eras, %d events; want 2 and 3", len(cal.Eras), len(all))
	}
}
