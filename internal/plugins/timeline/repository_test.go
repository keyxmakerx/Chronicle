// repository_test.go — C-CALV4-TIEFIX-PB Bug 2 guard: the timeline
// EventCount visibility fix. Two complementary tests:
//
//  1. TestEventCountVisibility_MatchesListFilters — a DB-less structural pin
//     (parses repository.go, no live SQL) that fails the instant someone
//     removes the visibility fragment from the count subqueries again. This
//     is the one that actually runs in CI.
//  2. TestTimelineEventCount_Integration — a real-MariaDB behavioral proof
//     that List/ListByCalendar's EventCount agrees with
//     ListEventLinks+ListStandaloneEvents' row counts for a player viewer
//     with a dm_only event in scope. Mirrors the two pre-existing DB
//     integration tests in internal/plugins/entities (same DSN/skip
//     conventions, copied verbatim) — like those two, it is skipped under
//     `-short` and therefore never runs in CI either; it was NOT executed in
//     this sandbox (no docker/MariaDB available here — `docker ps` fails
//     with "no such file or directory" on the docker socket). Run with
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
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

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

// TestEventCountVisibility_MatchesListFilters pins C-CALV4-TIEFIX-PB Bug 2:
// List and ListByCalendar's EventCount subqueries must carry the SAME
// conditional visibility fragment their row-returning siblings
// (ListEventLinks / ListStandaloneEvents) already use. Before the fix, the
// two COUNT(*) subqueries were unconditional — a player would see e.g. "12
// events" while the actual row reads returned only 9, the difference being a
// count of hidden events (a visibility oracle, cordinator#32-class leak).
func TestEventCountVisibility_MatchesListFilters(t *testing.T) {
	for _, fn := range []string{"List", "ListByCalendar"} {
		body := flatten(funcSource(t, "repository.go", fn))
		for _, want := range []string{
			"JOIN calendar_events ce ON ce.id = tel.event_id",
			`linkedEventVisFilter := "AND ce.visibility = 'everyone'"`,
			`standaloneVisFilter := "AND te.visibility = 'everyone'"`,
		} {
			if !strings.Contains(body, flatten(want)) {
				t.Errorf("%s: EventCount subquery missing %q — the count can silently drift from ListEventLinks/ListStandaloneEvents again", fn, want)
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

// TestTimelineEventCount_Integration exercises List/ListByCalendar +
// ListEventLinks/ListStandaloneEvents against a real MariaDB with one
// timeline carrying a public linked event, a dm_only linked event, and a
// dm_only standalone event — the exact "player viewer, dm_only event in
// scope" scenario the dispatch's guard asks for. Asserts EventCount equals
// the actual row count for BOTH a player and an owner viewer, so a
// regression that silently reintroduces the unconditional COUNT(*) (or
// over-corrects and hides events from owners) fails loudly here instead of
// shipping unnoticed, same rationale as the entities package's two
// integration tests.
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
	mustExec(t, db, `INSERT INTO timelines (id, campaign_id, calendar_id, name) VALUES (?, ?, ?, ?)`,
		timelineID, campaignID, calendarID, "Test Timeline")

	if err := repo.LinkEvent(ctx, &EventLink{TimelineID: timelineID, EventID: publicEventID}); err != nil {
		t.Fatalf("link public event: %v", err)
	}
	if err := repo.LinkEvent(ctx, &EventLink{TimelineID: timelineID, EventID: secretEventID}); err != nil {
		t.Fatalf("link secret event: %v", err)
	}
	if err := repo.CreateEvent(ctx, &TimelineEvent{
		ID: secretStandaloneID, TimelineID: timelineID, Name: "Secret Standalone",
		Year: 1, Month: 1, Day: 3, Visibility: "dm_only",
	}); err != nil {
		t.Fatalf("create standalone event: %v", err)
	}

	// assertAgrees is the actual "count and list agree" proof: it derives the
	// row count from the SAME two queries the merged ListTimelineEvents
	// service method reads from, and checks List's EventCount against that —
	// not against a hardcoded expectation — so it fails if EITHER side drifts.
	assertAgrees := func(t *testing.T, role int, wantCount int) {
		t.Helper()
		tls, err := repo.List(ctx, campaignID, role)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(tls) != 1 {
			t.Fatalf("expected exactly 1 timeline, got %d", len(tls))
		}

		links, err := repo.ListEventLinks(ctx, timelineID, role)
		if err != nil {
			t.Fatalf("ListEventLinks: %v", err)
		}
		standalone, err := repo.ListStandaloneEvents(ctx, timelineID, role)
		if err != nil {
			t.Fatalf("ListStandaloneEvents: %v", err)
		}
		gotRows := len(links) + len(standalone)

		if gotRows != wantCount {
			t.Fatalf("role %d: len(ListEventLinks)+len(ListStandaloneEvents) = %d, want %d (fixture drift, not the bug under test)", role, gotRows, wantCount)
		}
		if tls[0].EventCount != gotRows {
			t.Errorf("role %d: List's EventCount (%d) and the actual row count (%d) DISAGREE — the visibility-oracle bug is back", role, tls[0].EventCount, gotRows)
		}
	}

	t.Run("player sees only the public linked event", func(t *testing.T) {
		assertAgrees(t, permissions.RolePlayer, 1)
	})
	t.Run("owner sees all three events", func(t *testing.T) {
		assertAgrees(t, permissions.RoleOwner, 3)
	})

	// ListByCalendar carries the identical fix; confirm it agrees too.
	t.Run("ListByCalendar agrees for player", func(t *testing.T) {
		tls, err := repo.ListByCalendar(ctx, calendarID, permissions.RolePlayer)
		if err != nil {
			t.Fatalf("ListByCalendar: %v", err)
		}
		if len(tls) != 1 || tls[0].EventCount != 1 {
			t.Errorf("ListByCalendar (player): got %+v, want exactly 1 timeline with EventCount=1", tls)
		}
	})
}

// --- DB test helpers (mirrors internal/plugins/entities/repository_integration_test.go verbatim) ---

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if dsn == "" {
		cfg := mysql.NewConfig()
		cfg.User = getenvDefault("DB_USER", "chronicle")
		cfg.Passwd = getenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = getenvDefault("DB_HOST", "127.0.0.1:3306")
		cfg.DBName = getenvDefault("DB_NAME", "chronicle")
		cfg.ParseTime = true
		dsn = cfg.FormatDSN()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("no test DB reachable at %s (ping: %v) — run `make docker-up && make migrate-up`", maskDSN(dsn), err)
	}
	return db
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// maskDSN hides the password when reporting a skipped/failed connection.
func maskDSN(dsn string) string {
	if cfg, err := mysql.ParseDSN(dsn); err == nil {
		return cfg.Addr + "/" + cfg.DBName
	}
	return "configured DSN"
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
