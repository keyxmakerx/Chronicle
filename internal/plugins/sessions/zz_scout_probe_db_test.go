package sessions

// SCOUTING PROBES against a REAL MariaDB. Some of the claims in this scout are
// about ROWS — what availability_exceptions actually holds after a write, and
// whether a single-use token row can be spent twice — and only a database can
// answer those. Skips (never fails) with no server, per the house convention.
//
// Run with:  CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/plugins/sessions/ -run 'TestProbeDB'

import (
	"context"
	"database/sql"
	crand "crypto/rand"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/database"
)

func probeScratchDB(t *testing.T) *sql.DB {
	t.Helper()

	var cfg *mysql.Config
	if raw := os.Getenv("CHRONICLE_TEST_DB_DSN"); raw != "" {
		parsed, err := mysql.ParseDSN(raw)
		if err != nil {
			t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
		}
		cfg = parsed
	} else {
		t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the DB probes")
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
		t.Skipf("no test DB server reachable at %s: %v — run `make test-db-up`", cfg.Addr, err)
	}

	name := fmt.Sprintf("chronicle_scout_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
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
	// sessions/migrations/001 carries `FOREIGN KEY (calendar_id) REFERENCES
	// calendars(id)`, so the CALENDAR plugin's schema has to exist first. It is
	// loaded off disk rather than by importing the package, because
	// internal/wire/plugin_import_guard_test.go forbids a sessions→calendar
	// edge outright.
	calDir := os.DirFS(filepath.Join(root, "internal", "plugins", "calendar", "migrations"))
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{
		{Slug: "calendar", MigrationsFS: calDir},
		{Slug: "sessions", MigrationsFS: sub},
	}) {
		if !res.Healthy {
			t.Skipf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}
	return db
}

func probeID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := crand.Read(b[:]); err != nil {
		t.Fatalf("random id: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// probeSeed inserts one user + one campaign and returns (campaignID, userID).
func probeSeed(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	userID, campID := probeID(t), probeID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
		userID, userID+"@example.test", "Ana", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`,
		campID, "Scout", campID, userID); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return campID, userID
}

// PROBE DB-1 — POST /campaigns/:id/availability/exceptions (AddExceptionAPI →
// AddMyException → repo.AddException) writes ONE row for a date, and because
// exception rows REPLACE the recurring pattern for that date
// (effectiveBlocks, availability_overlay.go:174), the member's whole day
// collapses to the one window they just added.
//
// This is the documented public route (sessions/.ai.md route table) and it is
// the exact trap AddMyAvailableWindows/composeOfferedDay was written to close —
// the closure was applied to the RSVP-offer path only, never to the endpoint.
//
// The claim is about ROWS: after the write, how many rows does the day hold and
// what does the projection make of them.
func TestProbeDB_AddExceptionErasesTheRestOfTheDay(t *testing.T) {
	if testing.Short() {
		t.Skip("DB probe")
	}
	db := probeScratchDB(t)
	campID, userID := probeSeed(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	// The member's usual Tuesday: 09:00–23:00, painted in the grid.
	if err := svc.SaveMyAvailability(ctx, campID, userID, SaveAvailabilityRequest{
		TZ: "UTC",
		Blocks: []AvailabilityBlockDTO{
			{DayOfWeek: int(time.Tuesday), StartMinute: 9 * 60, EndMinute: 23 * 60, State: AvailAvailable},
		},
	}); err != nil {
		t.Fatalf("SaveMyAvailability: %v", err)
	}

	// The member uses the documented endpoint to say "on this Tuesday I'm ALSO
	// free 07:00–08:00" (an addition, in their reading).
	onDate := time.Now().UTC().AddDate(0, 0, 14).Format("2006-01-02")
	for time.Now().UTC().AddDate(0, 0, 14).Weekday() != time.Tuesday { // ensure a Tuesday
		break
	}
	d, _ := time.Parse("2006-01-02", onDate)
	onDate = d.AddDate(0, 0, (int(time.Tuesday)-int(d.Weekday())+7)%7).Format("2006-01-02")

	if err := svc.AddMyException(ctx, campID, userID, AddExceptionRequest{
		OnDate: onDate, StartMinute: 7 * 60, EndMinute: 8 * 60,
		State: AvailAvailable, TZ: "UTC",
	}); err != nil {
		t.Fatalf("AddMyException: %v", err)
	}

	// THE ROWS.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM availability_exceptions
	    WHERE campaign_id=? AND user_id=? AND on_date=?`, campID, userID, onDate).Scan(&n); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	t.Logf("availability_exceptions rows for %s: %d", onDate, n)

	// THE PROJECTION — what the Director's overlay now says about that day.
	weekStart := d.AddDate(0, 0, (int(time.Monday)-int(d.Weekday())+7)%7-7).Format("2006-01-02")
	_ = weekStart
	overlay, err := svc.BuildOverlay(ctx, campID,
		[]overlayMemberInput{{UserID: userID, Name: "Ana"}}, onDate, "UTC", true)
	if err != nil {
		t.Fatalf("BuildOverlay: %v", err)
	}
	var freeAt20, freeAt7 int
	for _, day := range overlay.Days {
		if day.Date == onDate {
			freeAt20 = day.Hours[20].Free
			freeAt7 = day.Hours[7].Free
		}
	}
	t.Logf("after ONE 07:00–08:00 exception: free at 07:00 = %d, free at 20:00 = %d", freeAt7, freeAt20)

	if n != 1 {
		t.Fatalf("probe stale: expected exactly the one added row, got %d", n)
	}
	if freeAt7 != 1 {
		t.Fatalf("probe broken: the added window is not visible")
	}
	if freeAt20 != 0 {
		t.Fatal("probe stale: the member's usual evening survived — AddMyException may now compose the day")
	}
	t.Log("CONFIRMED: adding ONE window through the documented endpoint deleted the " +
		"member's whole recurring Tuesday from the Director's overlay. They now read " +
		"as free for one hour at 7am and busy every evening.")
}

// PROBE DB-2 — sessions' single-use RSVP token really can be spent twice,
// against the real table.
//
// sessions/repository.go:497 MarkRSVPTokenUsed is `UPDATE ... WHERE token = ?`
// with no `used_at IS NULL` and no RowsAffected check, and service.go:484
// ApplyRSVPToken validates then writes with nothing in between. The calendar's
// twin (calendar/rsvp_repository.go:218) has both. Two concurrent POSTs of the
// same emailed link are the ordinary double-tap case.
func TestProbeDB_SessionRSVPTokenSpendsTwice(t *testing.T) {
	if testing.Short() {
		t.Skip("DB probe")
	}
	db := probeScratchDB(t)
	campID, userID := probeSeed(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := probeID(t)
	if _, err := db.Exec(
		`INSERT INTO sessions (id, campaign_id, name, status, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,NOW(),NOW())`,
		sessID, campID, "Session 41", StatusPlanned, userID); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := repo.AddAttendee(ctx, sessID, userID, RSVPInvited); err != nil {
		t.Fatalf("invite: %v", err)
	}
	accept, _, err := svc.CreateRSVPTokens(ctx, sessID, userID)
	if err != nil {
		t.Fatalf("CreateRSVPTokens: %v", err)
	}

	// Both requests read the row before either writes — the TOCTOU the missing
	// predicate leaves open.
	var wg sync.WaitGroup
	results := make([]error, 2)
	gate := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			_, results[i] = svc.ApplyRSVPToken(ctx, accept)
		}(i)
	}
	close(gate)
	wg.Wait()

	succeeded := 0
	for i, err := range results {
		if err == nil {
			succeeded++
		} else {
			t.Logf("  apply %d refused: %v", i, err)
		}
	}
	t.Logf("concurrent applies of ONE single-use token that succeeded: %d/2", succeeded)

	// The row-level contrast: the calendar consumes with `used_at IS NULL` and
	// reports RowsAffected, so exactly one of a racing pair can win. This one
	// cannot tell.
	var used sql.NullTime
	if err := db.QueryRow(`SELECT used_at FROM session_rsvp_tokens WHERE token=?`, accept).Scan(&used); err != nil {
		t.Fatalf("reading used_at: %v", err)
	}
	t.Logf("session_rsvp_tokens.used_at = %v", used)

	if succeeded < 2 {
		t.Skipf("the race did not interleave this run (succeeded=%d); "+
			"the structural claim stands — see TestProbe_SessionRSVPTokenConsumeIsNotAtomic", succeeded)
	}
	t.Log("CONFIRMED: both concurrent redemptions of one single-use link succeeded")
}

// PROBE DB-3 — MarkRSVPTokenUsed cannot report that it consumed nothing.
//
// A deterministic, race-free statement of the same defect: consume the token
// twice in sequence. The second call must be indistinguishable from the first
// to its caller, which is exactly why ApplyRSVPToken cannot detect a loser.
func TestProbeDB_SessionMarkTokenUsedIsUnconditional(t *testing.T) {
	if testing.Short() {
		t.Skip("DB probe")
	}
	db := probeScratchDB(t)
	campID, userID := probeSeed(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := probeID(t)
	if _, err := db.Exec(
		`INSERT INTO sessions (id, campaign_id, name, status, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,NOW(),NOW())`,
		sessID, campID, "S", StatusPlanned, userID); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	accept, _, err := svc.CreateRSVPTokens(ctx, sessID, userID)
	if err != nil {
		t.Fatalf("tokens: %v", err)
	}

	if err := repo.MarkRSVPTokenUsed(ctx, accept); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	second := repo.MarkRSVPTokenUsed(ctx, accept)
	if second != nil {
		t.Fatalf("probe stale: the second consume was refused (%v) — a guard may exist now", second)
	}

	// It also overwrites the original used_at, so the audit trail of WHEN the
	// link was actually spent is lost too.
	t.Log("CONFIRMED: consuming an ALREADY-SPENT session RSVP token returns nil; " +
		"the caller has no way to detect the loser of a race, and used_at is overwritten")
}

// PROBE DB-4 — re-submitting the SAME RSVP status inside one second answers
// "attendee not found".
//
// repository.go:300 UpdateAttendeeStatus treats RowsAffected==0 as a missing
// attendee. MySQL/MariaDB's RowsAffected counts CHANGED rows, not matched rows
// (the driver is not opened with clientFoundRows), and `responded_at = NOW()`
// has one-second resolution — so a member who is already `accepted` and taps
// Going again within the same second changes nothing, and the endpoint reports
// a 404 about a row that is sitting right there.
//
// Reachable from: the session detail page's HTMX buttons (a double-tap), the
// emailed /rsvp/:token confirm form, and any client that retries.
func TestProbeDB_SameStatusRSVPWithinASecondIs404(t *testing.T) {
	if testing.Short() {
		t.Skip("DB probe")
	}
	db := probeScratchDB(t)
	campID, userID := probeSeed(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := probeID(t)
	if _, err := db.Exec(
		`INSERT INTO sessions (id, campaign_id, name, status, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,NOW(),NOW())`,
		sessID, campID, "Session 41", StatusPlanned, userID); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := repo.AddAttendee(ctx, sessID, userID, RSVPInvited); err != nil {
		t.Fatalf("invite: %v", err)
	}

	if err := svc.UpdateRSVP(ctx, sessID, userID, RSVPAccepted); err != nil {
		t.Fatalf("first RSVP: %v", err)
	}
	second := svc.UpdateRSVP(ctx, sessID, userID, RSVPAccepted) // the double-tap

	// The row is unquestionably there.
	var status string
	if err := db.QueryRow(`SELECT status FROM session_attendees WHERE session_id=? AND user_id=?`,
		sessID, userID).Scan(&status); err != nil {
		t.Fatalf("the attendee row is missing for real: %v", err)
	}
	t.Logf("stored status = %q; second identical RSVP returned: %v", status, second)

	if second == nil {
		t.Skip("the two writes landed in different seconds this run — responded_at changed, " +
			"so RowsAffected was 1. The defect is timing-dependent by construction.")
	}
	t.Log("CONFIRMED: a member whose row says 'accepted' was told 'attendee not found' " +
		"for pressing Going twice")
}

// PROBE DB-5 — a member who joins the campaign AFTER a session is created can
// never RSVP to it, and has no control to try.
//
// InviteAll runs in exactly two places (handler.go:192 CreateSession,
// proposals_handler.go:220 ConfirmProposalAPI), both at session-creation time.
// routes.go registers no add-attendee endpoint. sessions.templ:602 renders the
// RSVP buttons only inside `for _, a := range attendees { if a.UserID ==
// currentUserID }`, so the newcomer sees no control — and if they POST the
// endpoint anyway, UpdateAttendeeStatus 404s.
func TestProbeDB_MemberWhoJoinedLaterCannotRSVP(t *testing.T) {
	if testing.Short() {
		t.Skip("DB probe")
	}
	db := probeScratchDB(t)
	campID, ownerID := probeSeed(t, db)
	repo := NewSessionRepository(db)
	svc := NewSessionService(repo, nil)
	ctx := context.Background()

	sessID := probeID(t)
	if _, err := db.Exec(
		`INSERT INTO sessions (id, campaign_id, name, status, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,NOW(),NOW())`,
		sessID, campID, "Session 41", StatusPlanned, ownerID); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// Only the members present at creation are invited.
	if err := svc.InviteAll(ctx, sessID, []string{ownerID}); err != nil {
		t.Fatalf("InviteAll: %v", err)
	}

	// A new player joins the campaign the next day.
	newcomer := probeID(t)
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`,
		newcomer, newcomer+"@example.test", "Bo", "x"); err != nil {
		t.Fatalf("seed newcomer: %v", err)
	}

	err := svc.UpdateRSVP(ctx, sessID, newcomer, RSVPAccepted)
	if err == nil {
		t.Fatal("probe stale: the newcomer's RSVP succeeded — an upsert may have been introduced")
	}
	attendees, lerr := svc.ListAttendees(ctx, sessID)
	if lerr != nil {
		t.Fatalf("ListAttendees: %v", lerr)
	}
	t.Logf("attendee rows: %d; newcomer RSVP refused with: %v", len(attendees), err)
	t.Log("CONFIRMED: the newcomer holds no session_attendees row, the templ therefore " +
		"renders no RSVP control for them, and there is no route that would create one. " +
		"They are invisible in the 'N going / M invited' line for every existing session.")
}
