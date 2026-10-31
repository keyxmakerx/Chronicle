// copies_service.go keeps remembered copies with their count as items move,
// get used and are made. Every function runs under the campaign lock its
// caller holds, and does nothing when the copy store is off (the armory
// plugin's own table did not migrate): items are then plain, as before.
package armory

import (
	"context"
	"fmt"
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
func (s *stashService) takeCopies(ctx context.Context, campaignID string, from Endpoint, to *Endpoint, itemID string, totalBefore, take int, pick []CopyPick) []Copy {
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
func (s *stashService) pickedFor(ctx context.Context, m *Move) []CopyPick {
	if m.pick > 0 {
		return []CopyPick{{ID: m.pick}}
	}
	if s.Copies == nil || m.ID == 0 {
		return nil
	}
	id, err := s.Copies.Picked(ctx, m.ID)
	if err != nil || id == 0 {
		return nil
	}
	return []CopyPick{{ID: id}}
}

// GoOff turns every copy due to go off by day into what it becomes: at its
// holder, that many of the item become that many of the other item, and the
// copy ends, with a line in the holder's history. A copy that becomes
// nothing stays, marked gone off. Only the Director may, since it
// changes every hero's pack at once; it returns how many copies turned.
func (s *stashService) GoOff(ctx context.Context, campaignID string, a Actor, day int64) (int, error) {
	if !a.IsOwner() {
		return 0, forbidden()
	}
	if s.Copies == nil || day <= 0 {
		return 0, nil
	}
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()

	mctx := context.WithoutCancel(ctx)
	due, err := s.Copies.DueOff(mctx, campaignID, day)
	if err != nil {
		return 0, err
	}
	turned := 0
	for _, c := range due {
		if c.Memory.Off {
			continue
		}
		if c.Memory.Becomes == "" || c.Memory.Becomes == c.ItemID {
			m := c.Memory
			m.Off = true
			if err := s.Copies.Remember(mctx, campaignID, c.ID, m); err != nil {
				return turned, err
			}
			continue
		}
		at := Endpoint{Kind: c.HolderKind, ID: c.HolderID}
		total, err := s.heldCount(mctx, campaignID, at, c.ItemID)
		if err != nil {
			return turned, err
		}
		// The holder's copies are fitted to its count, oldest first: a copy
		// the count no longer covers went with a change made outside the
		// Armory and has nothing left to turn.
		fit, err := s.copiesAt(mctx, campaignID, at, c.ItemID, total)
		if err != nil {
			return turned, err
		}
		n := 0
		for _, f := range fit {
			if f.ID == c.ID {
				n = f.Quantity
			}
		}
		if n == 0 {
			if err := s.Copies.SetQuantity(mctx, campaignID, c.ID, 0); err != nil {
				return turned, err
			}
			continue
		}
		from, err := s.loadItem(mctx, campaignID, c.ItemID)
		if err != nil {
			return turned, err
		}
		into, err := s.loadItem(mctx, campaignID, c.Memory.Becomes)
		if err != nil {
			// The item it becomes was deleted: it only goes off.
			continue
		}
		m := &Move{
			CampaignID: campaignID, Kind: MoveKindItem, ItemEntityID: into.ID, Quantity: n,
			From: at, To: at, Status: MoveApplied, RequestedBy: a.UserID, seesDmOnly: true,
			Reason: clipReason(fmt.Sprintf("%d × %s went off and became %s", n, from.Name, into.Name)),
		}
		take := &Move{CampaignID: campaignID, Kind: MoveKindItem, ItemEntityID: c.ItemID, Quantity: n, RequestedBy: a.UserID, seesDmOnly: true}
		ok, err := s.debit(mctx, take, at, nil)
		if err != nil {
			return turned, err
		}
		if !ok {
			continue
		}
		if err := s.credit(mctx, m, at, nil); err != nil {
			if rerr := s.credit(mctx, take, at, nil); rerr != nil {
				slog.Error("armory copies: RESTORE FAILED after going off", slog.String("campaign_id", campaignID),
					slog.Int64("copy_id", c.ID), slog.Any("error", rerr))
			}
			return turned, err
		}
		if err := s.Copies.SetQuantity(mctx, campaignID, c.ID, c.Quantity-n); err != nil {
			slog.Error("armory copies: went off but the copy stays", slog.String("campaign_id", campaignID),
				slog.Int64("copy_id", c.ID), slog.Any("error", err))
		}
		if at.Kind == EndpointCharacter {
			s.dropEmptyCarried(mctx, campaignID, at.ID, c.ItemID)
		}
		turned += n
		if err := s.Repo.InsertMove(mctx, m); err != nil {
			slog.Error("armory copies: WENT OFF BUT NOT RECORDED in the move history", slog.String("campaign_id", campaignID),
				slog.Int64("copy_id", c.ID), slog.Any("error", err))
			continue
		}
		ev.moved(m)
	}
	return turned, nil
}
