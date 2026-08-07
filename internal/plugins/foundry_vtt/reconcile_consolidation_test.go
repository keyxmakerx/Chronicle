package foundry_vtt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubReconciler drives every branch of reconcileConsolidationState with canned
// values. Same stub shape as premigration_test.go's checker stub.
type stubReconciler struct {
	exists     bool
	existsErr  error
	markErr    error
	markCalled int
}

func (s *stubReconciler) predecessorTableExists(context.Context) (bool, error) {
	return s.exists, s.existsErr
}

func (s *stubReconciler) markConsolidationApplied(context.Context) error {
	s.markCalled++
	return s.markErr
}

// TestReconcileConsolidationState_MarksInapplicableMigrationApplied is the
// fresh-install case (C-SWEEP-R4 / data/fvtt-fresh-db-rename): the predecessor
// token table has never existed, so migration 001's RENAME can never succeed
// and must be recorded as applied — otherwise the runner stops there forever
// and the Foundry integration is dead on every new self-hosted install.
func TestReconcileConsolidationState_MarksInapplicableMigrationApplied(t *testing.T) {
	s := &stubReconciler{exists: false}
	if err := reconcileConsolidationState(context.Background(), s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.markCalled != 1 {
		t.Errorf("migration 001 was not recorded as applied (markConsolidationApplied called %d times); "+
			"the runner will retry a RENAME with no source table and degrade the plugin forever",
			s.markCalled)
	}
}

// TestReconcileConsolidationState_LeavesRealUpgradeAlone is the half that keeps
// the fix from becoming a data-loss bug. When the predecessor table IS present
// the database is genuinely pre-consolidation: 001 has real token rows to
// carry across, so it must run. Marking it applied here would strand those rows
// under the old table name while the repository queries the new one.
func TestReconcileConsolidationState_LeavesRealUpgradeAlone(t *testing.T) {
	s := &stubReconciler{exists: true}
	if err := reconcileConsolidationState(context.Background(), s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.markCalled != 0 {
		t.Errorf("migration 001 was skipped on a database that still has %s; "+
			"its token rows would never be renamed into foundry_vtt_campaign_tokens",
			predecessorTokenTable)
	}
}

// TestReconcileConsolidationState_PropagatesLookupFailure: an unreadable
// information_schema means the applicability decision would be a guess. Guessing
// "inapplicable" would skip a migration that had work to do; guessing
// "applicable" reinstates the crash. So it must fail loudly, and the message
// must say what to check.
func TestReconcileConsolidationState_PropagatesLookupFailure(t *testing.T) {
	sentinel := errors.New("information_schema unreadable")
	s := &stubReconciler{existsErr: sentinel}

	err := reconcileConsolidationState(context.Background(), s)
	if err == nil {
		t.Fatal("expected an error when the schema lookup fails")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("underlying error not wrapped: %v", err)
	}
	if !strings.Contains(err.Error(), "information_schema.tables") {
		t.Errorf("error message does not name the failing query, so it is not actionable: %v", err)
	}
	if s.markCalled != 0 {
		t.Error("recorded migration 001 as applied despite not knowing whether it was applicable")
	}
}

// TestReconcileConsolidationState_PropagatesMarkFailure: if the tracking row
// cannot be written, the next boot repeats the doomed migration. Surfacing it
// is the difference between an operator seeing why Foundry is disabled and
// hunting a silent degradation.
func TestReconcileConsolidationState_PropagatesMarkFailure(t *testing.T) {
	sentinel := errors.New("plugin_schema_versions is read-only")
	s := &stubReconciler{exists: false, markErr: sentinel}

	err := reconcileConsolidationState(context.Background(), s)
	if err == nil {
		t.Fatal("expected an error when recording the applied version fails")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("underlying error not wrapped: %v", err)
	}
	if !strings.Contains(err.Error(), "plugin_schema_versions") {
		t.Errorf("error message does not name the table to check: %v", err)
	}
}

// TestReconcileConsolidationState_UsesTheRunnersSlug pins the trap that would
// make this whole fix a no-op while looking correct. This plugin has TWO
// exported identifiers: PluginSlug is "foundry-vtt" (hyphen, the external
// registry name) and PluginHealthKey is "foundry_vtt" (underscore), and
// cmd/server/main.go registers the migration schema under the UNDERSCORE one.
// A row written under the hyphen form is invisible to the migration runner:
// the reconciler would report success and migration 001 would still be
// attempted, still fail, and still kill the plugin.
func TestReconcileConsolidationState_UsesTheRunnersSlug(t *testing.T) {
	if migrationSlug != PluginHealthKey {
		t.Fatalf("migrationSlug = %q, want PluginHealthKey %q", migrationSlug, PluginHealthKey)
	}
	if migrationSlug == PluginSlug {
		t.Fatalf("migrationSlug is the EXTERNAL slug %q; plugin_schema_versions rows are keyed "+
			"by the underscore form the runner registers", PluginSlug)
	}
}
