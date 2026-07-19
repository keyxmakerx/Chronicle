// resilience_test.go covers the "export everything → error" fix: a single
// item's unconvertible HTML, or a single category's lister failure, must
// degrade to a skipped field/section instead of aborting the whole export.
// Also pins the paging fix that stopped the entity export truncating to 24.
package aiexport

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// deepHTML nests elements past golang.org/x/net/html's 512-open-node limit,
// which makes the html-to-markdown converter return
// "open stack of elements exceeds 512 nodes". This is a real content shape a
// campaign can accumulate (pasted/imported markup, malformed nesting); it is
// the class of input that used to abort the whole export. Private/owner-only
// bodies like this render only in Permitted/Everything mode, which is why the
// bug looked privacy-specific.
var deepHTML = strings.Repeat("<div>", 20000) + "boom" + strings.Repeat("</div>", 20000)

// errEntityLister fails the List call to simulate a category-level lister/DB
// error (the second, defensive failure mode Generate must now survive).
type errEntityLister struct{ types []entities.EntityType }

func (e errEntityLister) List(context.Context, string, int, int, string, entities.ListOptions) ([]entities.Entity, int, error) {
	return nil, 0, fmt.Errorf("simulated DB failure")
}
func (e errEntityLister) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return e.types, nil
}

// pagingEntityLister honors Page/PerPage so listAllEntities can be exercised
// past the old 24-row clamp.
type pagingEntityLister struct {
	total int
	types []entities.EntityType
}

func (p *pagingEntityLister) List(_ context.Context, _ string, _ int, _ int, _ string, opts entities.ListOptions) ([]entities.Entity, int, error) {
	start := (opts.Page - 1) * opts.PerPage
	if start >= p.total {
		return nil, p.total, nil
	}
	end := start + opts.PerPage
	if end > p.total {
		end = p.total
	}
	out := make([]entities.Entity, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, entities.Entity{
			ID: fmt.Sprintf("e%d", i), Name: fmt.Sprintf("Entity %d", i),
			EntityTypeID: 1, TypeName: "Character", EntryHTML: sp("<p>body</p>"),
		})
	}
	return out, p.total, nil
}
func (p *pagingEntityLister) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return p.types, nil
}

// TestGenerate_BadEntityHTML_SkipsFieldNotExport is the core regression: one
// entity with unconvertible HTML must not fail the export. The document still
// renders every entity; the bad body is replaced with the skip marker.
func TestGenerate_BadEntityHTML_SkipsFieldNotExport(t *testing.T) {
	svc := NewService(
		&stubEntityLister{
			ents: []entities.Entity{
				{ID: "e1", Name: "Cursed Page", EntityTypeID: 1, TypeName: "Character", EntryHTML: sp(deepHTML)},
				{ID: "e2", Name: "Clean Page", EntityTypeID: 1, TypeName: "Character", EntryHTML: sp("<p>all good here</p>")},
			},
			types: []entities.EntityType{{ID: 1, Name: "Character", NamePlural: "Characters"}},
		},
		&stubNoteLister{}, &stubCalendarLister{}, &stubSessionLister{}, &stubTimelineLister{},
		&stubRelationLister{}, &stubTagLister{},
	)

	// Everything mode is the reported trigger (owner-view content included).
	got, err := svc.Generate(context.Background(), "Ashfall", "owner-1", "camp-1",
		Options{Privacy: PrivacyModeEverything})
	if err != nil {
		t.Fatalf("Generate must not fail on one bad field: %v", err)
	}
	for _, want := range []string{
		"### Cursed Page", // the bad entity still appears...
		convertSkipMarker, // ...with its body replaced by the marker
		"### Clean Page",  // and the good entity is untouched
		"all good here",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q. Body:\n%s", want, got)
		}
	}
	if strings.Contains(got, "boom") {
		t.Errorf("unconvertible content leaked into output:\n%s", got)
	}
}

// TestGenerate_CategoryListerError_SkipsSectionNotExport pins the second
// resilience layer: a category whose lister errors is noted-and-skipped, and
// the remaining categories still render.
func TestGenerate_CategoryListerError_SkipsSectionNotExport(t *testing.T) {
	svc := NewService(
		errEntityLister{types: []entities.EntityType{{ID: 1, Name: "Character"}}},
		&stubNoteLister{list: []notes.Note{{ID: "n1", Title: "Survives", EntryHTML: sp("<p>note body</p>")}}},
		&stubCalendarLister{}, &stubSessionLister{}, &stubTimelineLister{},
		&stubRelationLister{}, &stubTagLister{},
	)

	got, err := svc.Generate(context.Background(), "Ashfall", "owner-1", "camp-1", Options{})
	if err != nil {
		t.Fatalf("Generate must survive a category lister error: %v", err)
	}
	if !strings.Contains(got, "could not be exported") {
		t.Errorf("expected a skip note for the failed entities category:\n%s", got)
	}
	if !strings.Contains(got, "# Notes") || !strings.Contains(got, "note body") {
		t.Errorf("later categories must still render after an earlier failure:\n%s", got)
	}
}

// TestListAllEntities_PagesPastClamp proves the export no longer truncates at
// 24 entities: a 130-entity campaign yields all 130.
func TestListAllEntities_PagesPastClamp(t *testing.T) {
	svc := NewService(
		&pagingEntityLister{total: 130, types: []entities.EntityType{{ID: 1, Name: "Character"}}},
		nil, nil, nil, nil, nil, nil,
	)
	all, err := svc.listAllEntities(context.Background(), "camp-1", 3, "owner-1")
	if err != nil {
		t.Fatalf("listAllEntities: %v", err)
	}
	if len(all) != 130 {
		t.Fatalf("want 130 entities (no 24-row truncation), got %d", len(all))
	}
	// Spot-check boundary rows that the old clamp would have dropped.
	ids := map[string]bool{}
	for _, e := range all {
		ids[e.ID] = true
	}
	for _, want := range []string{"e0", "e24", "e100", "e129"} {
		if !ids[want] {
			t.Errorf("missing entity %q — paging dropped it", want)
		}
	}
}

// TestBodyOrSkip covers the helper directly: nil error passes through, a real
// error is swallowed into the marker.
func TestBodyOrSkip(t *testing.T) {
	if got := bodyOrSkip("k", "i", "hello", nil); got != "hello" {
		t.Errorf("nil error should pass body through, got %q", got)
	}
	if got := bodyOrSkip("k", "i", "", fmt.Errorf("x")); got != convertSkipMarker {
		t.Errorf("error should yield the skip marker, got %q", got)
	}
}
