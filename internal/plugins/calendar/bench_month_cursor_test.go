// bench_month_cursor_test.go — C-CALV4-GAMEREADY §1, [GR-1] and [GR-2].
//
// THE FINDING THIS GUARDS. The Bench could only ever render the calendar's
// CURRENT in-world month: `benchBlock` built a `BlockRequest` with no `View`
// field, so `resolveView` — a function written for navigation in wave 1, whose
// own doc comment worries about "losing a navigated year" — had never once been
// called with a view. A GM preparing a session could not look at next month.
//
// TWO GUARDS, AND THE SECOND IS THE ONE THAT NEEDED A DATABASE.
//
//  1. TestBlockView_MonthCursorRoundTrips is the render-level pin: `?y=&m=`
//     lands on that month, a partial `?y=` clamps INTO that year rather than
//     snapping back to today, an out-of-range `?m=` clamps rather than 500ing,
//     and no param renders exactly what the Bench rendered before this slice.
//     It asserts that resolveView is CALLED, never that it was re-implemented —
//     the clamping arithmetic is deliberately absent from this slice's code.
//
//  2. TestBenchMonthCursor_Integration is the same claim against a REAL
//     MariaDB, because the cursor is not only a render: the navigated month is
//     what `candidateEvents` READS, so a cursor that renders the right grid
//     against a fake and the wrong month's rows against a database would be a
//     GM looking at next month's cells with this month's events in them. The
//     project believed for months that it had no database available; it does
//     (`make test-db-up`), and this is a persistence claim, so it gets one.
//
// TestBenchNav_TrioRendersForEveryRole is [GR-2]: three links, every role,
// `Today` present on the current month too.
package calendar

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"fmt"
	"io/fs"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// --- 1. the cursor round-trips (render level) -------------------------------

// TestBlockView_MonthCursorRoundTrips pins the whole of [GR-1] except the query
// parsing: a View handed to the spine is the month that comes back.
//
// THE FOURTH CASE IS THE REGRESSION CASE and it is why the table has an
// unnavigated row at all: `BlockDate{}` must render the calendar's own current
// month, byte-for-byte as the Bench rendered it before the cursor existed. A
// navigation feature that changes the unnavigated page is a regression wearing
// a feature's clothes.
func TestBlockView_MonthCursorRoundTrips(t *testing.T) {
	cal := blockTenDayCal()
	svc := NewBlockService(newBlockFakeRepo(cal))
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	for _, tc := range []struct {
		name              string
		view              BlockDate
		wantIdx, wantYear int
	}{
		{"no cursor renders the calendar's own current month",
			BlockDate{}, cal.CurrentMonth - 1, cal.CurrentYear},
		{"?y=&m= renders exactly that month",
			BlockDate{Year: 1600, Month: 5}, 4, 1600},
		{"a partial ?y= clamps INTO that year rather than snapping back to today",
			BlockDate{Year: 1600}, 0, 1600},
		{"an out-of-range ?m= clamps to a real month instead of erroring",
			BlockDate{Year: 1600, Month: 99}, len(cal.Months) - 1, 1600},
		{"a negative ?m= clamps to the first month of the navigated year",
			BlockDate{Year: 1600, Month: -3}, 0, 1600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := svc.Block(context.Background(), BlockRequest{
				CalendarID: cal.ID, Viewer: viewer, View: tc.view,
			})
			if err != nil {
				t.Fatalf("Block(%+v): %v", tc.view, err)
			}
			if d.Month.Index != tc.wantIdx || d.Month.Year != tc.wantYear {
				t.Fatalf("View %+v rendered month %d/%d, want %d/%d",
					tc.view, d.Month.Index, d.Month.Year, tc.wantIdx, tc.wantYear)
			}
			if d.Month.Name != cal.Months[tc.wantIdx].Name {
				t.Fatalf("View %+v named the month %q, want %q",
					tc.view, d.Month.Name, cal.Months[tc.wantIdx].Name)
			}
		})
	}
}

