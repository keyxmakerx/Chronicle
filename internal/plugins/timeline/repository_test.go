// Pins timeline EventCount visibility with two complementary tests:
//
//  1. TestEventCountVisibility_MatchesListFilters — a DB-less structural pin
//     (parses repository.go, no live SQL) that fails if the visibility
//     fragment is removed from the count subqueries. Runs in CI.
//  2. TestTimelineEventCount_Integration — a real-MariaDB behavioral proof
//     that List/ListByCalendar's EventCount agrees with the merged service
//     read (ListTimelineEvents) for a player viewer with a dm_only event and
//     a dm_only link override in scope. Skipped under `-short`. Run with
//     `make docker-up && make migrate-up && make test-int`.
package timeline

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	mrand "math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- structural guard (no database needed) ---

// funcSource returns the raw source text of the named top-level function in
// filename (relative to this package dir — the same convention the calendar
// package's event_scan_contract_test.go already relies on for `go test`'s
// working directory).
func funcSource(t *testing.T, filename, fnName string) string {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", filename, err)
	}

	var body string
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name == nil || fd.Name.Name != fnName {
			return true
		}
		body = string(src[fset.Position(fd.Pos()).Offset:fset.Position(fd.End()).Offset])
		found = true
		return false
	})
	if !found {
		t.Fatalf("function %s not found in %s — has it been restructured?", fnName, filename)
	}
	return body
}

var flattenWS = regexp.MustCompile(`\s+`)

// flatten collapses whitespace runs to a single space (COMMON §3's safe
// source-text-pin pattern) so this guard survives reflow/reindent instead of
// pinning exact multi-line formatting.
func flatten(s string) string {
	return strings.TrimSpace(flattenWS.ReplaceAllString(s, " "))
}

// TestEventCountVisibility_MatchesListFilters pins that List and
// ListByCalendar's EventCount subqueries carry the same conditional
// visibility fragment their row-returning siblings (ListEventLinks /
// ListStandaloneEvents) use, including the link-level override: a count
// that only checks ce.visibility would still count a link a GM overrode to
// dm_only even though the row reads hide it. COALESCE(NULLIF(tel
// .visibility_override, ''), ce.visibility) is the SQL mirror of
// EffectiveVisibility().
func TestEventCountVisibility_MatchesListFilters(t *testing.T) {
	for _, fn := range []string{"List", "ListByCalendar"} {
		body := flatten(funcSource(t, "repository.go", fn))
		for _, want := range []string{
			`standaloneVisFilter := "AND te.visibility = 'everyone'"`,
		} {
			if !strings.Contains(body, flatten(want)) {
				t.Errorf("%s: EventCount subquery missing %q — the count can silently drift from ListStandaloneEvents again", fn, want)
			}
		}

		// CALV5-PLACEHOLDER: the linked half of this count is dark while the
		// calendar tables are dropped. This asserts that dark state; when V5
		// reintroduces the calendar_events join, it must also restore the
		// linked visibility fragment in the same change, or a player's count
		// will again include events their rows don't show.
		if strings.Contains(body, flatten("JOIN calendar_events")) {
			if !strings.Contains(body, flatten(`COALESCE(NULLIF(tel.visibility_override, ''), ce.visibility) = 'everyone'`)) {
				t.Errorf("%s: the linked-event count is back WITHOUT its visibility fragment — "+
					"that is the counting oracle this test exists for. Restore the filter, or "+
					"read links through the calendar service instead of joining its table.", fn)
			}
		}
		// Both filters must be CLEARED (not just declared) for a DM-capable
		// viewer, exactly like every other visFilter in this file — a
		// declared-but-never-cleared fragment would permanently under-count
		// for owners too.
		if !strings.Contains(body, "permissions.CanSeeDmOnly(role)") {
			t.Errorf("%s: EventCount subquery filters must be gated on permissions.CanSeeDmOnly(role), same as the row queries", fn)
		}
	}
}

