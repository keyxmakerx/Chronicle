// stash_count.go lets the GM set how many of an Armory item a character
// carries in one step, for changes that arrive as a number rather than a
// give: an AI import the owner approved. Raising the count is a give, with
// its history line and the player's bell; lowering it takes plain copies
// first and then the oldest remembered ones, as every Armory take does, and
// writes a line saying the GM took them away. A hand edit of the inventory
// widget leaves no line; this does.
package armory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CarriedCount is what SetCarried did: how many the character held before
// and holds now.
type CarriedCount struct {
	Before, After int
}

func (s *stashService) SetCarried(ctx context.Context, campaignID string, a Actor, characterID, itemID string, qty int) (*CarriedCount, error) {
	if err := s.requireOwner(a); err != nil {
		return nil, err
	}
	if qty < 0 || qty > maxMoveQuantity {
		return nil, apperror.NewBadRequest("Enter a quantity from 0 to 1,000,000.")
	}
	char, err := s.loadCharacter(ctx, campaignID, strings.TrimSpace(characterID))
	if err != nil {
		return nil, err
	}
	item, err := s.loadItem(ctx, campaignID, itemID)
	if err != nil {
		return nil, err
	}

	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	held, err := s.heldCount(ctx, campaignID, Endpoint{Kind: EndpointCharacter, ID: char.ID}, item.ID)
	if err != nil {
		unlock()
		return nil, err
	}
	out := &CarriedCount{Before: held, After: qty}
	var given *GiveOutcome
	switch {
	case qty > held:
		given, err = s.give(ctx, campaignID, a, char, item, nil, qty-held, &ev)
	case qty < held:
		err = s.takeAway(ctx, campaignID, a, char, item, held, held-qty, &ev)
	}
	unlock()
	if err != nil {
		return nil, err
	}
	if given != nil {
		s.notifyGiven(ctx, campaignID, a, char, given, qty-held)
	}
	return out, nil
}

// takeAway takes n of an item from a character that held held, under the
// campaign lock, and writes the history line. The line is a give-shaped row
// (both ends the character) whose reason says what happened, so every
// history reader shows it as a note rather than a move.
func (s *stashService) takeAway(ctx context.Context, campaignID string, a Actor, char, item *EntityRef, held, n int, ev *eventBatch) error {
	mctx := context.WithoutCancel(ctx)
	ok, err := s.adjustCarried(mctx, campaignID, char.ID, item.ID, a.UserID, -n, true, true)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewConflict("That character's items changed. Try again.")
	}
	self := Endpoint{Kind: EndpointCharacter, ID: char.ID}
	s.takeCopies(mctx, campaignID, self, nil, item.ID, held, n, nil)
	s.dropEmptyCarried(mctx, campaignID, char.ID, item.ID)
	m := &Move{
		CampaignID: campaignID, Kind: MoveKindItem, ItemEntityID: item.ID, Quantity: n,
		From: self, To: self, Status: MoveApplied, RequestedBy: a.UserID, seesDmOnly: true,
		Reason: clipReason(fmt.Sprintf("The GM took away %d × %s", n, item.Name)),
	}
	if err := s.Repo.InsertMove(mctx, m); err != nil {
		slog.Error("set carried: TAKEN BUT NOT RECORDED in the move history",
			slog.String("campaign_id", campaignID), slog.String("character_id", char.ID),
			slog.String("item_id", item.ID), slog.Any("error", err))
		return nil
	}
	ev.moved(m)
	return nil
}