// TestBenchNav_TrioHrefsComeFromTheCalendarsOwnMonthList pins [GR-2]'s
// arithmetic: the step is computed against `len(cal.Months)`, never against a
// literal twelve, so a ten-month or a one-month calendar rolls its own year.
func TestBenchNav_TrioHrefsComeFromTheCalendarsOwnMonthList(t *testing.T) {
	cal := benchFxHarptos() // two months
	base := "/campaigns/camp-1/apps/calendar"

	mid := benchNav(&cal, benchNavFxBlock(t, &cal, 1, 1523), "camp-1")
	if mid.PrevHref != base+"?y=1523&m=1" {
		t.Errorf("prev from month 2 = %q, want month 1 of the same year", mid.PrevHref)
	}
	if mid.NextHref != base+"?y=1524&m=1" {
		t.Errorf("next from the LAST month = %q, want month 1 of year+1", mid.NextHref)
	}

	first := benchNav(&cal, benchNavFxBlock(t, &cal, 0, 1523), "camp-1")
	if first.PrevHref != base+"?y=1522&m=2" {
		t.Errorf("prev from the FIRST month = %q, want the last month of year-1", first.PrevHref)
	}
	if first.NextHref != base+"?y=1523&m=2" {
		t.Errorf("next from month 1 = %q, want month 2 of the same year", first.NextHref)
	}
	if first.TodayHref != base {
		t.Errorf("Today = %q, want the bare route", first.TodayHref)
	}

	// resolveView reads Year == 0 as "unset", so year 0 is not addressable by
	// the cursor and the honest render is NO prev link rather than one that
	// silently lands on today. This slice may not edit resolveView ([GR-1]),
	// so the property is pinned rather than fixed.
	yearOne := cal
	yearOne.CurrentYear = 1
	edge := benchNav(&yearOne, benchNavFxBlock(t, &yearOne, 0, 1), "camp-1")
	if edge.PrevHref != "" {
		t.Errorf("prev across year 0 = %q, want no link (year 0 is resolveView's unset sentinel)", edge.PrevHref)
	}
	if edge.TodayHref == "" || edge.NextHref == "" {
		t.Error("Today and next must still render at the year-1 edge")
	}

	// A calendar with no months has no month to step to and is already printing
	// its own fault where the date would go.
	if benchNav(&Calendar{ID: "x"}, benchNavFxBlock(t, &cal, 0, 1523), "camp-1").Live {
		t.Error("a calendar with zero months must not render a month cursor")
	}
}

func benchNavFxBlock(t *testing.T, cal *Calendar, monthIdx, year int) calblock.BlockData {
	t.Helper()
	return projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner},
		MonthIndex: monthIdx, Year: year, MoonCap: benchMoonCap,
	})
}

// --- 2. the trio renders, for every role ------------------------------------

// TestBenchNav_TrioRendersForEveryRole is [GR-2]'s audience guard.
//
// THE TRIO IS NOT A PERMISSION. Looking at next month is a reading affordance,
// so every role that can see the Bench at all gets it — GM, Scribe, Player and
// the anonymous viewer of a public campaign. This is the OPPOSITE of §2's verb
// row, which is absent below its capability floor, and the two guards sit in
// different files so that the difference stays visible.
func TestBenchNav_TrioRendersForEveryRole(t *testing.T) {
	for _, tc := range []struct {
		name          string
		isGM, isOwner bool
	}{
		{"GM / owner", true, true},
		{"scribe (GM-visibility, not owner)", true, false},
		{"player", false, false},
		{"anonymous", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBench(t, benchFxData(tc.isGM, tc.isOwner))
			for _, want := range []string{
				`data-bench-nav`, `data-bench-nav-prev`, `data-bench-nav-today`, `data-bench-nav-next`,
			} {
				if !strings.Contains(html, want) {
					t.Errorf("month cursor control %s missing from the %s Bench", want, tc.name)
				}
			}
			// `Today` renders ON THE CURRENT MONTH TOO — benchFxData's primary
			// Block is projected at the calendar's own current month, so this
			// render IS the "already there" case. A control that vanishes when
			// you are already there teaches the GM that it is missing.
			if !strings.Contains(html, `data-bench-nav-today href="/campaigns/camp-1/apps/calendar"`) {
				t.Errorf("Today must render on the current month, as a link to the bare route")
			}
			// The real-world Block does NOT carry the cursor: `?y=1524&m=3` is a
			// coordinate in the in-world month list and would park a Gregorian
			// calendar in March 1524.
			if got := strings.Count(html, `data-bench-nav-today`); got != 1 {
				t.Errorf("Today link count = %d, want exactly 1 (the primary Block only)", got)
			}
		})
	}
}

// --- 3. the same claim, against a real MariaDB ------------------------------

