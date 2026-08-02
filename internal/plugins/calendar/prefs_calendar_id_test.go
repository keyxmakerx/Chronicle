// Tests for the calendar_active preference writers' FOREIGN KEY, which is the
// reason the Block's layer switches, the Bench's disclosures and the sidebar pin
// all "did nothing" on a live instance.
//
// THE BUG, PRECISELY. calendar_active.calendar_id is `VARCHAR(36) NOT NULL` with
// `CONSTRAINT fk_calendar_active_cal FOREIGN KEY (calendar_id) REFERENCES
// calendars(id)` (migrations/006_active_calendar.up.sql:17,22-23; nothing later
// relaxes it — 007/014/016 only ADD COLUMN). All three preference writers
// inserted the literal empty string there as a documented "fallback on first
// write". No calendar has that id, InnoDB validates the foreign key on the
// ATTEMPTED INSERT — before the duplicate-key path resolves, so ON DUPLICATE KEY
// UPDATE never rescued it — and the write failed with errno 1452. handler_v2.go
// returned the raw error, app.go turned it into a 500, no HX-Refresh came back,
// aria-pressed never moved, and the operator saw a switch that did nothing.
//
// WHY NOTHING CAUGHT IT. Every existing test of these paths mocks the repository,
// so the SQL was never executed or even read; and nothing in the repo executes
// real SQL at all (no sqlmock, no dockertest in go.mod). The tests here close
// exactly that gap and no more: a recording driver stands in for MariaDB and
// captures the statement and the arguments database/sql actually sends, so the
// assertion is on the emitted parameters rather than on a mock's promise.
//
// WHAT THEY STILL CANNOT PROVE, stated plainly: no MariaDB runs in this build
// environment, so the FK is not EXERCISED here — that the old parameters
// violated it is read off the migration, and that the new ones satisfy it
// depends on the service resolving a calendar that exists, which is asserted
// separately against the resolution ladder. A real-DB integration test is booked
// in .ai/todo.md; the operator's instance is the live confirmation.
package calendar

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- a recording stand-in for the database ---------------------------------

// execCall is one statement as database/sql handed it to the driver.
type execCall struct {
	query string
	args  []driver.NamedValue
}

// recConn records every ExecContext and answers success. It implements
// driver.ExecerContext, so database/sql calls it directly and no Prepare round
// trip flattens the arguments out of view.
type recConn struct {
	calls *[]execCall
	fail  error
}

func (c *recConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	*c.calls = append(*c.calls, execCall{query: query, args: args})
	if c.fail != nil {
		return nil, c.fail
	}
	return driver.RowsAffected(1), nil
}

func (c *recConn) Prepare(string) (driver.Stmt, error) { return nil, io.EOF }
func (c *recConn) Close() error                        { return nil }
func (c *recConn) Begin() (driver.Tx, error)           { return nil, io.EOF }

type recConnector struct {
	calls *[]execCall
	fail  error
}

func (c recConnector) Connect(context.Context) (driver.Conn, error) {
	return &recConn{calls: c.calls, fail: c.fail}, nil
}
func (c recConnector) Driver() driver.Driver { return recDriver{} }

type recDriver struct{}

func (recDriver) Open(string) (driver.Conn, error) { return nil, io.EOF }

// recordingRepo returns a real calendarRepo wired to the recorder, plus the
// slice the statements land in.
func recordingRepo(t *testing.T, fail error) (CalendarRepository, *[]execCall) {
	t.Helper()
	calls := &[]execCall{}
	db := sql.OpenDB(recConnector{calls: calls, fail: fail})
	t.Cleanup(func() { _ = db.Close() })
	return NewCalendarRepository(db), calls
}

// argStrings renders the recorded arguments for a message a human can read.
func argStrings(args []driver.NamedValue) []any {
	out := make([]any, 0, len(args))
	for _, a := range args {
		out = append(out, a.Value)
	}
	return out
}

