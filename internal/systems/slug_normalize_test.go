// slug_normalize_test.go — C-SYSTEMS-REF-SLUG-FIX. Pins the load-time ID
// normalization in JSONProvider: ID wins when set, else Slug is promoted to
// ID, and an item with neither is skipped (with a diagnostic event) rather
// than loaded with a blank, ambiguous ID. See json_provider.go's load loop
// and system.go's ReferenceItem.Slug doc comment for the rationale.
package systems

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRawTestData writes a raw JSON array (bypassing the ReferenceItem
// struct) so fixtures can omit fields entirely — as real slug-only source
// data does — rather than marshaling an empty "id" key via the Go struct.
func writeRawTestData(t *testing.T, dir, category, rawJSON string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, category+".json"), []byte(rawJSON), 0o644); err != nil {
		t.Fatalf("write %s.json: %v", category, err)
	}
}

func TestJSONProvider_IDNormalization(t *testing.T) {
	dir := t.TempDir()
	writeRawTestData(t, dir, "creatures", `[
		{"name": "Slug Only", "slug": "slug-only-item", "summary": "s1"},
		{"name": "ID Only", "id": "id-only-item", "summary": "s2"},
		{"name": "Both Set", "id": "id-wins", "slug": "slug-loses", "summary": "s3"},
		{"name": "Neither", "summary": "s4"}
	]`)

	globalEventLog = NewEventLog(10)
	p, err := NewJSONProvider("test-mod", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	items, err := p.List("creatures")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (the neither-set item must be skipped): %+v", len(items), items)
	}

	byName := make(map[string]ReferenceItem, len(items))
	for _, it := range items {
		byName[it.Name] = it
	}

	if it, ok := byName["Slug Only"]; !ok || it.ID != "slug-only-item" {
		t.Errorf("slug-only item: ID = %q, want %q (fallback to slug)", it.ID, "slug-only-item")
	}
	if it, ok := byName["ID Only"]; !ok || it.ID != "id-only-item" {
		t.Errorf("id-only item: ID = %q, want %q", it.ID, "id-only-item")
	}
	if it, ok := byName["Both Set"]; !ok || it.ID != "id-wins" {
		t.Errorf("both-set item: ID = %q, want %q (id must win over slug)", it.ID, "id-wins")
	}
	if _, ok := byName["Neither"]; ok {
		t.Errorf("neither-set item must be skipped, not loaded with a blank ID")
	}

	// Get() must resolve every surviving item by its normalized ID.
	for _, id := range []string{"slug-only-item", "id-only-item", "id-wins"} {
		item, err := p.Get("creatures", id)
		if err != nil {
			t.Fatalf("get(%q): %v", id, err)
		}
		if item == nil {
			t.Errorf("get(%q): not found, want a match", id)
		}
	}

	// A blank-ID lookup must never match the skipped item.
	if item, _ := p.Get("creatures", ""); item != nil {
		t.Errorf("get(\"\") resolved an item (%+v); the neither-set item must not be loaded under a blank ID", item)
	}

	// The skip must be visible in admin diagnostics (skip + log, not skip +
	// silence), matching the existing EventSkipped pattern loader.go uses for
	// duplicate systems. Skips are aggregated to one event per file with a
	// count (C-SYSTEMS-REF-SLUG-FIX-R2), not one event per skipped item.
	events := DiagnosticEvents()
	found := false
	for _, e := range events {
		if e.Kind == EventSkipped && e.SystemID == "test-mod" &&
			strings.Contains(e.Error, `category "creatures"`) &&
			strings.Contains(e.Error, "1 missing id/slug") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an aggregated EventSkipped diagnostic for the neither-set item, got %+v", events)
	}
}