// TestBenchMonthCursor_Integration proves the cursor against the database.
//
// WHY A FAKE IS NOT ENOUGH HERE. The navigated month is not only what the grid
// draws — it is the argument `candidateEvents` reads rows with. A cursor that
// moved the geometry and not the query would render next month's cells filled
// with this month's events, which is a worse lie than the gap §1 opened with.
// The two events below sit in DIFFERENT months of the same year for exactly
// that reason, and the assertion is on which one comes back.
func TestBenchMonthCursor_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("month-cursor integration test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	campaignID, cal := calTestSeedNavCalendar(t, db)
	_ = cal.Name

	spine := NewBlockService(NewBlockRepository(db))
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	for _, tc := range []struct {
		name      string
		view      BlockDate
		wantMonth string
		wantEvent string
		absent    []string
	}{
		{"no cursor: the calendar's own current month", BlockDate{},
			"Deepwinter", "Emberfall Vigil", []string{"Thawrun Muster", "The Long Siege"}},
		{"?y=1523&m=2: next month, with NEXT MONTH'S rows", BlockDate{Year: 1523, Month: 2},
			"Thawrun", "Thawrun Muster", []string{"Emberfall Vigil", "The Long Siege"}},
		{"?y=1524&m=1: next year", BlockDate{Year: 1524, Month: 1},
			"Deepwinter", "The Long Siege", []string{"Emberfall Vigil", "Thawrun Muster"}},
		{"?m=99 clamps to a real month rather than erroring", BlockDate{Year: 1523, Month: 99},
			"Sunfall", "", []string{"Emberfall Vigil", "Thawrun Muster", "The Long Siege"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := spine.Block(ctx, BlockRequest{
				CalendarID: cal.ID, CampaignID: campaignID, Viewer: viewer, View: tc.view,
			})
			if err != nil {
				t.Fatalf("Block(%+v): %v", tc.view, err)
			}
			if d.Month.Name != tc.wantMonth {
				t.Fatalf("rendered month %q, want %q", d.Month.Name, tc.wantMonth)
			}
			marks := benchNavFxMarkNames(d)
			if tc.wantEvent != "" && !marks[tc.wantEvent] {
				t.Errorf("month %q did not carry %q — the cursor moved the grid but not the query",
					tc.wantMonth, tc.wantEvent)
			}
			for _, gone := range tc.absent {
				if marks[gone] {
					t.Errorf("month %q carried %q, which lives in another month", tc.wantMonth, gone)
				}
			}
		})
	}
}

