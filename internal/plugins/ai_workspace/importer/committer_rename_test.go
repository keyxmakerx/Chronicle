package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// TestCommit_ActionUpdate_RenameTo pins that rename_to reaches the page's
// name, and that a name another page holds is refused with nothing written.
func TestCommit_ActionUpdate_RenameTo(t *testing.T) {
	tests := []struct {
		name       string
		to, body   string
		taken      bool
		wantStatus RowStatus
		wantName   string
		wantReason string
		wantUndo   bool
	}{
		{"renames, no text", "Drakari", "", false, StatusUpdated, "Drakari", `Renamed from "The Drakari"`, true},
		{"renames and writes text", "Drakari", "New words.", false, StatusUpdated, "Drakari", `Renamed from "The Drakari"`, true},
		{"case-only rename", "the drakari", "", false, StatusUpdated, "the drakari", "Renamed from", true},
		{"name taken", "Elves", "", true, StatusFailed, "", `another page is already called "Elves"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{
				types: []entities.EntityType{{ID: 1, Name: "Race", Slug: "race", Enabled: true}},
				existing: map[string]*entities.Entity{
					"the-drakari": {ID: "ent-drakari", Name: "The Drakari", Slug: "the-drakari"},
				},
			}
			// Like the service: the page gets the new name's slug.
			f.updateFn = func(id string, in entities.UpdateEntityInput) (*entities.Entity, error) {
				e := &entities.Entity{ID: id, Name: in.Name.Val(""), Slug: entities.Slugify(in.Name.Val(""))}
				f.existing[e.Slug] = e
				return e, nil
			}
			if tt.taken {
				f.existing["elves"] = &entities.Entity{ID: "ent-elves", Name: "Elves", Slug: "elves"}
			}
			p := pageWithAction("The Drakari", "race", tt.body, ActionUpdate)
			p.FrontMatter.RenameTo = tt.to
			res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
				OwnerID:   "u-1",
				Pages:     []ParsedPage{p},
				Decisions: []RowDecision{decisionWithAction(true, "The Drakari", "race", "", "", ActionUpdate, false)},
			})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			row := res.Rows[0]
			if row.Status != tt.wantStatus || !strings.Contains(row.Reason, tt.wantReason) {
				t.Fatalf("row = %q %q, want %q with %q", row.Status, row.Reason, tt.wantStatus, tt.wantReason)
			}
			if tt.wantStatus == StatusFailed {
				if len(f.updateCalls) != 0 || len(f.updateEntries) != 0 {
					t.Errorf("a refused rename wrote: %d updates, %d texts", len(f.updateCalls), len(f.updateEntries))
				}
				return
			}
			if len(f.updateCalls) != 1 || f.updateCalls[0].Name.Val("") != tt.wantName {
				t.Fatalf("Update calls %+v, want name %q", f.updateCalls, tt.wantName)
			}
			if row.Name != tt.wantName {
				t.Errorf("row name %q, want %q", row.Name, tt.wantName)
			}
			if (row.Before != nil) != tt.wantUndo || (row.Before != nil && row.Before.Name != "The Drakari") {
				t.Errorf("Before = %+v, want undo %v back to the old name", row.Before, tt.wantUndo)
			}
		})
	}
}
