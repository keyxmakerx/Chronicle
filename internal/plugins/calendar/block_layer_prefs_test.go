// block_layer_prefs_test.go — the per-viewer Block layer store
// (C-CALV4-LAYERS-P9 [LYR-3], migration 014).
//
// THE ONE PROPERTY EVERY ASSERTION HERE ORBITS: NULL IS NOT THE EMPTY SET.
// A viewer who has never opened the switchboard and a viewer who turned every
// layer off are two different people, and the store has to be able to say so —
// otherwise the bare month is unreachable and the "default" the whole slice
// exists to make leavable becomes a floor instead. Every branch below is
// written so collapsing the two would fail it.
package calendar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// --- Service: GetBlockLayers -----------------------------------------------

func TestGetBlockLayers_AnonymousGetsNoStoredSet(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{
		getBlockLayersFn: func(_ context.Context, _, _ string) ([]string, error) {
			t.Fatal("an anonymous viewer must not reach the repository at all")
			return nil, nil
		},
	})
	got, err := svc.GetBlockLayers(context.Background(), "", "camp-1")
	if err != nil {
		t.Fatalf("anonymous get should not error; got %v", err)
	}
	if got != nil {
		t.Errorf("anonymous stored set = %v, want nil — there is nowhere to persist a "+
			"per-anonymous preference, so the host's seed renders", got)
	}
}

func TestGetBlockLayers_NeverChosenIsNilNotEmpty(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{})
	got, err := svc.GetBlockLayers(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetBlockLayers: %v", err)
	}
	if got != nil {
		t.Errorf("stored set = %#v, want nil — NULL means the viewer has never chosen, "+
			"and the producer must render the HOST'S SEED for it", got)
	}
}

func TestGetBlockLayers_ChosenNothingIsEmptyNotNil(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{
		getBlockLayersFn: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{}, nil // the '' column value: a bare month
		},
	})
	got, err := svc.GetBlockLayers(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetBlockLayers: %v", err)
	}
	if got == nil {
		t.Fatal("a viewer who turned every layer off must NOT read back as nil — that would " +
			"silently restore the host's seed and the bare month would be unreachable")
	}
	if len(got) != 0 {
		t.Errorf("stored set = %v, want an empty non-nil slice", got)
	}
}

// TestGetBlockLayers_UnknownStoredKeyIsFilteredNotFatal pins dispatch §12.1's
// read rule. The route rejects an unknown key on the way IN, so the only way
// one reaches a read is a registry that SHRANK between the write and the
// render — a deploy, not a caller. Answering 500 would brick a viewer's
// calendar over the product's own history.
func TestGetBlockLayers_UnknownStoredKeyIsFilteredNotFatal(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{
		getBlockLayersFn: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{"moons", "skybox", "ledger"}, nil
		},
	})
	got, err := svc.GetBlockLayers(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("a retired key must not fail the read: %v", err)
	}
	if want := []string{"moons", "ledger"}; !reflect.DeepEqual(got, want) {
		t.Errorf("filtered set = %v, want %v", got, want)
	}
}

// A viewer whose ENTIRE stored set retired still counts as having chosen. The
// honest answer is a bare month, not a silent fall back to the host's seed —
// they never asked for the seed.
func TestGetBlockLayers_AllKeysRetiredStaysNonNil(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{
		getBlockLayersFn: func(_ context.Context, _, _ string) ([]string, error) {
			return []string{"skybox", "timepiece"}, nil
		},
	})
	got, err := svc.GetBlockLayers(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetBlockLayers: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("stored set = %#v, want an empty NON-NIL slice", got)
	}
}

func TestGetBlockLayers_PropagatesRepoError(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{
		getBlockLayersFn: func(_ context.Context, _, _ string) ([]string, error) {
			return nil, errors.New("db down")
		},
	})
	if _, err := svc.GetBlockLayers(context.Background(), "user-1", "camp-1"); err == nil {
		t.Error("repo error should propagate")
	}
}

// --- Service: SetBlockLayers -----------------------------------------------

func TestSetBlockLayers_EmptyUserRejects(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{})
	if err := svc.SetBlockLayers(context.Background(), "", "camp-1", []string{"moons"}); err == nil {
		t.Fatal("empty user_id should reject — an anonymous write would report success " +
			"and persist nothing")
	}
}

// THE REJECT-DO-NOT-DROP RULE (dispatch §12.1). A silently dropped key
// half-applies a choice and the viewer cannot tell which half landed.
func TestSetBlockLayers_UnknownKeyRejectsWholeWrite(t *testing.T) {
	called := false
	svc := NewCalendarService(&mockCalendarRepo{
		setBlockLayersFn: func(_ context.Context, _, _, _ string, _ []string) error {
			called = true
			return nil
		},
	})
	err := svc.SetBlockLayers(context.Background(), "user-1", "camp-1",
		[]string{"moons", "skybox"})
	if err == nil {
		t.Fatal("an unknown layer key must reject the whole write, not be dropped")
	}
	if called {
		t.Error("the repository was written despite the validation failure — the reject " +
			"must happen BEFORE the upsert, or a typo half-applies")
	}
}

func TestSetBlockLayers_TooManyKeysRejects(t *testing.T) {
	over := append(append([]string{}, calblock.LayerKeys...), calblock.LayerKeys...)
	svc := NewCalendarService(&mockCalendarRepo{})
	if err := svc.SetBlockLayers(context.Background(), "user-1", "camp-1", over); err == nil {
		t.Fatal("more entries than the registry has keys should reject")
	}
}