// --- real-MariaDB behavioral proof (see file doc comment for why this is
// skipped rather than run in this sandbox / in CI) ---

// TestTimelineEventCount_Integration exercises List/ListByCalendar against a
// real MariaDB with a public linked event, a dm_only linked event, a public
// linked event whose link is overridden to dm_only
// (timeline_event_links.visibility_override), and a dm_only standalone
// event. Asserts EventCount equals the row count the real service method
// (ListTimelineEvents: repo filter + EffectiveVisibility()) returns for both
// a player and an owner viewer. Links carrying visibility_rules remain
// outside this agreement (resolved in Go per user; see List's doc comment).
//
// CALV5-PLACEHOLDER: event links are dark until V5 restores the
// calendar_events join, so these counts cover only the standalone dm_only
// event. With the join back, the wants are player 1, owner 4, ListByCalendar 1.
func TestTimelineEventCount_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewTimelineRepository(db)

	userID := testUUID(t)
	campaignID := testUUID(t)
	calendarID := testUUID(t)
	timelineID := testUUID(t)
	publicEventID := testUUID(t)
	secretEventID := testUUID(t)
	overriddenEventID := testUUID(t)
	secretStandaloneID := testUUID(t)

	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "tiefix-int-"+userID+"@example.test", "TieFix Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "TieFix Int Test", "tiefix-int-"+campaignID[:8], userID)
	// CASCADE on campaigns → calendars/timelines/... means deleting the
	// campaign clears everything below it; users has no cascade.
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, userID)
	}()

	mustExec(t, db, `INSERT INTO calendars (id, campaign_id, name) VALUES (?, ?, ?)`,
		calendarID, campaignID, "Test Calendar")
	mustExec(t, db, `INSERT INTO calendar_events (id, calendar_id, name, year, month, day, visibility) VALUES (?, ?, ?, 1, 1, 1, 'everyone')`,
		publicEventID, calendarID, "Public Event")
	mustExec(t, db, `INSERT INTO calendar_events (id, calendar_id, name, year, month, day, visibility) VALUES (?, ?, ?, 1, 1, 2, 'dm_only')`,
		secretEventID, calendarID, "Secret Event")
	mustExec(t, db, `INSERT INTO calendar_events (id, calendar_id, name, year, month, day, visibility) VALUES (?, ?, ?, 1, 1, 4, 'everyone')`,
		overriddenEventID, calendarID, "Overridden Event")
	mustExec(t, db, `INSERT INTO timelines (id, campaign_id, calendar_id, name) VALUES (?, ?, ?, ?)`,
		timelineID, campaignID, calendarID, "Test Timeline")

	if err := repo.LinkEvent(ctx, &EventLink{TimelineID: timelineID, EventID: publicEventID}); err != nil {
		t.Fatalf("link public event: %v", err)
	}
	if err := repo.LinkEvent(ctx, &EventLink{TimelineID: timelineID, EventID: secretEventID}); err != nil {
		t.Fatalf("link secret event: %v", err)
	}
	// The §7 case: the EVENT is 'everyone' but the GM overrode this LINK to
	// dm_only. The service's EffectiveVisibility() hides it from a player's
	// rows; the count subquery must agree or the link-level oracle is back.
	if err := repo.LinkEvent(ctx, &EventLink{TimelineID: timelineID, EventID: overriddenEventID}); err != nil {
		t.Fatalf("link overridden event: %v", err)
	}
	dmOnly := "dm_only"
	if err := repo.UpdateEventLinkVisibility(ctx, timelineID, overriddenEventID, &dmOnly, nil); err != nil {
		t.Fatalf("override link visibility: %v", err)
	}
	if err := repo.CreateEvent(ctx, &TimelineEvent{
		ID: secretStandaloneID, TimelineID: timelineID, Name: "Secret Standalone",
		Year: 1, Month: 1, Day: 3, Visibility: "dm_only",
	}); err != nil {
		t.Fatalf("create standalone event: %v", err)
	}

	// assertAgrees derives the row count from the real merged service read
	// (ListTimelineEvents, including the EffectiveVisibility() override
	// step) and checks List's EventCount against that, not a hardcoded
	// expectation, so it fails if either side drifts.
	svc := NewTimelineService(repo, nil, nil, nil)
	assertAgrees := func(t *testing.T, role int, wantCount int) {
		t.Helper()
		tls, err := repo.List(ctx, campaignID, role)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(tls) != 1 {
			t.Fatalf("expected exactly 1 timeline, got %d", len(tls))
		}

		rows, err := svc.ListTimelineEvents(ctx, timelineID, permissions.RequestViewer(role, userID))
		if err != nil {
			t.Fatalf("ListTimelineEvents: %v", err)
		}
		gotRows := len(rows)

		if gotRows != wantCount {
			t.Fatalf("role %d: len(ListTimelineEvents) = %d, want %d (fixture drift, not the bug under test)", role, gotRows, wantCount)
		}
		if tls[0].EventCount != gotRows {
			t.Errorf("role %d: List's EventCount (%d) and the actual row count (%d) DISAGREE — the visibility-oracle bug is back", role, tls[0].EventCount, gotRows)
		}
	}

	t.Run("player sees nothing (linked events dark, standalone is dm_only)", func(t *testing.T) {
		assertAgrees(t, permissions.RolePlayer, 0)
	})
	t.Run("owner sees only the standalone event (linked events dark)", func(t *testing.T) {
		assertAgrees(t, permissions.RoleOwner, 1)
	})

	// ListByCalendar carries the identical fix; confirm it agrees too.
	t.Run("ListByCalendar agrees for player", func(t *testing.T) {
		tls, err := repo.ListByCalendar(ctx, calendarID, permissions.RolePlayer)
		if err != nil {
			t.Fatalf("ListByCalendar: %v", err)
		}
		if len(tls) != 1 || tls[0].EventCount != 0 {
			t.Errorf("ListByCalendar (player): got %+v, want exactly 1 timeline with EventCount=0 (linked events dark, standalone is dm_only)", tls)
		}
	})
}

