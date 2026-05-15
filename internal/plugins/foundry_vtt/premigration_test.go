// Tests for PreMigrationCheck — the operator-facing fail-loud guard
// against C-FMC-5c silently dropping foundry_module_versions rows.
//
// Three paths pinned:
//
//  1. Table doesn't exist (fresh deploy / migration already ran) → nil.
//  2. Table exists + empty → nil. The migration is safe to run.
//  3. Table exists + has rows → ERROR with the operator-actionable
//     four-clause message including the row count + the manual
//     DROP TABLE escape hatch.
//
// Uses a small in-package fake (fakeChecker) rather than sqlmock so
// the test doesn't pull a new dep into go.mod just for this one
// check. The fakeChecker satisfies the unexported preMigrationChecker
// interface that preMigrationCheck consumes.
package foundry_vtt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeChecker stubs preMigrationChecker with canned values per test.
// existsErr / countErr let tests exercise the DB-failure branches.
type fakeChecker struct {
	exists    bool
	existsErr error
	count     int
	countErr  error
}

func (f fakeChecker) tableExists(_ context.Context) (bool, error) {
	return f.exists, f.existsErr
}
func (f fakeChecker) rowCount(_ context.Context) (int, error) {
	return f.count, f.countErr
}

// TestPreMigrationCheck_TableMissing — typical post-migration state:
// the rename + drop already ran, so the table is gone. Pre-check
// is a no-op (idempotent re-run on every boot).
func TestPreMigrationCheck_TableMissing(t *testing.T) {
	err := preMigrationCheck(context.Background(), fakeChecker{exists: false})
	if err != nil {
		t.Fatalf("expected nil error when table doesn't exist, got: %v", err)
	}
}

// TestPreMigrationCheck_TableExistsEmpty — pre-migration state on a
// freshly upgraded deployment: foundry_module_versions still exists
// (from foundry_modules' migration 001) but operator never uploaded.
// Migration is safe; pre-check returns nil.
func TestPreMigrationCheck_TableExistsEmpty(t *testing.T) {
	err := preMigrationCheck(context.Background(), fakeChecker{exists: true, count: 0})
	if err != nil {
		t.Fatalf("expected nil error for empty table, got: %v", err)
	}
}

// TestPreMigrationCheck_TableHasRows — the operator-actionable
// failure path. Pre-check refuses to let startup proceed; the error
// message must include the row count and the manual DROP TABLE
// escape hatch so the operator knows exactly what to do.
//
// Critical contract: this test failing means the C-FMC-5c migration
// could silently destroy operator data.
func TestPreMigrationCheck_TableHasRows(t *testing.T) {
	err := preMigrationCheck(context.Background(), fakeChecker{exists: true, count: 3})
	if err == nil {
		t.Fatal("expected error when foundry_module_versions has rows, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "3 row(s)") {
		t.Errorf("error should mention row count, got: %s", msg)
	}
	if !strings.Contains(msg, "DROP TABLE") {
		t.Errorf("error should include the manual DROP TABLE escape hatch, got: %s", msg)
	}
	if !strings.Contains(msg, "C-FMC-5c") {
		t.Errorf("error should identify the migration, got: %s", msg)
	}
}

// TestPreMigrationCheck_NilDB — defensive: passing nil to the public
// PreMigrationCheck wrapper returns a clear error rather than
// panicking. The unexported preMigrationCheck never sees nil
// (sqlDBChecker wraps the *sql.DB).
func TestPreMigrationCheck_NilDB(t *testing.T) {
	if err := PreMigrationCheck(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil DB, got nil")
	}
}

// TestPreMigrationCheck_TableExistsQueryFails — information_schema
// query fails (permissions, network blip). Pre-check returns the
// wrapping error with the operator-actionable next step.
func TestPreMigrationCheck_TableExistsQueryFails(t *testing.T) {
	err := preMigrationCheck(context.Background(), fakeChecker{
		existsErr: errors.New("connection refused"),
	})
	if err == nil {
		t.Fatal("expected error when tableExists query fails, got nil")
	}
	if !strings.Contains(err.Error(), "information_schema") {
		t.Errorf("error should mention information_schema, got: %s", err)
	}
}

// TestPreMigrationCheck_RowCountQueryFails — the row-count query
// fails after the table is confirmed to exist. Pre-check surfaces
// the underlying error wrapped in the actionable message.
func TestPreMigrationCheck_RowCountQueryFails(t *testing.T) {
	err := preMigrationCheck(context.Background(), fakeChecker{
		exists:   true,
		countErr: errors.New("table corrupt"),
	})
	if err == nil {
		t.Fatal("expected error when rowCount query fails, got nil")
	}
	if !strings.Contains(err.Error(), "row count") {
		t.Errorf("error should mention row count failure, got: %s", err)
	}
}
