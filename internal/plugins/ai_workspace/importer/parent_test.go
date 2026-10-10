// parent_test.go covers the `parent:` front-matter key: the review-time
// problems the classifier reports and the committer's create / move /
// lift / keep behavior through the entities service contract.

package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func withParent(name, parent, action string) ParsedPage {
	p := page(name, "location", "# "+name+"\nBody.")
	p.FrontMatter.Parent = parent
	p.FrontMatter.Action = action
	return p
}

func TestClassify_ParentProblems(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{
			"harbor": {ID: "ent-harbor", Name: "Harbor", Slug: "harbor"},
			"docks":  {ID: "ent-docks", Name: "Docks", Slug: "docks"},
		},
		types: []entities.EntityType{{ID: 1, Slug: "location", Name: "Location"}},
	}
	tests := []struct {
		name       string
		pages      []ParsedPage
		row        int
		wantStatus ParseStatus
		wantReason string
	}{
		{"existing parent", []ParsedPage{withParent("Pier", "Harbor", "")}, 0, StatusNew, ""},
		{"no parent key", []ParsedPage{withParent("Pier", "", "")}, 0, StatusNew, ""},
		{"none needs no lookup", []ParsedPage{withParent("Docks", "none", "update")}, 0, StatusNew, ""},
		{"parent earlier in paste", []ParsedPage{withParent("Isle", "", ""), withParent("Beach", "Isle", "")}, 1, StatusNew, ""},
		{"missing parent", []ParsedPage{withParent("Pier", "Nowhere", "")}, 0, StatusParentProblem, "No page called"},
		{"self parent", []ParsedPage{withParent("Pier", "pier", "")}, 0, StatusParentProblem, "its own parent"},
		{"parent later in paste", []ParsedPage{withParent("Beach", "Isle", ""), withParent("Isle", "", "")}, 0, StatusParentProblem, "comes later"},
		{"ambiguous in paste", []ParsedPage{withParent("Isle", "", ""), withParent("Isle", "", ""), withParent("Beach", "Isle", "")}, 2, StatusParentProblem, "more than one"},
		{"parent deleted in paste", []ParsedPage{withParent("Harbor", "", "delete"), withParent("Pier", "Harbor", "")}, 1, StatusParentProblem, "being deleted"},
		{"delete row ignores parent", []ParsedPage{withParent("Docks", "Nowhere", "delete")}, 0, StatusNew, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cls, err := NewClassifier(lookup, "camp-1").ClassifyAll(context.Background(), tt.pages)
			if err != nil {
				t.Fatal(err)
			}
			got := cls[tt.row]
			// Pages named like an existing slug classify as conflict for
			// create; only the parent verdict is under test here.
			if tt.wantStatus == StatusParentProblem && got.Status != StatusParentProblem {
				t.Fatalf("status = %q, want parent problem", got.Status)
			}
			if tt.wantStatus != StatusParentProblem && got.Status == StatusParentProblem {
				t.Fatalf("unexpected parent problem: %s", got.Reason)
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestCommit_Parent(t *testing.T) {
	types := []entities.EntityType{{ID: 1, Name: "Location", Slug: "location", Enabled: true}}
	existing := func() map[string]*entities.Entity {
		return map[string]*entities.Entity{
			"harbor": {ID: "ent-harbor", Name: "Harbor", Slug: "harbor"},
			"docks":  {ID: "ent-docks", Name: "Docks", Slug: "docks"},
		}
	}
	tests := []struct {
		name       string
		pages      []ParsedPage
		decisions  []RowDecision
		check      func(t *testing.T, f *fakeCreator, res CommitResult)
		wantStatus []RowStatus
	}{
		{
			name:       "create under existing page",
			pages:      []ParsedPage{withParent("Pier", "Harbor", "")},
			decisions:  []RowDecision{decision(true, "Pier", "location", "private", "rename")},
			wantStatus: []RowStatus{StatusCreated},
			check: func(t *testing.T, f *fakeCreator, _ CommitResult) {
				if f.createCalls[0].ParentID != "ent-harbor" {
					t.Errorf("ParentID = %q", f.createCalls[0].ParentID)
				}
			},
		},
		{
			name:       "create under page from the same paste",
			pages:      []ParsedPage{withParent("Isle", "", ""), withParent("Beach", "Isle", "")},
			decisions:  []RowDecision{decision(true, "Isle", "location", "private", "rename"), decision(true, "Beach", "location", "private", "rename")},
			wantStatus: []RowStatus{StatusCreated, StatusCreated},
			check: func(t *testing.T, f *fakeCreator, _ CommitResult) {
				if f.createCalls[1].ParentID != "ent-isle" {
					t.Errorf("ParentID = %q", f.createCalls[1].ParentID)
				}
			},
		},
		{
			name:       "update moves the page",
			pages:      []ParsedPage{withParent("Docks", "Harbor", ActionUpdate)},
			decisions:  []RowDecision{decisionWithAction(true, "Docks", "location", "", "rename", ActionUpdate, false)},
			wantStatus: []RowStatus{StatusUpdated},
			check: func(t *testing.T, f *fakeCreator, _ CommitResult) {
				got := f.updateCalls[0].ParentID
				if !got.Present() || got.IsNull() || got.Val("") != "ent-harbor" {
					t.Errorf("ParentID = %+v, want ent-harbor", got)
				}
			},
		},
		{
			name:       "update with parent none lifts to top level",
			pages:      []ParsedPage{withParent("Docks", "none", ActionUpdate)},
			decisions:  []RowDecision{decisionWithAction(true, "Docks", "location", "", "rename", ActionUpdate, false)},
			wantStatus: []RowStatus{StatusUpdated},
			check: func(t *testing.T, f *fakeCreator, _ CommitResult) {
				if got := f.updateCalls[0].ParentID; !got.Present() || !got.IsNull() {
					t.Errorf("ParentID = %+v, want explicit null", got)
				}
			},
		},
		{
			name:       "update without parent key keeps the page where it is",
			pages:      []ParsedPage{withParent("Docks", "", ActionUpdate)},
			decisions:  []RowDecision{decisionWithAction(true, "Docks", "location", "", "rename", ActionUpdate, false)},
			wantStatus: []RowStatus{StatusUpdated},
			check: func(t *testing.T, f *fakeCreator, _ CommitResult) {
				if f.updateCalls[0].ParentID.Present() {
					t.Errorf("ParentID must be absent, got %+v", f.updateCalls[0].ParentID)
				}
			},
		},
		{
			name:       "unknown parent fails only that row",
			pages:      []ParsedPage{withParent("Pier", "Nowhere", ""), withParent("Quay", "", "")},
			decisions:  []RowDecision{decision(true, "Pier", "location", "private", "rename"), decision(true, "Quay", "location", "private", "rename")},
			wantStatus: []RowStatus{StatusFailed, StatusCreated},
			check: func(t *testing.T, f *fakeCreator, res CommitResult) {
				if !strings.Contains(res.Rows[0].Reason, "Nowhere") || len(f.createCalls) != 1 {
					t.Errorf("reason %q, creates %d", res.Rows[0].Reason, len(f.createCalls))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{types: types, existing: existing()}
			res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{
				OwnerID: "u-1", Pages: tt.pages, Decisions: tt.decisions,
			})
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range tt.wantStatus {
				if res.Rows[i].Status != want {
					t.Fatalf("row %d status = %q (%s), want %q", i, res.Rows[i].Status, res.Rows[i].Reason, want)
				}
			}
			tt.check(t, f, res)
		})
	}
}