// --- DB test helpers (mirrors internal/plugins/entities/repository_integration_test.go verbatim) ---

// openTestDB returns a scratch schema with core, calendar and timeline
// migrations applied (timeline_event_links references calendar_events),
// dropped on cleanup. It never uses the DSN's own database: test-int-local's
// DSN names none.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		cfg := mysql.NewConfig()
		cfg.User = getenvDefault("DB_USER", "chronicle")
		cfg.Passwd = getenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = getenvDefault("DB_HOST", "127.0.0.1:3306")
		raw = cfg.FormatDSN()
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
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
		t.Skipf("no test DB server reachable at %s: %v — run `make docker-up` or `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_tl_%06d", mrand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })

	scratchCfg := *cfg
	scratchCfg.DBName = name
	db, err := sql.Open("mysql", scratchCfg.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if err := database.RunMigrations(db, scratchCfg.FormatDSN(), filepath.Join(root, "db", "migrations")); err != nil {
		t.Skipf("core migrations did not apply: %v", err)
	}
	sub, err := fs.Sub(MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	// timeline_event_links.event_id references calendar_events(id), so the
	// calendar plugin's schema has to exist first. Loaded off disk rather
	// than by importing the calendar package, mirroring the sessions
	// plugin's scratch-schema helper, which keeps this test fixture from
	// depending on another plugin's exported Go surface.
	calDir := os.DirFS(filepath.Join(root, "internal", "plugins", "calendar", "migrations"))
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: "calendar", MigrationsFS: calDir},
		{Slug: "timeline", MigrationsFS: sub},
	}) {
		if !res.Healthy {
			t.Skipf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}
	return db
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func testUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
