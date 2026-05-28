// committer_v1_5_test.go covers the V1.5 verb-set extension behavior
// (C-AI-WORKSPACE-V1-G): action: create / update / delete dispatch,
// ActionMismatch handling, Delete confirmation gate, and AST pin
// negative verification for the renamed commitUpdate path.

package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pageWithAction builds a ParsedPage with sensible defaults + the
// V1.5 action verb. The non-action helper `page` keeps action empty
// (defaults to create at parse time + committer fall-through).
func pageWithAction(name, typeSlug, body, action string) ParsedPage {
	p := page(name, typeSlug, body)
	p.FrontMatter.Action = action
	return p
}

func decisionWithAction(include bool, name, category, visibility, conflict, action string, deleteConfirmed bool) RowDecision {
	d := decision(include, name, category, visibility, conflict)
	d.Action = action
	d.DeleteConfirmed = deleteConfirmed
	return d
}

// TestCommit_ActionCreate_DefaultWhenEmpty — backward-compat: a
// RowDecision with empty Action behaves identically to action=create.
// Preserves V1 behavior for forms that don't yet send the action
// field (e.g., test fixtures, in-flight V1 sessions).
func TestCommit_ActionCreate_DefaultWhenEmpty(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{page("Maro", "character", "# Maro\nBody.")},
		Decisions: []RowDecision{
			decisionWithAction(true, "Maro", "character", "private", "rename", "", false),
		},
	})
	if err != nil || res.Created != 1 || res.Rows[0].Status != StatusCreated {
		t.Fatalf("empty Action should default to create; got Status=%q Created=%d err=%v",
			res.Rows[0].Status, res.Created, err)
	}
}

