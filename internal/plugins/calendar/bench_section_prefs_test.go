// bench_section_prefs_test.go — the per-viewer Bench disclosure store
// (C-CALV4-BENCH-R2 slice R2-1, [BR2-5] SIGNED).
//
// It is the sibling of block_layer_prefs_test.go and deliberately reads like
// it: same three-way NULL / '' / list discipline, same reject-don't-drop rule on
// the way in, same filter-and-log rule on the way out. One discipline, learned
// once.
package calendar

import (
	"context"
	"slices"
	"testing"
)

// benchSectionRepo is a stateful fake for the two new accessors ONLY. It
// embeds the package's shared mock so every other repository method keeps its
// established zero-value behaviour, and it holds real state because the flip is
// a read-modify-write and a func-hook mock cannot show that the read and the
// write agree.
type benchSectionRepo struct {
	*mockCalendarRepo
	benchSections        []string
	benchSectionsWritten bool
}

func (r *benchSectionRepo) GetBenchSections(_ context.Context, _, _ string) ([]string, error) {
	return r.benchSections, nil
}

func (r *benchSectionRepo) SetBenchSections(_ context.Context, _, _ string, keys []string) error {
	r.benchSections = keys
	r.benchSectionsWritten = true
	return nil
}

func newBenchSectionSvc(t *testing.T) (CalendarService, *benchSectionRepo) {
	t.Helper()
	repo := &benchSectionRepo{mockCalendarRepo: &mockCalendarRepo{}}
	return NewCalendarService(repo), repo
}

// --- the registry -----------------------------------------------------------

// The four keys are a CLOSED registry and the order is the page's contract
// order. A fifth key is a product decision, not a typo, so it is pinned.
func TestBenchSectionKeys_IsTheFourSignedSections(t *testing.T) {
	want := []string{"ribbon", "rsvp", "nextup", "rows"}
	if !slices.Equal(benchSectionKeys, want) {
		t.Errorf("benchSectionKeys = %v, want %v (BR2-3 SIGNED: four disclosures, in contract order)", benchSectionKeys, want)
	}
	// The surfaces BR2-3 rules may never collapse must never acquire a key.
	for _, forbidden := range []string{"stack", "phead", "sechead", "caption", "primary", "realworld"} {
		if slices.Contains(benchSectionKeys, forbidden) {
			t.Errorf("benchSectionKeys contains %q — BR2-3 SIGNED: the two Blocks, .phead, .sechead and .caption never collapse", forbidden)
		}
	}
}

// --- resolution: NULL is not '' ---------------------------------------------

// [BR2-4] SIGNED Option A: closed-by-default, at EVERY width. A viewer who has
// never chosen (NULL → nil) meets four closed sections.
func TestResolveBenchSections_NeverChosenIsAllClosed(t *testing.T) {
	got := resolveBenchSections(nil)
	for _, k := range benchSectionKeys {
		if !got[k] {
			t.Errorf("resolveBenchSections(nil)[%q] = false, want true — the ruled default is CLOSED at every width", k)
		}
	}
}

// '' is a real, reachable state and it is NOT the default: the viewer opened
// everything on purpose. This is the whole reason the store keeps the CLOSED
// set rather than the open one (migration 014's discipline, [BR2-5]).
func TestResolveBenchSections_ChoseNothingClosedIsAllOpen(t *testing.T) {
	got := resolveBenchSections([]string{})
	for _, k := range benchSectionKeys {
		if got[k] {
			t.Errorf("resolveBenchSections([])[%q] = true, want false — '' means 'I closed nothing', which must not be byte-identical to the default", k)
		}
	}
}

func TestResolveBenchSections_StoredListIsTheClosedSet(t *testing.T) {
	got := resolveBenchSections([]string{"rsvp", "rows"})
	for k, wantClosed := range map[string]bool{
		"ribbon": false, "rsvp": true, "nextup": false, "rows": true,
	} {
		if got[k] != wantClosed {
			t.Errorf("resolveBenchSections([rsvp rows])[%q] = %v, want %v", k, got[k], wantClosed)
		}
	}
}

// A key that has left the registry must never brick a viewer's page.
func TestResolveBenchSections_UnknownStoredKeyIsIgnored(t *testing.T) {
	got := resolveBenchSections([]string{"rsvp", "skyband"})
	if got["skyband"] {
		t.Error("a retired key resolved to a real closed section")
	}
	if !got["rsvp"] {
		t.Error("a retired key beside a live one must not take the live one down with it")
	}
}

// --- the service: the flip --------------------------------------------------

func TestGetBenchSections_AnonymousGetsNoStoredSet(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	repo.benchSections = []string{"rsvp"}
	got, err := svc.GetBenchSections(context.Background(), "", "camp-1")
	if err != nil {
		t.Fatalf("GetBenchSections: %v", err)
	}
	if got != nil {
		t.Errorf("anonymous viewer got %v; there is nowhere to persist a per-anonymous preference", got)
	}
}

