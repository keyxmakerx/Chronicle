package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

const gmHTML = `<p>Public.</p><p><span data-secret="true">one</span></p><p><span data-secret="true">two</span></p>` +
	`<figure class="ce-img ce-img--gm"><img src="/m/a.png"></figure>`

func strp(s string) *string { return &s }

func TestGMOnlyParts_Phrase(t *testing.T) {
	tests := []struct {
		name string
		in   GMOnlyParts
		want string
	}{
		{"none", GMOnlyParts{}, ""},
		{"one passage", GMOnlyParts{Passages: 1}, "1 GM-only passage"},
		{"passages and picture", GMOnlyParts{Passages: 2, Pictures: 1}, "2 GM-only passages and 1 GM-only picture"},
		{"all three", GMOnlyParts{Passages: 1, Pictures: 2, Diagrams: 3}, "1 GM-only passage, 2 GM-only pictures and 3 GM-only diagrams"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Phrase(); got != tt.want {
				t.Errorf("Phrase() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassify_GMOnlyCounted(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{
			"secretive": {ID: "e1", Name: "Secretive", Slug: "secretive", EntryHTML: strp(gmHTML)},
			"plain":     {ID: "e2", Name: "Plain", Slug: "plain", EntryHTML: strp("<p>All public.</p>")},
		},
		types: []entities.EntityType{{ID: 1, Slug: "character"}},
	}
	pages := []ParsedPage{
		{Name: "Secretive", FrontMatter: FrontMatter{Type: "character", Action: ActionUpdate}, Status: StatusNew},
		{Name: "Plain", FrontMatter: FrontMatter{Type: "character", Action: ActionUpdate}, Status: StatusNew},
		{Name: "Secretive", FrontMatter: FrontMatter{Type: "character"}, Status: StatusNew}, // create-conflict
	}
	got, err := NewClassifier(lookup, "camp-1").ClassifyAll(context.Background(), pages)
	if err != nil {
		t.Fatal(err)
	}
	if want := (GMOnlyParts{Passages: 2, Pictures: 1}); got[0].GMOnly != want {
		t.Errorf("update row GMOnly = %+v, want %+v", got[0].GMOnly, want)
	}
	if got[1].GMOnly.Any() {
		t.Errorf("page without GM-only parts flagged: %+v", got[1].GMOnly)
	}
	if !got[2].GMOnly.Any() {
		t.Errorf("create row that conflicts with a GM-only page not flagged")
	}
}

// TestCommit_UpdateOverGMOnlyPage pins the server-side tick: no tick means
// no write of any kind, a tick writes, and pages without GM-only parts are
// untouched by the rule.
func TestCommit_UpdateOverGMOnlyPage(t *testing.T) {
	tests := []struct {
		name       string
		html       *string
		action     string
		conflict   string
		confirmed  bool
		wantStatus RowStatus
		wantWrite  bool
	}{
		{"update, no tick", strp(gmHTML), ActionUpdate, "", false, StatusSkipped, false},
		{"update, ticked", strp(gmHTML), ActionUpdate, "", true, StatusUpdated, true},
		{"create over it, update mode, no tick", strp(gmHTML), ActionCreate, "update", false, StatusSkipped, false},
		{"create over it, overwrite alias, ticked", strp(gmHTML), ActionCreate, "overwrite", true, StatusUpdated, true},
		{"update, plain page, no tick", strp("<p>Public only.</p>"), ActionUpdate, "", false, StatusUpdated, true},
		{"update, page with no text, no tick", nil, ActionUpdate, "", false, StatusUpdated, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{
				types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
				existing: map[string]*entities.Entity{
					"maro": {ID: "ent-1", Name: "Maro", Slug: "maro", EntryHTML: tt.html},
				},
			}
			pg := pageWithAction("Maro", "character", "# Maro\nNew text.", tt.action)
			d := decisionWithAction(true, "Maro", "character", "", tt.conflict, tt.action, false)
			d.ReplaceGMOnlyConfirmed = tt.confirmed
			res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
				OwnerID: "u-1", Pages: []ParsedPage{pg}, Decisions: []RowDecision{d},
			})
			if err != nil {
				t.Fatal(err)
			}
			row := res.Rows[0]
			if row.Status != tt.wantStatus {
				t.Fatalf("status = %q (%s), want %q", row.Status, row.Reason, tt.wantStatus)
			}
			wrote := len(f.updateEntries) > 0 || len(f.updateCalls) > 0
			if wrote != tt.wantWrite {
				t.Errorf("wrote = %v, want %v", wrote, tt.wantWrite)
			}
			if tt.wantStatus == StatusSkipped {
				if !strings.Contains(row.Reason, "2 GM-only passages and 1 GM-only picture") {
					t.Errorf("reason should name the parts; got %q", row.Reason)
				}
				if res.Skipped != 1 {
					t.Errorf("Skipped = %d, want 1", res.Skipped)
				}
			}
		})
	}
}