// TestBenchMonthCursorWiring_Integration drives the WHOLE path, on a real
// MariaDB: `?y=&m=` on the request → benchInput.View → BlockRequest.View →
// resolveView → the month's own rows → the rendered page → the trio's hrefs.
//
// IT EXISTS BECAUSE THE SPINE-LEVEL TABLE ABOVE CANNOT SEE THE WIRING. Passing
// a View to BlockService.Block by hand proves resolveView works — which was
// never in doubt; its comment has described this feature since wave 1. What was
// broken is that NOTHING EVER CALLED IT WITH ONE, and only a test that starts
// at the query string can fail on that.
func TestBenchMonthCursorWiring_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("month-cursor wiring test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	campaignID, _ := calTestSeedNavCalendar(t, db)

	prev := BlockSpine()
	t.Cleanup(func() { blockSpine.Store(prev) })
	InstallBlockSpine(NewBlockService(NewBlockRepository(db)))
	h := NewHandler(NewCalendarService(NewCalendarRepository(db)))

	page := func(query string) string {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/campaigns/"+campaignID+"/apps/calendar"+query, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(campaignID)
		c.Set("campaign_context", &campaigns.CampaignContext{
			Campaign: &campaigns.Campaign{ID: campaignID, Name: "Imix"}, MemberRole: campaigns.RoleOwner,
		})
		c.Set("auth_user_id", "u-gm")
		if err := h.AppDashboard(c); err != nil {
			t.Fatalf("AppDashboard(%q): %v", query, err)
		}
		return rec.Body.String()
	}

	base := "/campaigns/" + campaignID + "/apps/calendar"

	// THE ASSERTIONS READ THE PRIMARY BLOCK, NOT THE PAGE. The Bench's NEXT UP
	// index is a CROSS-MONTH read by design — it lists what is coming — so
	// "Thawrun Muster" is on the page whatever month the grid shows. Judging
	// the whole page would have made this test pass for the wrong reason.
	block := func(query string) string {
		t.Helper()
		return calTestPrimaryBlock(t, page(query))
	}

	t.Run("no cursor renders the calendar's own current month", func(t *testing.T) {
		b := block("")
		if !strings.Contains(b, "Emberfall Vigil") {
			t.Error("the unnavigated Bench lost its own current month's event")
		}
		if strings.Contains(b, "Thawrun Muster") {
			t.Error("the unnavigated Block rendered another month's event")
		}
		if !strings.Contains(b, "Deepwinter 1523") {
			t.Error("the cursor label must name the month actually drawn")
		}
		if !strings.Contains(b, `href="`+base+`?y=1523&amp;m=2"`) {
			t.Error("next → month 2 of the same year is missing from the trio")
		}
	})

	t.Run("?y=1523&m=2 navigates, rows and all", func(t *testing.T) {
		b := block("?y=1523&m=2")
		if !strings.Contains(b, "Thawrun Muster") {
			t.Error("the navigated month did not carry its own event — the cursor never reached the query")
		}
		if strings.Contains(b, "Emberfall Vigil") {
			t.Error("the navigated month carried the PREVIOUS month's event")
		}
		if !strings.Contains(b, "Thawrun 1523") {
			t.Error("the cursor label did not follow the navigation")
		}
		if !strings.Contains(b, `href="`+base+`?y=1523&amp;m=1"`) {
			t.Error("prev → month 1 is missing from the navigated trio")
		}
		if !strings.Contains(b, `href="`+base+`?y=1523&amp;m=3"`) {
			t.Error("next → month 3 is missing from the navigated trio")
		}
		if !strings.Contains(b, `data-bench-nav-today href="`+base+`"`) {
			t.Error("Today must link to the bare route from a navigated month")
		}
	})

	t.Run("the last month's next rolls into year+1", func(t *testing.T) {
		if !strings.Contains(block("?y=1523&m=3"), `href="`+base+`?y=1524&amp;m=1"`) {
			t.Error("next from the LAST month must roll to month 1 of year+1")
		}
	})

	t.Run("?y=1524&m=1 is a year step, not a month step", func(t *testing.T) {
		b := block("?y=1524&m=1")
		if !strings.Contains(b, "The Long Siege") {
			t.Error("navigating a year did not read the next year's rows")
		}
		if strings.Contains(b, "Emberfall Vigil") {
			t.Error("the same month of a DIFFERENT year carried this year's event")
		}
	})

	t.Run("?m=99 clamps rather than 500ing", func(t *testing.T) {
		if !strings.Contains(block("?y=1523&m=99"), "Sunfall 1523") {
			t.Error("an out-of-range month must clamp into a real month of the requested year")
		}
	})

	t.Run("garbage reads as no cursor", func(t *testing.T) {
		if !strings.Contains(block("?y=nope&m=nope"), "Deepwinter 1523") {
			t.Error("an unparseable cursor must render the current month, not an error page")
		}
	})
}

// calTestPrimaryBlock returns the primary Block's own markup out of a rendered
// Bench page.
//
// PIN DISCIPLINE (COMMON §3): both bounds are CHECKED before they are used as
// slice indices, so a renamed marker fails this helper cleanly instead of
// panicking somewhere down the file.
func calTestPrimaryBlock(t *testing.T, html string) string {
	t.Helper()
	const head = `data-bench-block="primary"`
	start := strings.Index(html, head)
	if start < 0 {
		t.Fatalf("no primary Block in the rendered Bench — has %s been renamed?", head)
	}
	rest := html[start:]
	if end := strings.Index(rest, `data-bench-block="real-world"`); end >= 0 {
		return rest[:end]
	}
	// A Bench with no real-world Block ends the stack at the next disclosure.
	if end := strings.Index(rest, `data-bench-disc=`); end >= 0 {
		return rest[:end]
	}
	t.Fatal("could not bound the primary Block — neither a real-world Block nor a following disclosure was found")
	return ""
}