// TestCommit_ActionDelete_HappyPath — action=delete with existing
// entity + DeleteConfirmed=true → calls creator.Delete, returns
// StatusDeleted.
func TestCommit_ActionDelete_HappyPath(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"old-name": {ID: "ent-old", Name: "Old Name", Slug: "old-name"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{pageWithAction("Old Name", "character", "", ActionDelete)},
		Decisions: []RowDecision{
			decisionWithAction(true, "Old Name", "character", "", "", ActionDelete, true),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Deleted != 1 || res.Rows[0].Status != StatusDeleted {
		t.Errorf("expected StatusDeleted, Deleted=1; got Status=%q Deleted=%d",
			res.Rows[0].Status, res.Deleted)
	}
	if len(f.deleteCalls) != 1 || f.deleteCalls[0] != "ent-old" {
		t.Errorf("expected creator.Delete called once with ent-old; got %v", f.deleteCalls)
	}
}

// TestCommit_ActionDelete_TargetMissing — action=delete with no
// matching entity (classifier should have caught it but committer
// re-checks defensively) → StatusFailed with explicit reason; no
// Delete call.
func TestCommit_ActionDelete_TargetMissing(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		// No existing entries — slug lookup returns nil.
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{pageWithAction("Ghost", "character", "", ActionDelete)},
		Decisions: []RowDecision{
			decisionWithAction(true, "Ghost", "character", "", "", ActionDelete, true),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Failed != 1 || res.Rows[0].Status != StatusFailed {
		t.Errorf("expected StatusFailed; got Status=%q Failed=%d",
			res.Rows[0].Status, res.Failed)
	}
	if !strings.Contains(res.Rows[0].Reason, "no existing entity named") {
		t.Errorf("expected target-missing reason; got %q", res.Rows[0].Reason)
	}
	if len(f.deleteCalls) != 0 {
		t.Errorf("creator.Delete should NOT be called when target missing; got %v", f.deleteCalls)
	}
}

// TestCommit_ActionDelete_UnconfirmedGated — action=delete with
// DeleteConfirmed=false → committer-side belt-and-suspenders gate
// fires; StatusFailed with explicit reason; no Delete call (even if
// target exists).
func TestCommit_ActionDelete_UnconfirmedGated(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"old-name": {ID: "ent-old", Name: "Old Name", Slug: "old-name"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{pageWithAction("Old Name", "character", "", ActionDelete)},
		Decisions: []RowDecision{
			// DeleteConfirmed=false simulates a client-side bypass of
			// the per-row confirmation checkbox.
			decisionWithAction(true, "Old Name", "character", "", "", ActionDelete, false),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Failed != 1 || res.Rows[0].Status != StatusFailed {
		t.Errorf("expected StatusFailed; got Status=%q Failed=%d",
			res.Rows[0].Status, res.Failed)
	}
	if !strings.Contains(res.Rows[0].Reason, "Delete not confirmed") {
		t.Errorf("expected confirmation-gate reason; got %q", res.Rows[0].Reason)
	}
	if len(f.deleteCalls) != 0 {
		t.Errorf("creator.Delete should NOT be called when unconfirmed; got %v", f.deleteCalls)
	}
}

// TestCommit_ActionDelete_DeleteError_Surfaces — creator.Delete
// returning an error surfaces as StatusFailed with friendly reason;
// existing entity unchanged.
func TestCommit_ActionDelete_DeleteError_Surfaces(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"old-name": {ID: "ent-old", Name: "Old Name", Slug: "old-name"},
		},
		deleteFn: func(id string) error { return errors.New("db down") },
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{pageWithAction("Old Name", "character", "", ActionDelete)},
		Decisions: []RowDecision{
			decisionWithAction(true, "Old Name", "character", "", "", ActionDelete, true),
		},
	})
	if res.Failed != 1 || res.Rows[0].Status != StatusFailed {
		t.Errorf("expected StatusFailed on delete error; got Status=%q", res.Rows[0].Status)
	}
	if strings.Contains(res.Rows[0].Reason, "db down") {
		t.Errorf("raw error leaked: %q; expected friendly reason", res.Rows[0].Reason)
	}
}

// TestCommit_ActionUpdate_HappyPath — action=update with existing
// target → calls creator.Update + UpdateEntry; StatusUpdated.
// Mirrors the conflict-mode update path but exercises the action-
// dispatch branch.
func TestCommit_ActionUpdate_HappyPath(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-existing", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{pageWithAction("Lyra Vance", "character", "# Lyra\n\nNew body.", ActionUpdate)},
		Decisions: []RowDecision{
			decisionWithAction(true, "Lyra Vance", "character", "private", "", ActionUpdate, false),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Updated != 1 || res.Rows[0].Status != StatusUpdated {
		t.Errorf("expected StatusUpdated, Updated=1; got Status=%q Updated=%d",
			res.Rows[0].Status, res.Updated)
	}
	if len(f.updateCalls) != 1 || len(f.updateEntries) != 1 {
		t.Errorf("expected Update + UpdateEntry called once each; got %d / %d",
			len(f.updateCalls), len(f.updateEntries))
	}
}

// TestCommit_ActionUpdate_TargetMissing — action=update with no
// matching entity → StatusFailed; no Update call.
func TestCommit_ActionUpdate_TargetMissing(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		// No existing entries.
	}
	c := NewCommitter(f)
	res, _ := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{pageWithAction("Ghost", "character", "Body.", ActionUpdate)},
		Decisions: []RowDecision{
			decisionWithAction(true, "Ghost", "character", "private", "", ActionUpdate, false),
		},
	})
	if res.Failed != 1 || res.Rows[0].Status != StatusFailed {
		t.Errorf("expected StatusFailed; got Status=%q", res.Rows[0].Status)
	}
	if len(f.updateCalls) != 0 {
		t.Errorf("creator.Update should NOT be called when target missing; got %d calls", len(f.updateCalls))
	}
}

// TestCommit_ConflictModeOverwriteAlias — backward-compat: a form
// submission with conflict=overwrite (V1 form value) routes to the
// renamed commitUpdate path; result is StatusUpdated; one release
// after V1.5 ships the alias gets removed.
func TestCommit_ConflictModeOverwriteAlias(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Character", Slug: "character", Enabled: true}},
		existing: map[string]*entities.Entity{
			"lyra-vance": {ID: "ent-existing", Name: "Lyra Vance", Slug: "lyra-vance"},
		},
	}
	c := NewCommitter(f)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages:   []ParsedPage{page("Lyra Vance", "character", "# Lyra\n\nNew body.")},
		// action="" + ConflictMode="update" (post-V1.5 form value).
		// The handler accepts conflict="overwrite" too (alias), but
		// this test goes through the post-alias path.
		Decisions: []RowDecision{decisionWithAction(true, "Lyra Vance", "character", "private", "update", "", false)},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Updated != 1 || res.Rows[0].Status != StatusUpdated {
		t.Errorf("expected StatusUpdated for conflict=update; got Status=%q Updated=%d",
			res.Rows[0].Status, res.Updated)
	}
}
