// committer_partial_update_test.go pins the AI import's half of the
// absent-means-preserve contract: it asserts what the committer SENDS to
// entities.EntityService on an update (privacy, type label, field data
// stay untouched unless front matter says otherwise). The other half —
// that the service preserves an absent field — is pinned independently
// against the real service by entities/permissions_inline_component_test.go
// and internal/patch/partial_update_contract_test.go.
package importer

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pageWithVisibility builds a parsed page whose front matter carries an
// explicit `visibility:` key — the ONLY signal the committer accepts as an
// instruction to change an existing page's privacy.
func pageWithVisibility(name, typeSlug, body, visibility string) ParsedPage {
	p := page(name, typeSlug, body)
	p.FrontMatter.Visibility = visibility
	return p
}

// existingDMOnlyPage is the entity under threat in finding 6: a page that is
// already hidden, which an import must not publish as a side effect.
func existingDMOnlyPage() map[string]*entities.Entity {
	return map[string]*entities.Entity{
		"lyra-vance": {ID: "ent-original", Name: "Lyra Vance", Slug: "lyra-vance", IsPrivate: true},
	}
}

func commitOneUpdate(t *testing.T, f *fakeCreator, p ParsedPage, d RowDecision) entities.UpdateEntityInput {
	t.Helper()
	c := NewCommitter(f)
	if _, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID:   "u-1",
		Pages:     []ParsedPage{p},
		Decisions: []RowDecision{d},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(f.updateCalls) != 1 {
		t.Fatalf("expected exactly 1 Update call; got %d", len(f.updateCalls))
	}
	return f.updateCalls[0]
}

func charTypes() []entities.EntityType {
	return []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}}
}

// FINDING 6. Front matter with no `visibility:` key must leave the existing
// page's privacy alone — even though the review row always carries a
// visibility value, because that value is pre-filled from the incoming
// markdown (or a bulk default), never from the entity being updated. An
// operator who leaves the dropdown alone has not consented to publish
// anything.
func TestCommitUpdate_AbsentFrontMatterVisibility_PreservesPrivacy(t *testing.T) {
	for _, conflictMode := range []string{"update"} {
		t.Run(conflictMode, func(t *testing.T) {
			f := &fakeCreator{types: charTypes(), existing: existingDMOnlyPage()}
			// decision() carries "public" exactly as the review form would
			// after a bulk default — the page's front matter says nothing.
			got := commitOneUpdate(t, f,
				page("Lyra Vance", "character", "# Lyra\n\nNew body."),
				decision(true, "Lyra Vance", "character", "public", conflictMode))

			if got.IsPrivate != nil {
				t.Errorf("IsPrivate must be nil (preserve) when front matter carries no visibility key; got %v, which would have published a dm_only page", *got.IsPrivate)
			}
		})
	}
}

// FINDING 6, the other direction: an explicit key IS an instruction, and
// must still work. A contract that only ever preserves is not a partial
// update, it is a broken one.
func TestCommitUpdate_ExplicitFrontMatterVisibility_IsHonored(t *testing.T) {
	tests := []struct {
		visibility    string
		wantIsPrivate bool
	}{
		{"public", false},
		{"private", true},
		{"dm_only", true}, // finding 8: dm_only lands as IsPrivate, nowhere else
	}
	for _, tc := range tests {
		t.Run(tc.visibility, func(t *testing.T) {
			f := &fakeCreator{types: charTypes(), existing: existingDMOnlyPage()}
			got := commitOneUpdate(t, f,
				pageWithVisibility("Lyra Vance", "character", "# Lyra\n\nBody.", tc.visibility),
				decision(true, "Lyra Vance", "character", tc.visibility, "update"))

			if got.IsPrivate == nil {
				t.Fatalf("explicit `visibility: %s` must reach IsPrivate; got nil (preserve)", tc.visibility)
			}
			if *got.IsPrivate != tc.wantIsPrivate {
				t.Errorf("visibility %q → IsPrivate %v; want %v", tc.visibility, *got.IsPrivate, tc.wantIsPrivate)
			}
		})
	}
}

// FINDING 7a. The importer has no source of structured field data — front
// matter carries no key for it — so a non-nil empty map was never "nothing
// to write", it was "delete every field on this page". This one was not
// attacker-directed: it fired on EVERY update commit.
func TestCommitUpdate_NeverWipesFieldsData(t *testing.T) {
	f := &fakeCreator{types: charTypes(), existing: existingDMOnlyPage()}
	got := commitOneUpdate(t, f,
		page("Lyra Vance", "character", "# Lyra\n\nBody."),
		decision(true, "Lyra Vance", "character", "private", "update"))

	if got.FieldsData != nil {
		t.Errorf("FieldsData must be nil (absent/preserve); got %#v — a non-nil map replaces the entity's whole field set", got.FieldsData)
	}
}

// FINDING 7b. patch.Of("") is an explicit CLEAR, not an absence. A page
// whose front matter omits `subcategory:` must not erase the descriptor off
// the entity it updates.
func TestCommitUpdate_AbsentSubcategory_PreservesTypeLabel(t *testing.T) {
	f := &fakeCreator{types: charTypes(), existing: existingDMOnlyPage()}
	got := commitOneUpdate(t, f,
		page("Lyra Vance", "character", "# Lyra\n\nBody."), // no subcategory
		decision(true, "Lyra Vance", "character", "private", "update"))

	if got.TypeLabel.Present() {
		t.Errorf("TypeLabel must be ABSENT when no subcategory was supplied; got present with %q, which clears the label", got.TypeLabel.Val(""))
	}
}

// And a supplied subcategory must still set it.
func TestCommitUpdate_ExplicitSubcategory_SetsTypeLabel(t *testing.T) {
	f := &fakeCreator{types: charTypes(), existing: existingDMOnlyPage()}
	p := page("Lyra Vance", "character", "# Lyra\n\nBody.")
	p.FrontMatter.Subcategory = "Rival"
	d := decision(true, "Lyra Vance", "character", "private", "update")
	d.Subcategory = "Rival"

	got := commitOneUpdate(t, f, p, d)

	if !got.TypeLabel.Present() || got.TypeLabel.Val("") != "Rival" {
		t.Errorf("explicit subcategory must reach TypeLabel; present=%v value=%q", got.TypeLabel.Present(), got.TypeLabel.Val(""))
	}
}
