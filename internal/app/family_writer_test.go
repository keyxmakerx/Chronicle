package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// memRelations is the relations service as the Family tree's writer sees it:
// Create writes the forward row then the reverse row, Delete removes a row and
// its reverse, and either step can be made to fail.
type memRelations struct {
	relations.RelationService
	rows        []relations.Relation
	next        int
	failReverse bool // Create writes the forward row and then fails
	failDelete  bool
}

func (m *memRelations) Create(_ context.Context, campaignID, src, dst, typ, rev, by string, meta json.RawMessage, dmOnly ...bool) (*relations.Relation, error) {
	dm := len(dmOnly) > 0 && dmOnly[0]
	m.next++
	fwd := relations.Relation{ID: m.next, CampaignID: campaignID, SourceEntityID: src, TargetEntityID: dst, RelationType: typ, ReverseRelationType: rev, Metadata: meta, DmOnly: dm, CreatedBy: by}
	m.rows = append(m.rows, fwd)
	if m.failReverse {
		return nil, errors.New("reverse row failed")
	}
	m.next++
	m.rows = append(m.rows, relations.Relation{ID: m.next, CampaignID: campaignID, SourceEntityID: dst, TargetEntityID: src, RelationType: rev, ReverseRelationType: typ, DmOnly: dm, CreatedBy: by})
	return &fwd, nil
}

func (m *memRelations) ListByEntity(_ context.Context, _, id string) ([]relations.Relation, error) {
	var out []relations.Relation
	for _, r := range m.rows {
		if r.SourceEntityID == id {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memRelations) Delete(_ context.Context, id int) error {
	if m.failDelete {
		return errors.New("delete failed")
	}
	var target *relations.Relation
	for i := range m.rows {
		if m.rows[i].ID == id {
			target = &m.rows[i]
		}
	}
	if target == nil {
		return errors.New("not found")
	}
	t := *target
	var keep []relations.Relation
	for _, r := range m.rows {
		isRev := r.SourceEntityID == t.TargetEntityID && r.TargetEntityID == t.SourceEntityID && r.RelationType == t.ReverseRelationType
		if r.ID == id || isRev {
			continue
		}
		keep = append(keep, r)
	}
	m.rows = keep
	return nil
}

func (m *memRelations) describe() string {
	var s []string
	for _, r := range m.rows {
		s = append(s, r.SourceEntityID+" "+r.RelationType+" "+r.TargetEntityID)
	}
	return strings.Join(s, "; ")
}

// The writer reuses the relations service's own two-row create and delete, so
// what the Edit tab writes is exactly what the Relations block would.
func TestFamilyWriterUsesTheRelationsPath(t *testing.T) {
	ctx := context.Background()

	t.Run("link writes both directions with the link's own flags", func(t *testing.T) {
		m := &memRelations{}
		w := newFamilyWriterAdapter(m)
		err := w.Link(ctx, "c1", entities.FamilyNewLink{Source: "tam", Target: "maud", Type: "child of", Reverse: "parent of", CreatedBy: "u1", DMOnly: true, Metadata: json.RawMessage(`{"k":1}`)})
		if err != nil {
			t.Fatal(err)
		}
		if got := m.describe(); got != "tam child of maud; maud parent of tam" {
			t.Errorf("rows = %q", got)
		}
		if !m.rows[0].DmOnly || !m.rows[1].DmOnly || m.rows[0].CreatedBy != "u1" {
			t.Errorf("flags not carried: %+v", m.rows)
		}
	})

	t.Run("a failure between the two rows takes the first back", func(t *testing.T) {
		m := &memRelations{failReverse: true}
		w := newFamilyWriterAdapter(m)
		err := w.Link(ctx, "c1", entities.FamilyNewLink{Source: "tam", Target: "maud", Type: "child of", Reverse: "parent of"})
		var half *entities.FamilyHalfWritten
		if err == nil || errors.As(err, &half) {
			t.Fatalf("want the plain error after a clean takeback, got %v", err)
		}
		if len(m.rows) != 0 {
			t.Errorf("a half pair was left: %q", m.describe())
		}
	})

	t.Run("a takeback that fails is reported as half-written", func(t *testing.T) {
		m := &memRelations{failReverse: true}
		w := newFamilyWriterAdapter(m)
		m.failDelete = true
		err := w.Link(ctx, "c1", entities.FamilyNewLink{Source: "tam", Target: "maud", Type: "child of", Reverse: "parent of"})
		var half *entities.FamilyHalfWritten
		if !errors.As(err, &half) {
			t.Fatalf("want FamilyHalfWritten, got %v", err)
		}
	})

	t.Run("unlink removes both rows and hands back what it removed", func(t *testing.T) {
		m := &memRelations{}
		w := newFamilyWriterAdapter(m)
		_ = w.Link(ctx, "c1", entities.FamilyNewLink{Source: "tam", Target: "maud", Type: "Child Of ", Reverse: "parent of", DMOnly: true})
		_, _ = m.Create(ctx, "c1", "tam", "maud", "knows", "knows", "u", nil)
		removed, err := w.Unlink(ctx, "c1", "tam", "maud", []string{"child of", "adopted child of"}, []string{"parent of"})
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 || removed[0].Type != "Child Of " || !removed[0].DMOnly || removed[0].Reverse != "parent of" {
			t.Errorf("removed = %+v", removed)
		}
		if got := m.describe(); got != "tam knows maud; maud knows tam" {
			t.Errorf("only the family pair should go, rows = %q", got)
		}
	})

	t.Run("a reverse row left alone is cleaned up", func(t *testing.T) {
		m := &memRelations{rows: []relations.Relation{{ID: 90, SourceEntityID: "maud", TargetEntityID: "tam", RelationType: "parent of", ReverseRelationType: "child of"}}}
		w := newFamilyWriterAdapter(m)
		if _, err := w.Unlink(ctx, "c1", "tam", "maud", []string{"child of"}, []string{"parent of"}); err != nil {
			t.Fatal(err)
		}
		if len(m.rows) != 0 {
			t.Errorf("rows = %q", m.describe())
		}
	})

	t.Run("an unlink that fails after the reverse went is flagged", func(t *testing.T) {
		m := &memRelations{rows: []relations.Relation{{ID: 1, SourceEntityID: "tam", TargetEntityID: "maud", RelationType: "child of", ReverseRelationType: "parent of"}}}
		w := newFamilyWriterAdapter(m)
		m.failDelete = true
		_, err := w.Unlink(ctx, "c1", "tam", "maud", []string{"child of"}, []string{"parent of"})
		var half *entities.FamilyHalfWritten
		if !errors.As(err, &half) {
			t.Fatalf("want FamilyHalfWritten (the reverse row is already gone), got %v", err)
		}
	})
}