func TestSetBlockLayers_DeduplicatesAndPersists(t *testing.T) {
	var gotUser, gotCampaign, gotCalendar string
	var gotKeys []string
	svc := NewCalendarService(&mockCalendarRepo{
		getByCampaignIDFn: stockDefaultCalendar,
		setBlockLayersFn: func(_ context.Context, userID, campaignID, calendarID string, keys []string) error {
			gotUser, gotCampaign, gotCalendar, gotKeys = userID, campaignID, calendarID, keys
			return nil
		},
	})
	err := svc.SetBlockLayers(context.Background(), "user-1", "camp-1",
		[]string{"moons", "eras", "moons"})
	if err != nil {
		t.Fatalf("SetBlockLayers: %v", err)
	}
	if gotUser != "user-1" || gotCampaign != "camp-1" {
		t.Errorf("repo called with (%q, %q); want (user-1, camp-1)", gotUser, gotCampaign)
	}
	// The row this lands on is FK'd to calendars(id): an empty id here is the
	// errno 1452 that made the switchboard inert.
	if gotCalendar != prefsDefaultCalendarID {
		t.Errorf("repo called with calendar_id %q, want the resolved default %q",
			gotCalendar, prefsDefaultCalendarID)
	}
	if want := []string{"moons", "eras"}; !reflect.DeepEqual(gotKeys, want) {
		t.Errorf("persisted keys = %v, want %v (deduplicated, order preserved)", gotKeys, want)
	}
}

// The bare month is a legal WRITE, and it must not arrive at the repository as
// nil — nil is the reset that clears the row back to NULL.
func TestSetBlockLayers_EmptySetPersistsAsEmptyNotNil(t *testing.T) {
	var gotKeys []string
	var called bool
	svc := NewCalendarService(&mockCalendarRepo{
		getByCampaignIDFn: stockDefaultCalendar,
		setBlockLayersFn: func(_ context.Context, _, _, calendarID string, keys []string) error {
			called, gotKeys = true, keys
			if calendarID == "" {
				t.Error("the bare-month write went to an empty calendar_id — the column is " +
					"NOT NULL and foreign-keyed to calendars(id), so MariaDB refuses it")
			}
			return nil
		},
	})
	if err := svc.SetBlockLayers(context.Background(), "user-1", "camp-1", nil); err != nil {
		t.Fatalf("SetBlockLayers: %v", err)
	}
	if !called {
		t.Fatal("the write never reached the repository")
	}
	if gotKeys == nil {
		t.Error("an empty choice must reach the repository as an empty NON-NIL slice — " +
			"nil is the column's NULL, which means 'never chosen'")
	}
	if len(gotKeys) != 0 {
		t.Errorf("persisted keys = %v, want empty", gotKeys)
	}
}

// --- The migration itself --------------------------------------------------

// TestMigration014_IsAppendOnlyAndIdempotent reads the SQL as text. The
// migration-immutability guard proves nothing was EDITED; this proves what was
// ADDED obeys the rules that guard exists to protect (CLAUDE.md, ADR-044/045).
func TestMigration014_IsAppendOnlyAndIdempotent(t *testing.T) {
	up := readMigration(t, "014_block_layer_prefs.up.sql")
	down := readMigration(t, "014_block_layer_prefs.down.sql")

	if !containsFold(up, "ADD COLUMN IF NOT EXISTS") {
		t.Error("014 up must use ADD COLUMN IF NOT EXISTS — migration 007's bare ADD COLUMN " +
			"predates the rule and is immutable, so it is not the shape to copy")
	}
	if !containsFold(up, "ALTER TABLE calendar_active") {
		t.Error("014 must extend calendar_active (PR #368 stop-and-flag #3), not create a table")
	}
	if containsFold(up, "CREATE TABLE") {
		t.Error("014 must not create a table — [LYR-3] signs the (user_id, campaign_id) grain " +
			"on the existing row")
	}
	if !containsFold(up, "block_layers VARCHAR(255) DEFAULT NULL") {
		t.Error("the column must be VARCHAR(255) DEFAULT NULL — NULL is 'never chosen' and " +
			"is distinct from the empty set")
	}
	for _, bad := range []string{"INSERT ", "UPDATE ", "DELETE "} {
		if containsFold(up, bad) {
			t.Errorf("014 contains %q — migrations are SCHEMA-ONLY; a one-time data fix "+
				"belongs in an idempotent reconciler", bad)
		}
	}
	if !containsFold(down, "DROP COLUMN IF EXISTS block_layers") {
		t.Error("014 down must drop the column idempotently")
	}
}

// readMigration reads one of the plugin's own migration files off disk, so the
// assertions above judge the SQL that actually ships rather than a copy.
func readMigration(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("migrations", name))
	if err != nil {
		t.Fatalf("read migrations/%s: %v", name, err)
	}
	return string(body)
}

// containsFold ignores case and collapses whitespace, so a reflowed comment or
// a lower-cased keyword cannot fail an assertion about SQL semantics.
func containsFold(haystack, needle string) bool {
	flat := strings.Join(strings.Fields(haystack), " ")
	return strings.Contains(strings.ToUpper(flat), strings.ToUpper(strings.Join(strings.Fields(needle), " ")))
}