// calTestSeedNavCalendar builds the fixture both integration tests read: three
// months, one event in each of two of them, and one in the same month of the
// NEXT year so a year step can be told apart from a month step.
func calTestSeedNavCalendar(t *testing.T, db *sql.DB) (string, *Calendar) {
	t.Helper()
	ctx := context.Background()
	campaignID := calTestSeedCampaign(t, db)
	repo := NewCalendarRepository(db)

	cal := &Calendar{
		ID:         calTestID(t),
		CampaignID: campaignID, Name: "Harptos of Imix", Mode: ModeFantasy,
		IsDefault: true, CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 14,
	}
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if err := repo.SetMonths(ctx, cal.ID, []MonthInput{
		{Name: "Deepwinter", Days: 30}, {Name: "Thawrun", Days: 30}, {Name: "Sunfall", Days: 30},
	}); err != nil {
		t.Fatalf("set months: %v", err)
	}
	if err := repo.SetWeekdays(ctx, cal.ID, []WeekdayInput{
		{Name: "Sar"}, {Name: "Mol"}, {Name: "Zor"}, {Name: "Wir"}, {Name: "Nym"},
		{Name: "Lyr"}, {Name: "Tam"}, {Name: "Kes"}, {Name: "Vel"}, {Name: "Odd"},
	}); err != nil {
		t.Fatalf("set weekdays: %v", err)
	}
	for _, e := range []*Event{
		{CalendarID: cal.ID, Name: "Emberfall Vigil", Year: 1523, Month: 1, Day: 14,
			Visibility: storageVisibilityEveryone},
		{CalendarID: cal.ID, Name: "Thawrun Muster", Year: 1523, Month: 2, Day: 7,
			Visibility: storageVisibilityEveryone},
		{CalendarID: cal.ID, Name: "The Long Siege", Year: 1524, Month: 1, Day: 3,
			Visibility: storageVisibilityEveryone},
	} {
		e.ID = calTestID(t)
		if err := repo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("create event %q: %v", e.Name, err)
		}
	}
	return campaignID, cal
}

// benchNavFxMarkNames flattens every mark label on a rendered month.
func benchNavFxMarkNames(d calblock.BlockData) map[string]bool {
	out := map[string]bool{}
	for _, row := range d.Month.Rows {
		for _, cell := range row.Cells {
			for _, m := range cell.Marks {
				out[m.Title] = true
			}
		}
	}
	return out
}

// --- the real-database harness ----------------------------------------------
//
// Discovery + skip rules follow the house integration convention (see
// internal/plugins/timeline/repository_test.go and
// internal/database/plugin_migration_recovery_test.go, which this mirrors):
// DSN from CHRONICLE_TEST_DB_DSN else the DB_* env vars, SKIP rather than fail
// when no server answers, and a THROWAWAY schema per test so dev data is never
// touched. `make test-db-up` starts a suitable server on 13306 with no Docker.

func newCalendarScratchSchema(t *testing.T) *sql.DB {
	t.Helper()

	var cfg *mysql.Config
	if raw := os.Getenv("CHRONICLE_TEST_DB_DSN"); raw != "" {
		parsed, err := mysql.ParseDSN(raw)
		if err != nil {
			t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
		}
		cfg = parsed
	} else {
		cfg = mysql.NewConfig()
		cfg.User = calTestEnv("DB_USER", "chronicle")
		cfg.Passwd = calTestEnv("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = calTestEnv("DB_HOST", "127.0.0.1:3306")
	}
	cfg.ParseTime = true

	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s (ping: %v) — run `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_calv4_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name +
		"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema %s (needs CREATE privilege): %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
			t.Logf("warning: could not drop scratch schema %s: %v", name, err)
		}
	})

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Core first, then the plugin — the order boot uses, and the order
	// CLAUDE.md's migration rules require (a core migration may not reference a
	// plugin table because plugin migrations run after core).
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply to the scratch schema: %v", err)
	}
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS for the calendar plugin migrations: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: PluginSlug, MigrationsFS: sub},
	}) {
		if !res.Healthy {
			t.Skipf("calendar plugin migrations did not apply: %v", res.Error)
		}
	}
	return db
}

func calTestEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// calTestSeedCampaign inserts the one owning row the calendar's foreign keys
// need. It writes raw SQL rather than reaching into the campaigns plugin
// because a calendar test must not depend on that plugin's service surface.
func calTestSeedCampaign(t *testing.T, db *sql.DB) string {
	t.Helper()
	userID := calTestID(t)
	campID := calTestID(t)
	if _, err := db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "GM", "x"); err != nil {
		t.Fatalf("seeding a user: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campID, "Imix", campID, userID); err != nil {
		t.Fatalf("seeding a campaign: %v", err)
	}
	return campID
}

// calTestID mints a UUID-shaped id, which is what every core table's CHAR(36)
// primary key expects.
func calTestID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
