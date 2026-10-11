// calendar_kind_integration_test.go runs the calendar block against the real
// calendar service and a scratch MariaDB schema: the create path's import,
// the update path's structure save, and events written right after a new
// calendar. Skips when no test DB server answers (`make test-db-up`).
package records

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	mrand "math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// openCalendarTestDB makes a scratch schema with core and calendar
// migrations applied; one small copy per package, as the repo does.
func openCalendarTestDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "chronicle", "chronicle", "tcp", "127.0.0.1:3306"
		raw = cfg.FormatDSN()
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
	}
	cfg.ParseTime = true
	server := *cfg
	server.DBName = ""
	admin, err := sql.Open("mysql", server.FormatDSN())
	if err != nil {
		t.Skipf("no test DB: %v", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable: %v", err)
	}
	name := fmt.Sprintf("chronicle_aical_%06d", mrand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	root, _ := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err := database.RunMigrations(db, scratch.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Fatalf("core migrations: %v", err)
	}
	sub, err := fs.Sub(calendar.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: calendar.PluginSlug, MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("calendar migrations: %v", res.Error)
		}
	}
	return db
}

func TestCalendarKind_Integration(t *testing.T) {
	db := openCalendarTestDB(t)
	ctx := context.Background()
	user, campaign := "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`, []any{user, "aical@example.test", "AI Cal", "x"}},
		{`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`, []any{campaign, "AI Cal", "ai-cal", user}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	svc := calendar.NewCalendarService(calendar.NewCalendarRepository(db), calendar.NewEventRepository(db),
		calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	a := Actor{UserID: user, Role: 3}
	reg := NewRegistry(CalendarKind{Svc: svc}, EventKind{Svc: svc})

	// One paste: the calendar, then an event in a month only it defines.
	recs := []Record{
		rec(KindCalendar, ActionCreate, "Harptos", newCalendarFields(), ""),
		rec("event", ActionCreate, "Greengrass", map[string]any{"year": 1492, "month": "Ches", "day": 1}, ""),
	}
	kinds, plans := reg.PlanAll(ctx, campaign, a, recs)
	for i, p := range plans {
		if p.Error != "" {
			t.Fatalf("row %d: %+v", i, p)
		}
		if err := kinds[i].Apply(ctx, campaign, a, recs[i]); err != nil {
			t.Fatalf("apply row %d: %v", i, err)
		}
	}
	cal, err := svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	if err != nil {
		t.Fatalf("the new calendar is not the main one: %v", err)
	}
	if len(cal.Months) != 3 || len(cal.Weekdays) != 2 || len(cal.Moons) != 1 || len(cal.Seasons) != 2 || len(cal.Eras) != 1 {
		t.Fatalf("structure: %d months %d weekdays %d moons %d seasons %d eras", len(cal.Months), len(cal.Weekdays), len(cal.Moons), len(cal.Seasons), len(cal.Eras))
	}
	if cal.CurrentMonth != 2 || cal.CurrentDay != 5 || cal.EpochName == nil || *cal.EpochName != "DR" {
		t.Fatalf("today %d/%d label %v", cal.CurrentMonth, cal.CurrentDay, cal.EpochName)
	}
	evs, _ := svc.ListEventsForCalendar(ctx, campaign, cal.ID, 3)
	if len(evs) != 1 || evs[0].Month != 3 {
		t.Fatalf("events %+v", evs)
	}
	moonID := cal.Moons[0].ID

	// Insert a month before Ches: the event follows Ches to its new place,
	// the moon keeps its id, and a new era is added.
	up := rec(KindCalendar, ActionUpdate, "", map[string]any{
		"months": []any{
			map[string]any{"name": "Hammer", "days": 30},
			map[string]any{"name": "Alturiak", "days": 30},
			map[string]any{"name": "Midwinter", "days": 1, "intercalary": true},
			map[string]any{"name": "Ches", "days": 30},
		},
		"moons":         []any{map[string]any{"name": "Selune", "cycle": 30}},
		"eras":          []any{map[string]any{"name": "Age of Humanity", "start_year": -2000, "end_year": 0}},
		"current_month": "Ches", "current_year": 1492, "current_day": 10,
	}, "")
	k := CalendarKind{Svc: svc}
	if p := k.Plan(ctx, campaign, a, up); p.Error != "" {
		t.Fatalf("update plan: %+v", p)
	}
	if err := k.Apply(ctx, campaign, a, up); err != nil {
		t.Fatalf("update: %v", err)
	}
	cal, _ = svc.GetDefaultCalendarForViewer(ctx, campaign, a.Viewer())
	if len(cal.Months) != 4 || cal.Moons[0].ID != moonID || len(cal.Eras) != 2 || cal.CurrentMonth != 4 || cal.CurrentDay != 10 {
		t.Fatalf("after update: %d months, moon %d (was %d), %d eras, today %d/%d", len(cal.Months), cal.Moons[0].ID, moonID, len(cal.Eras), cal.CurrentMonth, cal.CurrentDay)
	}
	evs, _ = svc.ListEventsForCalendar(ctx, campaign, cal.ID, 3)
	if len(evs) != 1 || evs[0].Month != 4 {
		t.Fatalf("event did not follow its month: %+v", evs)
	}
}
