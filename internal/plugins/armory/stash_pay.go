package armory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
)

const (
	// maxPayCents bounds one payment, like a give's quantity cap.
	maxPayCents = Cents(1_000_000 * 100)
	// maxPayReason bounds the label a payment carries into money history.
	maxPayReason = 120

	payAmountMessage = "Enter an amount from 0.01 to 1,000,000."
	payWealthMessage = "That sheet counts Wealth as a level, so coins can't be added. Raise its Wealth on the sheet."
)

// PayInput is coins the GM adds to one character's sheet. Amount is in the
// sheet's main unit (gold on a 5e sheet), in hundredths. Reason labels the
// line in the character's money history ("as a quest reward").
type PayInput struct {
	CharacterID string
	Amount      Cents
	Reason      string
}

// PayOutcome names who was paid, for the caller's confirmation.
type PayOutcome struct {
	CharacterName string `json:"characterName"`
	Paid          string `json:"paid"`
}

// Pay adds coins to a character's sheet. Owner visibility only. The sheet
// write runs under a change source carrying the reason, so the character's
// money history records it the way it records any other sheet change.
func (s *stashService) Pay(ctx context.Context, campaignID string, a Actor, in PayInput) (*PayOutcome, error) {
	if err := s.requireOwner(a); err != nil {
		return nil, err
	}
	if in.Amount < 1 || in.Amount > maxPayCents {
		return nil, apperror.NewBadRequest(payAmountMessage)
	}
	char, err := s.loadCharacter(ctx, campaignID, strings.TrimSpace(in.CharacterID))
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) > maxPayReason {
		reason = string([]rune(reason)[:maxPayReason])
	}
	wctx := changesource.With(context.WithoutCancel(ctx), changesource.Source{Kind: changesource.KindWeb, Label: reason, UserID: a.UserID})

	unlock := s.locks.lock(campaignID)
	paid, err := s.pay(wctx, char, in.Amount)
	unlock()
	if err != nil {
		return nil, err
	}
	s.notifyPaid(ctx, campaignID, a, char, paid)
	return &PayOutcome{CharacterName: char.Name, Paid: paid}, nil
}

// pay writes the coins under the campaign lock and returns how the payment
// reads ("40 gp", "12 coins").
func (s *stashService) pay(ctx context.Context, char *EntityRef, amount Cents) (string, error) {
	switch buyKind(char) {
	case "":
		return "", apperror.NewBadRequest(noMoneyMessage)
	case BuyKindWealth:
		return "", apperror.NewBadRequest(payWealthMessage)
	case BuyKindPurse:
		deltas, ok := purseDeltas(char.Purse, int64(amount))
		if !ok {
			return "", apperror.NewBadRequest("That sheet has no coin small enough for this amount. Use a whole number of gold.")
		}
		ok, err := s.adjustPurse(ctx, char, deltas)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", apperror.NewBadRequest("That character's coins aren't numbers, so coins can't be added.")
		}
		return formatPurse(deltas), nil
	}
	if ok, err := s.adjustCharacterMoney(ctx, char, amount); err != nil {
		return "", err
	} else if !ok {
		return "", apperror.NewBadRequest("That character's money isn't a number, so coins can't be added.")
	}
	unit := char.MoneyLabel
	if unit == "" {
		unit = char.MoneyKey
	}
	return fmt.Sprintf("%s %s", amount, strings.ToLower(unit)), nil
}

// purseDeltas splits cp (gold hundredths are copper) into the largest coins
// the sheet has among gp, sp and cp. False when a remainder has no coin.
func purseDeltas(purse map[string]string, cp int64) (map[string]int64, bool) {
	out := map[string]int64{}
	for _, coin := range []string{"gp", "sp", "cp"} {
		if _, ok := purse[coin]; !ok {
			continue
		}
		rate := coinRateCp[coin]
		if n := cp / rate; n > 0 {
			out[coin] = n
			cp -= n * rate
		}
	}
	return out, cp == 0
}

// notifyPaid tells the character's claimed player, as a give does.
func (s *stashService) notifyPaid(ctx context.Context, campaignID string, a Actor, char *EntityRef, paid string) {
	if s.Notifier == nil || char.OwnerUserID == "" || char.OwnerUserID == a.UserID {
		return
	}
	link := "/campaigns/" + campaignID + "/entities/" + char.ID
	if err := s.Notifier.ItemGiven(ctx, campaignID, []string{char.OwnerUserID}, "Your GM gave you "+paid, "On "+char.Name, link); err != nil {
		slog.Warn("pay: could not notify the player", slog.Any("error", err))
	}
}
