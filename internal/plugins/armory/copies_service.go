// copies_service.go keeps remembered copies with their count as items move,
// get used and are made. Every function runs under the campaign lock its
// caller holds, and does nothing when the copy store is off (the armory
// plugin's own table did not migrate): items are then plain, as before.
package armory

import (
	"context"
	"log/slog"
)

// holderOf names an endpoint as a copy's holder.
func holderOf(e Endpoint) (kind, id string) { return e.Kind, e.ID }

// heldCount is how many of an item a holder has, DM-only lines included:
// copies follow the count whoever is looking.
func (s *stashService) heldCount(ctx context.Context, campaignID string, at Endpoint, itemID string) (int, error) {
	if at.Kind == EndpointCharacter {
		rel, err := s.hasItem(ctx, campaignID, at.ID, itemID, true)
		if err != nil || rel == nil {
			return 0, err
		}
		q, _ := parseCarried(rel.Metadata)
		return q, nil
	}
	sid, ok := at.StashID()
	if !ok {
		return 0, nil
	}
	all, err := s.Repo.ListItems(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	for _, r := range all[sid] {
		if r.ItemEntityID == itemID {
			return r.Quantity, nil
		}
	}
	return 0, nil
}

// copiesAt reads a holder's remembered copies of one item, fitted to its
// count.
func (s *stashService) copiesAt(ctx context.Context, campaignID string, at Endpoint, itemID string, total int) ([]Copy, error) {
	if s.Copies == nil {
		return nil, nil
	}
	kind, id := holderOf(at)
	cs, err := s.Copies.List(ctx, campaignID, kind, id, itemID)
	if err != nil {
		return nil, err
	}
	return fitCopies(cs, total), nil
}

// takeCopies takes the copies that leave when take of an item go from a
// holder that had totalBefore, and either hands them to another holder
// (to non-nil) or ends them (used up). It returns what they remembered.
// A failure is logged, never fatal: the count has already moved, and a
// lost memory is better than a refused move.
func (s *stashService) takeCopies(ctx context.Context, campaignID string, from Endpoint, to *Endpoint, itemID string, totalBefore, take int, pick []int64) []Copy {
	if s.Copies == nil || take <= 0 {
		return nil
	}
	kind, id := holderOf(from)
	all, err := s.Copies.List(ctx, campaignID, kind, id, itemID)
	if err != nil {
		slog.Error("armory copies: cannot read", slog.String("campaign_id", campaignID), slog.String("item_id", itemID), slog.Any("error", err))
		return nil
	}
	if len(all) == 0 {
		return nil
	}
	rowQty := map[int64]int{}
	for _, c := range all {
		rowQty[c.ID] = c.Quantity
	}
	leaving := planTake(all, totalBefore, take, pick)
	for _, c := range leaving {
		switch {
		case to != nil && c.Quantity == rowQty[c.ID]:
			tk, tid := holderOf(*to)
			err = s.Copies.Rehome(ctx, campaignID, c.ID, tk, tid)
		case to != nil:
			if err = s.Copies.SetQuantity(ctx, campaignID, c.ID, rowQty[c.ID]-c.Quantity); err == nil {
				tk, tid := holderOf(*to)
				_, err = s.Copies.Add(ctx, campaignID, tk, tid, c)
			}
		default:
			err = s.Copies.SetQuantity(ctx, campaignID, c.ID, rowQty[c.ID]-c.Quantity)
		}
		if err != nil {
			slog.Error("armory copies: MEMORY LOST on a move", slog.String("campaign_id", campaignID),
				slog.Int64("copy_id", c.ID), slog.Any("error", err))
		}
	}
	return leaving
}

// remember stores qty copies of an item at a holder with what they
// remember; plain copies store nothing.
func (s *stashService) remember(ctx context.Context, campaignID string, at Endpoint, itemID string, qty int, m Memory) {
	if s.Copies == nil || qty <= 0 {
		return
	}
	if m = m.Clean(); m.Empty() {
		return
	}
	kind, id := holderOf(at)
	if _, err := s.Copies.Add(ctx, campaignID, kind, id, Copy{ItemID: itemID, Quantity: qty, Memory: m}); err != nil {
		slog.Error("armory copies: MEMORY LOST on a give", slog.String("campaign_id", campaignID),
			slog.String("item_id", itemID), slog.Any("error", err))
	}
}

// pickedFor is the copy a move asked for, kept with the move while it waits.
func (s *stashService) pickedFor(ctx context.Context, m *Move) []int64 {
	if m.pick > 0 {
		return []int64{m.pick}
	}
	if s.Copies == nil || m.ID == 0 {
		return nil
	}
	id, err := s.Copies.Picked(ctx, m.ID)
	if err != nil || id == 0 {
		return nil
	}
	return []int64{id}
}