// --- the emitted parameters -------------------------------------------------

// TestPrefsWriters_NeverEmitAnEmptyCalendarID is the regression pin.
//
// It asserts on the ARGUMENTS database/sql sends, because that is the layer the
// bug lived at: the SQL text was fine, the third parameter was not. A repository
// that once again hardcodes a placeholder id — empty string, "0", "-" — fails
// here regardless of how the statement is worded.
func TestPrefsWriters_NeverEmitAnEmptyCalendarID(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		write func(r CalendarRepository) error
	}{
		{"sidebar pin", func(r CalendarRepository) error {
			return r.SetSidebarPinned(ctx, "user-1", "camp-1", "cal-7", false)
		}},
		{"block layers", func(r CalendarRepository) error {
			return r.SetBlockLayers(ctx, "user-1", "camp-1", "cal-7", []string{"moons", "eras"})
		}},
		{"bench sections", func(r CalendarRepository) error {
			return r.SetBenchSections(ctx, "user-1", "camp-1", "cal-7", []string{"rsvp"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, calls := recordingRepo(t, nil)
			if err := tc.write(repo); err != nil {
				t.Fatalf("write: %v", err)
			}
			if len(*calls) != 1 {
				t.Fatalf("%d statements executed, want exactly 1", len(*calls))
			}
			got := (*calls)[0]

			if !strings.Contains(got.query, "INSERT INTO calendar_active") {
				t.Fatalf("statement is not the calendar_active upsert:\n%s", got.query)
			}
			// The literal the bug was made of must not survive anywhere in the
			// statement: not as a parameter, and not inlined into the VALUES list.
			if strings.Contains(collapseSpace(got.query), "VALUES (?, ?, '',") {
				t.Errorf("the statement still hardcodes an empty calendar_id:\n%s\n"+
					"calendar_active.calendar_id is NOT NULL and foreign-keyed to calendars(id) "+
					"(migration 006:17,22-23); the empty string matches no calendar and MariaDB "+
					"rejects the INSERT with errno 1452 before the duplicate-key path is reached.",
					got.query)
			}
			if len(got.args) != 4 {
				t.Fatalf("%d arguments, want 4 (user, campaign, calendar, value): %v",
					len(got.args), argStrings(got.args))
			}
			calID, ok := got.args[2].Value.(string)
			if !ok {
				t.Fatalf("calendar_id argument is %T, want string: %v", got.args[2].Value, argStrings(got.args))
			}
			if calID == "" {
				t.Errorf("calendar_id was written as the empty string — this is the 1452 that made "+
					"the switches inert; args = %v", argStrings(got.args))
			}
			if calID != "cal-7" {
				t.Errorf("calendar_id = %q, want the id the caller supplied (cal-7); args = %v",
					calID, argStrings(got.args))
			}
			if u, _ := got.args[0].Value.(string); u != "user-1" {
				t.Errorf("user_id = %q, want user-1", u)
			}
			if c, _ := got.args[1].Value.(string); c != "camp-1" {
				t.Errorf("campaign_id = %q, want camp-1", c)
			}
		})
	}
}

// TestPrefsWriters_ConflictUpdatesOnlyTheirOwnColumn.
//
// THE UPSERT MUST NOT TOUCH calendar_id ON CONFLICT, and this is not a
// nice-to-have: the row it upserts is the ACTIVE-CALENDAR POINTER. A viewer who
// switched to their secondary calendar and then collapsed a Bench section would,
// under a clause that also wrote calendar_id, be silently moved back to the
// campaign default — a preference write stealing a navigation choice. So each
// writer names exactly one column in ON DUPLICATE KEY UPDATE: its own.
func TestPrefsWriters_ConflictUpdatesOnlyTheirOwnColumn(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		column string
		write  func(r CalendarRepository) error
	}{
		{"sidebar pin", "sidebar_pinned", func(r CalendarRepository) error {
			return r.SetSidebarPinned(ctx, "u", "c", "cal-7", true)
		}},
		{"block layers", "block_layers", func(r CalendarRepository) error {
			return r.SetBlockLayers(ctx, "u", "c", "cal-7", []string{"moons"})
		}},
		{"bench sections", "bench_sections", func(r CalendarRepository) error {
			return r.SetBenchSections(ctx, "u", "c", "cal-7", []string{"rsvp"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, calls := recordingRepo(t, nil)
			if err := tc.write(repo); err != nil {
				t.Fatalf("write: %v", err)
			}
			q := collapseSpace((*calls)[0].query)
			at := strings.Index(q, "ON DUPLICATE KEY UPDATE")
			if at < 0 {
				t.Fatalf("no ON DUPLICATE KEY UPDATE clause — the write is not an upsert:\n%s", q)
			}
			clause := q[at:]
			want := "ON DUPLICATE KEY UPDATE " + tc.column + " = VALUES(" + tc.column + ")"
			if strings.TrimSpace(clause) != want {
				t.Errorf("conflict clause is %q; want exactly %q — naming any other column here "+
					"lets a preference write overwrite the viewer's active-calendar choice",
					strings.TrimSpace(clause), want)
			}
		})
	}
}

// A nil key slice still has to reach the driver as NULL rather than as the empty
// string: NULL is "never chosen" and '' is a real choice, and the calendar_id fix
// must not have collapsed that distinction while re-ordering the parameters.
func TestPrefsWriters_NilKeysStillWriteNull(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		write func(r CalendarRepository) error
	}{
		{"block layers", func(r CalendarRepository) error {
			return r.SetBlockLayers(ctx, "u", "c", "cal-7", nil)
		}},
		{"bench sections", func(r CalendarRepository) error {
			return r.SetBenchSections(ctx, "u", "c", "cal-7", nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, calls := recordingRepo(t, nil)
			if err := tc.write(repo); err != nil {
				t.Fatalf("write: %v", err)
			}
			if v := (*calls)[0].args[3].Value; v != nil {
				t.Errorf("nil keys reached the driver as %#v, want NULL — NULL means "+
					"'never chosen' and the empty string means a real, different choice", v)
			}
		})
	}
	// And the empty non-nil slice is the other side of the same rule.
	repo, calls := recordingRepo(t, nil)
	if err := repo.SetBlockLayers(ctx, "u", "c", "cal-7", []string{}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if v := (*calls)[0].args[3].Value; v != "" {
		t.Errorf("an empty non-nil slice reached the driver as %#v, want the empty string "+
			"(the bare month, a chosen state)", v)
	}
}

// collapseSpace folds the SQL's formatting whitespace so an assertion can be
// written against the statement's meaning rather than its indentation.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// --- the resolution the service does ----------------------------------------

// TestPrefsCalendarID_WalksTheSameLadderAsGetActiveCalendar.
//
// The id the writers need is resolved by the service, and it reuses
// resolveActiveCalendar rather than re-deriving the rule. That reuse is the
// guarantee worth pinning: a preference row can never point somewhere the reader
// would not have gone.
func TestPrefsCalendarID_WalksTheSameLadderAsGetActiveCalendar(t *testing.T) {
	ctx := context.Background()

	t.Run("the viewer's active pointer wins", func(t *testing.T) {
		var wrote string
		repo := &mockCalendarRepo{
			getActiveCalendarIDFn: func(context.Context, string, string) (string, error) {
				return "cal-chosen", nil
			},
			getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
				return &Calendar{ID: id, CampaignID: "camp-1"}, nil
			},
			getByCampaignIDFn: stockDefaultCalendar,
			setBlockLayersFn: func(_ context.Context, _, _, calendarID string, _ []string) error {
				wrote = calendarID
				return nil
			},
		}
		if err := NewCalendarService(repo).SetBlockLayers(ctx, "u", "camp-1", []string{"moons"}); err != nil {
			t.Fatalf("SetBlockLayers: %v", err)
		}
		if wrote != "cal-chosen" {
			t.Errorf("wrote calendar_id %q; a viewer with an active calendar must have their "+
				"preference row attached to it, not to the default", wrote)
		}
	})

	t.Run("no pointer falls back to the campaign default", func(t *testing.T) {
		var wrote string
		repo := &mockCalendarRepo{
			getByCampaignIDFn: stockDefaultCalendar,
			setBlockLayersFn: func(_ context.Context, _, _, calendarID string, _ []string) error {
				wrote = calendarID
				return nil
			},
		}
		if err := NewCalendarService(repo).SetBlockLayers(ctx, "u", "camp-1", []string{"moons"}); err != nil {
			t.Fatalf("SetBlockLayers: %v", err)
		}
		if wrote != prefsDefaultCalendarID {
			t.Errorf("wrote calendar_id %q, want the campaign default %q — this is the rung the "+
				"viewer who never used the multi-calendar switcher lands on, and the rung the old "+
				"empty-string seed made unreachable", wrote, prefsDefaultCalendarID)
		}
	})

	t.Run("a stale pointer falls through instead of writing it", func(t *testing.T) {
		var wrote string
		repo := &mockCalendarRepo{
			getActiveCalendarIDFn: func(context.Context, string, string) (string, error) {
				return "cal-elsewhere", nil
			},
			// Resolves, but into a DIFFERENT campaign: resolveActiveCalendar
			// rejects it, and so must the preference write — a row pointing at
			// another campaign's calendar is the FK violation's cousin.
			getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
				return &Calendar{ID: id, CampaignID: "camp-other"}, nil
			},
			getByCampaignIDFn: stockDefaultCalendar,
			setBlockLayersFn: func(_ context.Context, _, _, calendarID string, _ []string) error {
				wrote = calendarID
				return nil
			},
		}
		if err := NewCalendarService(repo).SetBlockLayers(ctx, "u", "camp-1", []string{"moons"}); err != nil {
			t.Fatalf("SetBlockLayers: %v", err)
		}
		if wrote != prefsDefaultCalendarID {
			t.Errorf("wrote calendar_id %q; a pointer into another campaign must not be written, "+
				"the default must be", wrote)
		}
	})

	t.Run("no default falls back to the first by sort order", func(t *testing.T) {
		var wrote string
		repo := &mockCalendarRepo{
			listByCampaignIDFn: func(context.Context, string) ([]Calendar, error) {
				return []Calendar{{ID: "cal-first", CampaignID: "camp-1"}}, nil
			},
			setBlockLayersFn: func(_ context.Context, _, _, calendarID string, _ []string) error {
				wrote = calendarID
				return nil
			},
		}
		if err := NewCalendarService(repo).SetBlockLayers(ctx, "u", "camp-1", []string{"moons"}); err != nil {
			t.Fatalf("SetBlockLayers: %v", err)
		}
		if wrote != "cal-first" {
			t.Errorf("wrote calendar_id %q, want cal-first — a campaign whose default was never "+
				"flagged still has to be able to save a preference", wrote)
		}
	})
}

// TestPrefsWrite_WithNoCalendarRefusesRatherThanInventingOne.
//
// STOP-AND-FLAG, RECORDED AS A TEST. The schema makes this state expressible —
// preferences are per (user, campaign) but the row they live on is FK'd to a
// calendar — so a campaign with zero calendars has no valid row to write. It is
// not reachable from any surface that offers these controls (all three live on
// the Bench or the V2 shell, both of which need a calendar to render), so the fix
// is NOT a migration: it is refusing honestly. A migration relaxing the FK would
// be append-only, idempotent and plugin-scoped work with a real design question
// behind it (should the preference outlive every calendar?), and that decision is
// not this hotfix's to make.
func TestPrefsWrite_WithNoCalendarRefusesRatherThanInventingOne(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		write func(s CalendarService, repo *mockCalendarRepo) error
	}{
		{"block layers", func(s CalendarService, _ *mockCalendarRepo) error {
			return s.SetBlockLayers(ctx, "u", "camp-empty", []string{"moons"})
		}},
		{"sidebar pin", func(s CalendarService, _ *mockCalendarRepo) error {
			return s.SetSidebarPinned(ctx, "u", "camp-empty", true)
		}},
		{"bench section", func(s CalendarService, _ *mockCalendarRepo) error {
			return s.ToggleBenchSection(ctx, "u", "camp-empty", "rsvp")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written := false
			repo := &mockCalendarRepo{
				setBlockLayersFn: func(context.Context, string, string, string, []string) error {
					written = true
					return nil
				},
				setSidebarPinnedFn: func(context.Context, string, string, string, bool) error {
					written = true
					return nil
				},
			}
			err := tc.write(NewCalendarService(repo), repo)
			if err == nil {
				t.Fatal("a campaign with no calendars must refuse the write — there is no valid " +
					"calendar_id for the row, and inventing one is the bug this hotfix fixes")
			}
			if written {
				t.Error("the repository was written anyway: the refusal must happen before the upsert")
			}
			var appErr *apperror.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("error is %T, want an *apperror.AppError so the client sees a real message", err)
			}
			if appErr.Code >= 500 {
				t.Errorf("status %d — a campaign with no calendar is a request the server understood "+
					"and cannot satisfy, not a server fault", appErr.Code)
			}
		})
	}
}