func TestGetBenchSections_NeverChosenIsNilNotEmpty(t *testing.T) {
	svc, _ := newBenchSectionSvc(t)
	got, err := svc.GetBenchSections(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetBenchSections: %v", err)
	}
	if got != nil {
		t.Errorf("never-chosen resolved to %#v; NULL must stay nil so it is distinguishable from ''", got)
	}
}

func TestGetBenchSections_ChoseNothingIsEmptyNotNil(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	repo.benchSections = []string{}
	got, err := svc.GetBenchSections(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetBenchSections: %v", err)
	}
	if got == nil {
		t.Fatal("'' resolved to nil; 'I closed nothing' is a choice, not an absence")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty non-nil slice", got)
	}
}

// §12.1 / the 014 discipline: a stored key that has left the registry is
// filtered at READ with a log line, never a 500.
func TestGetBenchSections_UnknownStoredKeyIsFilteredNotFatal(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	repo.benchSections = []string{"rsvp", "skyband", "rows"}
	got, err := svc.GetBenchSections(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("GetBenchSections: %v", err)
	}
	if !slices.Equal(got, []string{"rsvp", "rows"}) {
		t.Errorf("got %v, want [rsvp rows] — the retired key is filtered, the rest survives", got)
	}
}

func TestToggleBenchSection_UnknownKeyRejectsTheWholeWrite(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	if err := svc.ToggleBenchSection(context.Background(), "user-1", "camp-1", "stack"); err == nil {
		t.Fatal("toggling an unknown section key succeeded; the registry is closed and rejection is the rule")
	}
	if repo.benchSectionsWritten {
		t.Error("a rejected toggle still wrote; validation must reject BEFORE the write, not after")
	}
}

func TestToggleBenchSection_EmptyUserRejects(t *testing.T) {
	svc, _ := newBenchSectionSvc(t)
	if err := svc.ToggleBenchSection(context.Background(), "", "camp-1", "rsvp"); err == nil {
		t.Error("an anonymous toggle reported success and persisted nothing")
	}
}

// The first flip from NULL is the one that has to be right: the default is
// "all four closed", so opening one section stores the OTHER THREE.
func TestToggleBenchSection_FirstFlipFromNullOpensExactlyOne(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	if err := svc.ToggleBenchSection(context.Background(), "user-1", "camp-1", "rsvp"); err != nil {
		t.Fatalf("ToggleBenchSection: %v", err)
	}
	if !slices.Equal(repo.benchSections, []string{"ribbon", "nextup", "rows"}) {
		t.Errorf("stored %v, want [ribbon nextup rows] — the store holds the CLOSED set, so opening rsvp closes the rest explicitly", repo.benchSections)
	}
}

func TestToggleBenchSection_ClosingTheLastOpenSectionStoresAllFour(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	repo.benchSections = []string{"ribbon", "nextup", "rows"}
	if err := svc.ToggleBenchSection(context.Background(), "user-1", "camp-1", "rsvp"); err != nil {
		t.Fatalf("ToggleBenchSection: %v", err)
	}
	if !slices.Equal(repo.benchSections, []string{"ribbon", "rsvp", "nextup", "rows"}) {
		t.Errorf("stored %v, want all four in contract order", repo.benchSections)
	}
}

// Opening every section must reach '' — a non-nil empty slice — and not fall
// back to nil, or the viewer could never express "I opened all four".
func TestToggleBenchSection_OpeningAllFourReachesEmptyNotNil(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	for _, k := range benchSectionKeys {
		if err := svc.ToggleBenchSection(context.Background(), "user-1", "camp-1", k); err != nil {
			t.Fatalf("ToggleBenchSection(%s): %v", k, err)
		}
	}
	if repo.benchSections == nil {
		t.Fatal("opening all four wrote NULL; that is 'never chosen', which renders all four CLOSED — the exact inverse of what the viewer asked for")
	}
	if len(repo.benchSections) != 0 {
		t.Errorf("stored %v, want an empty non-nil slice", repo.benchSections)
	}
}

// The stored order is the registry's order, not click order, so the column is
// greppable and two viewers with the same state store the same bytes.
func TestToggleBenchSection_StoredOrderIsContractOrder(t *testing.T) {
	svc, repo := newBenchSectionSvc(t)
	repo.benchSections = []string{"rows", "ribbon"}
	if err := svc.ToggleBenchSection(context.Background(), "user-1", "camp-1", "nextup"); err != nil {
		t.Fatalf("ToggleBenchSection: %v", err)
	}
	if !slices.Equal(repo.benchSections, []string{"ribbon", "nextup", "rows"}) {
		t.Errorf("stored %v, want [ribbon nextup rows] in contract order", repo.benchSections)
	}
}
