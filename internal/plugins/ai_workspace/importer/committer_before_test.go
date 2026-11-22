package importer

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func str(s string) *string { return &s }

// beforeRig is one existing page, "Lyra Vance", whose text an update changes.
func beforeRig() *fakeCreator {
	f := &fakeCreator{types: charTypes(), existing: map[string]*entities.Entity{
		"lyra-vance": {ID: "ent-lyra", CampaignID: "camp-1", Name: "Lyra Vance", Slug: "lyra-vance",
			Entry: str(`{"old":true}`), EntryHTML: str("<p>Old text</p>")},
	}}
	// The service writes the page: the live entity follows the writes.
	f.updateEntryFn = func(id, j, h string) error {
		f.existing["lyra-vance"].Entry, f.existing["lyra-vance"].EntryHTML = str(j), str(h)
		return nil
	}
	return f
}

func commitRows(t *testing.T, f *fakeCreator, pages []ParsedPage, decs []RowDecision) []RowOutcome {
	t.Helper()
	res, err := NewCommitter(f).Commit(context.Background(), "camp-1", CommitInput{OwnerID: "u-1", Pages: pages, Decisions: decs})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return res.Rows
}

func updateDecision() RowDecision {
	return RowDecision{Include: true, Name: "Lyra Vance", Action: ActionUpdate, CategorySpec: "character"}
}

// An update that changes only the title and text keeps the page as it was and
// a fingerprint of the page as the request left it.
func TestCommitBefore_UpdateKeepsTextAndFingerprint(t *testing.T) {
	f := beforeRig()
	rows := commitRows(t, f, []ParsedPage{page("Lyra Vance", "character", "New text")}, []RowDecision{updateDecision()})
	b := rows[0].Before
	if b == nil || b.Action != ActionUpdate || b.EntityID != "ent-lyra" {
		t.Fatalf("Before = %+v", b)
	}
	if b.Name != "Lyra Vance" || *b.Entry != `{"old":true}` || *b.EntryHTML != "<p>Old text</p>" {
		t.Errorf("kept the wrong before: %+v", b)
	}
	if b.AfterHash != PageFingerprint(f.existing["lyra-vance"]) || b.AfterHash == "" {
		t.Errorf("AfterHash is not the fingerprint of the page as left")
	}
	// The kept before is a copy: a later write to the page does not change it.
	if *b.EntryHTML == *f.existing["lyra-vance"].EntryHTML {
		t.Error("the before follows the live page")
	}
}

func TestCommitBefore_Cases(t *testing.T) {
	tags := page("Lyra Vance", "character", "New text")
	tags.FrontMatter.Tags = []string{"npc"}
	fields := page("Lyra Vance", "character", "New text")
	fields.FrontMatter.Fields = map[string]any{"level": 3}
	parent := page("Lyra Vance", "character", "New text")
	parent.FrontMatter.Parent = "Elsewhere"
	vis := page("Lyra Vance", "character", "New text")
	vis.FrontMatter.Visibility = "public"
	textless := page("Lyra Vance", "character", "")

	tests := []struct {
		name string
		page ParsedPage
		want bool
	}{
		{"text only", page("Lyra Vance", "character", "New text"), true},
		{"also tags", tags, false},
		{"also sheet fields", fields, false},
		{"also a parent", parent, false},
		{"also visibility", vis, false},
		{"no text written", textless, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := commitRows(t, beforeRig(), []ParsedPage{tt.page}, []RowDecision{updateDecision()})
			if got := rows[0].Before != nil; got != tt.want {
				t.Errorf("Before kept = %v, want %v (status %s %s)", got, tt.want, rows[0].Status, rows[0].Reason)
			}
		})
	}
}

func TestCommitBefore_CreateAndDelete(t *testing.T) {
	f := beforeRig()
	rows := commitRows(t, f,
		[]ParsedPage{page("Sable Quay", "character", "A harbour."), page("Lyra Vance", "character", "")},
		[]RowDecision{
			{Include: true, Name: "Sable Quay", Action: ActionCreate, CategorySpec: "character"},
			{Include: true, Name: "Lyra Vance", Action: ActionDelete, DeleteConfirmed: true},
		})
	c := rows[0].Before
	if c == nil || c.Action != ActionCreate || c.EntityID != "ent-sable-quay" || c.AfterHash == "" || c.Entry != nil {
		t.Errorf("create Before = %+v", c)
	}
	d := rows[1].Before
	if d == nil || d.Action != ActionDelete || d.EntityID != "ent-lyra" {
		t.Errorf("delete Before = %+v", d)
	}
}

// A page two rows write keeps the earliest before, once, with the final
// fingerprint, so Undo goes back to the page as it was before the request.
func TestCommitBefore_SamePageTwiceKeepsFirst(t *testing.T) {
	f := beforeRig()
	rows := commitRows(t, f,
		[]ParsedPage{page("Lyra Vance", "character", "First"), page("Lyra Vance", "character", "Second")},
		[]RowDecision{updateDecision(), updateDecision()})
	if rows[0].Before == nil || rows[1].Before != nil {
		t.Fatalf("befores = %+v / %+v", rows[0].Before, rows[1].Before)
	}
	if *rows[0].Before.EntryHTML != "<p>Old text</p>" {
		t.Errorf("kept %q, want the original text", *rows[0].Before.EntryHTML)
	}
	if rows[0].Before.AfterHash != PageFingerprint(f.existing["lyra-vance"]) {
		t.Error("fingerprint is not of the final state")
	}
}

// A failed or skipped row keeps nothing.
func TestCommitBefore_FailedRowKeepsNothing(t *testing.T) {
	f := beforeRig()
	f.updateEntryFn = func(string, string, string) error { return context.DeadlineExceeded }
	rows := commitRows(t, f, []ParsedPage{page("Lyra Vance", "character", "New")}, []RowDecision{updateDecision()})
	if rows[0].Status != StatusFailed || rows[0].Before != nil {
		t.Errorf("row = %+v", rows[0])
	}
	rows = commitRows(t, beforeRig(), []ParsedPage{page("Lyra Vance", "character", "New")}, []RowDecision{{Include: false}})
	if rows[0].Before != nil {
		t.Error("an excluded row kept a before")
	}
}