// --- the failure the operator could not see ---------------------------------

// TestPrefsWrite_FailureIsAnAppErrorWithAMessage.
//
// The switches read as inert because a failed write produced a bare 500: no
// HX-Refresh, no state change, and a generic toast that named nothing. A driver
// error must not travel to the client, but the FACT of the failure must — a
// control that reports its own failure can be retried; one that reports nothing
// looks broken in a way nobody files a bug about for months.
func TestPrefsWrite_FailureIsAnAppErrorWithAMessage(t *testing.T) {
	boom := errors.New("Error 1452 (23000): Cannot add or update a child row")
	repo := &mockCalendarRepo{
		getByCampaignIDFn: stockDefaultCalendar,
		setBlockLayersFn: func(context.Context, string, string, string, []string) error {
			return boom
		},
	}
	err := NewCalendarService(repo).SetBlockLayers(context.Background(), "u", "camp-1", []string{"moons"})
	if err == nil {
		t.Fatal("a failed write must not be reported as success")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error is %T, want *apperror.AppError — a raw driver error reaches app.go as an "+
			"anonymous 500 and the UI has nothing to say", err)
	}
	if appErr.Message == "" || appErr.Message == "An unexpected error occurred. Please try again." {
		t.Errorf("client message is %q; it must name what failed so the toast can say "+
			"'could not save' rather than nothing useful", appErr.Message)
	}
	if strings.Contains(appErr.Message, "1452") || strings.Contains(appErr.Message, "child row") {
		t.Errorf("the driver error leaked into the client message: %q", appErr.Message)
	}
	if !errors.Is(err, boom) {
		t.Error("the underlying error must stay reachable for the log")
	}
}

// A validation failure keeps its own status: prefsWriteError must not promote a
// 422 the service raised on purpose into an anonymous 500.
func TestPrefsWriteError_DoesNotReclassifyADomainError(t *testing.T) {
	in := apperror.NewValidation("unknown layer key")
	out := prefsWriteError(in)
	var appErr *apperror.AppError
	if !errors.As(out, &appErr) {
		t.Fatalf("out is %T, want *apperror.AppError", out)
	}
	if appErr.Code != in.Code {
		t.Errorf("status %d, want %d — wrapping a classified error would turn every rejected "+
			"layer key into a server fault", appErr.Code, in.Code)
	}
	if prefsWriteError(nil) != nil {
		t.Error("prefsWriteError(nil) must stay nil")
	}
}