// TestJSONProvider_DrawSteelCreatureFixture loads a fixture shaped like the
// real Draw Steel data contract (slug-keyed, no "id" key at all — see
// Chronicle-Draw-Steel docs/DATA-SCHEMA.md's creatures.json example) and
// asserts Get() resolves it — the exact bug this fix closes (previously
// every DS creature loaded with ID == "", making Get() dead).
func TestJSONProvider_DrawSteelCreatureFixture(t *testing.T) {
	dir := t.TempDir()
	writeRawTestData(t, dir, "creatures", `[{
		"slug": "goblin-sniper",
		"name": "Goblin Sniper",
		"summary": "L1 Artillery Minion",
		"description": "A small, cunning goblin that pelts enemies with arrows.",
		"properties": {
			"level": 1,
			"organization": "Minion",
			"role": "Artillery",
			"ev": 1,
			"stamina": 7
		},
		"tags": ["creature", "minion", "artillery", "goblin", "level-1"],
		"source": "Draw Steel CC-BY-4.0, MCDM Productions"
	}]`)

	p, err := NewJSONProvider("chronicle-draw-steel", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	item, err := p.Get("creatures", "goblin-sniper")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item == nil {
		t.Fatal("Get(\"goblin-sniper\") returned nil — slug-keyed DS data is unaddressable")
	}
	if item.Name != "Goblin Sniper" {
		t.Errorf("name = %q, want %q", item.Name, "Goblin Sniper")
	}
	if lvl, _ := item.Properties["level"].(float64); lvl != 1 {
		t.Errorf("properties.level = %v, want 1", item.Properties["level"])
	}
}

// TestJSONProvider_DnD55eMonsterFixture loads a fixture shaped like the
// actual DnD-5.5e package data (id-keyed, no "slug" key — verified against
// Chronicle-DnD-5.5e's data/*.json, which is NOT slug-keyed as the dispatch
// assumed). Confirms this shape was never broken and stays unaffected: ID is
// already set from source, so the slug fallback never triggers.
func TestJSONProvider_DnD55eMonsterFixture(t *testing.T) {
	dir := t.TempDir()
	writeRawTestData(t, dir, "monsters", `[{
		"id": "goblin",
		"name": "Goblin",
		"summary": "A small, cunning humanoid.",
		"properties": {"cr": 0.25, "type": "humanoid"},
		"source": "SRD 5.1"
	}]`)

	p, err := NewJSONProvider("chronicle-dnd-5-5e", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	item, err := p.Get("monsters", "goblin")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item == nil {
		t.Fatal("Get(\"goblin\") returned nil — id-keyed DnD-5.5e data must keep working")
	}
	if item.ID != "goblin" || item.Slug != "" {
		t.Errorf("id-keyed item: ID = %q, Slug = %q — want ID from source id, Slug empty (no fallback needed)", item.ID, item.Slug)
	}
}

// TestJSONProvider_DuplicateNormalizedID pins C-SYSTEMS-REF-SLUG-FIX-R2's
// duplicate-ID guard: an explicit "id" and another item's "slug" can
// normalize to the identical ID within one category. Pre-R2 both items
// loaded into the slice, but Get() is first-match-wins, so the second
// item's own ID silently resolved to the FIRST item's content — a
// user-visibly wrong answer with zero diagnostic. The second (shadowing)
// occurrence must now be dropped at load time and flagged.
func TestJSONProvider_DuplicateNormalizedID(t *testing.T) {
	dir := t.TempDir()
	writeRawTestData(t, dir, "creatures", `[
		{"id": "goblin", "name": "Goblin (explicit id)", "summary": "the real goblin"},
		{"slug": "goblin", "name": "Goblin (slug fallback)", "summary": "a shadowing duplicate"}
	]`)

	globalEventLog = NewEventLog(10)
	p, err := NewJSONProvider("test-mod", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	items, err := p.List("creatures")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (the shadowing duplicate must be skipped): %+v", len(items), items)
	}
	if items[0].Name != "Goblin (explicit id)" {
		t.Errorf("surviving item = %q, want the first-loaded item to win", items[0].Name)
	}

	// Get() must resolve to the first item's content — never the dropped
	// duplicate's, and never a mix of the two.
	item, err := p.Get("creatures", "goblin")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if item == nil {
		t.Fatal("Get(\"goblin\") returned nil — the surviving item must still be addressable")
	}
	if item.Name != "Goblin (explicit id)" {
		t.Errorf("Get(\"goblin\").Name = %q, want %q", item.Name, "Goblin (explicit id)")
	}

	// The duplicate must be visible in admin diagnostics, not silently
	// dropped.
	events := DiagnosticEvents()
	found := false
	for _, e := range events {
		if e.Kind == EventSkipped && e.SystemID == "test-mod" &&
			strings.Contains(e.Error, `category "creatures"`) &&
			strings.Contains(e.Error, "1 duplicate id") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an EventSkipped diagnostic for the duplicate id, got %+v", events)
	}
}
