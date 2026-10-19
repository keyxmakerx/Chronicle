package app

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// familyLinksAdapter lets the Family tree block read a page's relation rows
// through the relations service, so the entities plugin never reaches into the
// relation tables. Which of those rows count as family, and who may see them,
// is decided by the family tree service, not here.
type familyLinksAdapter struct {
	svc relations.RelationService
}

// LinksOf implements entities.FamilyLinks.
func (a *familyLinksAdapter) LinksOf(ctx context.Context, campaignID, entityID string) ([]entities.FamilyLink, error) {
	rels, err := a.svc.ListByEntity(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	out := make([]entities.FamilyLink, 0, len(rels))
	for _, r := range rels {
		out = append(out, entities.FamilyLink{OtherID: r.TargetEntityID, Type: r.RelationType, DMOnly: r.DmOnly})
	}
	return out, nil
}

// familyWriterAdapter lets the Family tree's Edit tab write family relations
// through the relations service, the same path the Relations block uses, so
// both directions of a pair are created and removed together and the live
// events and sync change feed see every row. This file decides nothing about
// who may edit or which types count as family; the entities service does.
type familyWriterAdapter struct {
	familyLinksAdapter
}

func newFamilyWriterAdapter(svc relations.RelationService) *familyWriterAdapter {
	return &familyWriterAdapter{familyLinksAdapter{svc: svc}}
}

func sameRelationType(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func relationTypeIn(t string, list []string) bool {
	for _, x := range list {
		if sameRelationType(t, x) {
			return true
		}
	}
	return false
}

// rowOf finds the one relation source->target of the given type, if any.
func (a *familyWriterAdapter) rowOf(ctx context.Context, campaignID, source, target string, types []string) ([]relations.Relation, error) {
	rels, err := a.svc.ListByEntity(ctx, campaignID, source)
	if err != nil {
		return nil, err
	}
	var out []relations.Relation
	for _, r := range rels {
		if r.TargetEntityID == target && relationTypeIn(r.RelationType, types) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Link implements entities.FamilyWriter. The relations service writes the
// forward row and then the reverse row without a transaction, so a failure
// between them leaves one row; this takes it back so a pair is never left
// half-written, and says so loudly when it cannot.
func (a *familyWriterAdapter) Link(ctx context.Context, campaignID string, l entities.FamilyNewLink) error {
	_, err := a.svc.Create(ctx, campaignID, l.Source, l.Target, l.Type, l.Reverse, l.CreatedBy, l.Metadata, l.DMOnly)
	if err == nil {
		return nil
	}
	fwd, ferr := a.rowOf(ctx, campaignID, l.Source, l.Target, []string{l.Type})
	rev, rerr := a.rowOf(ctx, campaignID, l.Target, l.Source, []string{l.Reverse})
	if ferr != nil || rerr != nil {
		return &entities.FamilyHalfWritten{Err: err}
	}
	if len(fwd) > 0 && len(rev) > 0 {
		// Both rows exist (the reverse was already there): the pair is whole.
		return err
	}
	for _, r := range append(fwd, rev...) {
		if derr := a.svc.Delete(ctx, r.ID); derr != nil {
			return &entities.FamilyHalfWritten{Err: err}
		}
	}
	return err
}

// Unlink implements entities.FamilyWriter.
func (a *familyWriterAdapter) Unlink(ctx context.Context, campaignID, source, target string, types, reverseTypes []string) ([]entities.FamilyNewLink, error) {
	fwd, err := a.rowOf(ctx, campaignID, source, target, types)
	if err != nil {
		return nil, err
	}
	var removed []entities.FamilyNewLink
	for _, r := range fwd {
		if err := a.svc.Delete(ctx, r.ID); err != nil {
			// Delete takes the reverse row first, so a failure can leave only
			// the forward row.
			if rev, rerr := a.rowOf(ctx, campaignID, target, source, reverseTypes); rerr != nil || len(rev) == 0 {
				return removed, &entities.FamilyHalfWritten{Err: err}
			}
			return removed, err
		}
		removed = append(removed, entities.FamilyNewLink{
			Source: r.SourceEntityID, Target: r.TargetEntityID, Type: r.RelationType, Reverse: r.ReverseRelationType,
			DMOnly: r.DmOnly, Metadata: r.Metadata, CreatedBy: r.CreatedBy,
		})
	}
	// A reverse row left without its forward row (an earlier half-written
	// pair) is what keeps a removed link showing on the other page.
	left, err := a.rowOf(ctx, campaignID, target, source, reverseTypes)
	if err != nil {
		return removed, &entities.FamilyHalfWritten{Err: err}
	}
	for _, r := range left {
		if err := a.svc.Delete(ctx, r.ID); err != nil {
			return removed, &entities.FamilyHalfWritten{Err: err}
		}
	}
	return removed, nil
}
