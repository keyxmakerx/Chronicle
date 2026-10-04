// money_history.go keeps a history line for money that changes on a
// character's sheet outside a stash move (typed in on the page, pushed from
// Foundry, changed by an extension). Without it, a balance would change with
// no trace while moves are fully logged.
//
// The line is an item_moves row from the character to itself, so the existing
// history views show it with no new table.
package armory

import (
	"context"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/changesource"
)

// MoneyFieldKeys is the order in which a character type's money field is
// looked for: the first numeric field whose key is one of these. Both the
// directory adapter and the history recorder rely on the same list.
var MoneyFieldKeys = []string{"gp", "coins", "gold", "money", "wealth"}

// maxReasonLen is the width of item_moves.reason.
const maxReasonLen = 255

// MoneyHistory writes the history line for a sheet money edit.
type MoneyHistory struct {
	repo      StashRepository
	directory StashDirectory
	enabled   func(ctx context.Context, campaignID string) bool
	events    StashEventPublisher
}

// NewMoneyHistory creates the recorder. enabled says whether the plugin is on
// for a campaign (nil means always); events is optional.
func NewMoneyHistory(repo StashRepository, directory StashDirectory, enabled func(ctx context.Context, campaignID string) bool, events StashEventPublisher) *MoneyHistory {
	return &MoneyHistory{repo: repo, directory: directory, enabled: enabled, events: events}
}

// sourcePhrase says where an edit came from, or false when the edit should
// not be logged here.
func sourcePhrase(src changesource.Source) (string, bool) {
	if src.Label != "" {
		return src.Label, true
	}
	switch src.Kind {
	case changesource.KindFoundry:
		return "in Foundry", true
	case changesource.KindWeb:
		return "on the sheet", true
	case changesource.KindExtension:
		return "by an extension", true
	case changesource.KindShop:
		return "at a shop", true
	}
	return "", false
}

// purseCoinKeys are the extra 5e coin fields a purse sheet carries beside gp.
var purseCoinKeys = []string{"cp", "sp", "ep", "pp"}

// touchesMoney is a cheap pre-check, so the common field write costs no lookup.
// It also watches the other coin fields, so a change to silver alone is seen.
func touchesMoney(oldFields, newFields map[string]any) bool {
	for _, k := range append(append([]string{}, MoneyFieldKeys...), purseCoinKeys...) {
		o, n := oldFields[k], newFields[k]
		if fmt.Sprint(o) != fmt.Sprint(n) {
			return true
		}
	}
	return false
}

// RecordFieldChange logs a money change between oldFields and newFields on
// the entity, when it is a character with a money field. Writes that carry no
// source, or come from a stash move (which logs itself), are not logged.
func (h *MoneyHistory) RecordFieldChange(ctx context.Context, campaignID, entityID string, oldFields, newFields map[string]any) error {
	src, ok := changesource.From(ctx)
	if !ok || src.Kind == changesource.KindStash {
		return nil
	}
	phrase, ok := sourcePhrase(src)
	if !ok || !touchesMoney(oldFields, newFields) {
		return nil
	}
	if h.enabled != nil && !h.enabled(ctx, campaignID) {
		return nil
	}
	ref, err := h.directory.GetEntity(ctx, campaignID, entityID)
	if err != nil || ref == nil || !ref.IsCharacter || ref.MoneyKey == "" {
		return err
	}
	if len(ref.Purse) > 0 {
		return h.recordPurseChange(ctx, campaignID, ref, oldFields, newFields, src.UserID, phrase)
	}
	before, err := toCents(oldFields[ref.MoneyKey])
	if err != nil {
		return nil // Not a number before: nothing meaningful to compare.
	}
	after, err := toCents(newFields[ref.MoneyKey])
	if err != nil || before == after {
		return nil
	}

	name := ref.MoneyLabel
	if name == "" {
		name = ref.MoneyKey
	}
	return h.insert(ctx, campaignID, ref.ID, after-before,
		fmt.Sprintf("%s changed %s → %s · %s", name, before, after, phrase), src.UserID)
}

// recordPurseChange logs a change to a multi-coin purse as one line comparing
// the whole purse, with the amount as the copper value gained or lost (gp-cents
// are copper). A sheet that holds a coin as a non-number is skipped, as a
// non-numeric single money field is.
func (h *MoneyHistory) recordPurseChange(ctx context.Context, campaignID string, ref *EntityRef, oldFields, newFields map[string]any, userID, phrase string) error {
	before, _, err1 := coinCounts(ref.Purse, oldFields)
	after, _, err2 := coinCounts(ref.Purse, newFields)
	if err1 != nil || err2 != nil {
		return nil
	}
	bv, av := purseValueCp(before), purseValueCp(after)
	if bv == av {
		return nil
	}
	return h.insert(ctx, campaignID, ref.ID, Cents(av-bv),
		fmt.Sprintf("Purse %s → %s · %s", formatPurse(before), formatPurse(after), phrase), userID)
}

// insert writes the character-to-itself money row and announces it.
func (h *MoneyHistory) insert(ctx context.Context, campaignID, characterID string, amount Cents, reason, userID string) error {
	end := Endpoint{Kind: EndpointCharacter, ID: characterID}
	m := &Move{
		CampaignID:  campaignID,
		Kind:        MoveKindMoney,
		Amount:      amount,
		From:        end,
		To:          end,
		Status:      MoveApplied,
		Reason:      clipReason(reason),
		RequestedBy: userID,
	}
	if err := h.repo.InsertMove(ctx, m); err != nil {
		return err
	}
	if h.events != nil {
		h.events.PublishStashEvent(EventStashMoneyChanged, campaignID, strconv.FormatInt(m.ID, 10),
			map[string]any{"characterId": characterID, "moveId": m.ID})
	}
	return nil
}

// clipReason keeps a reason inside its column; a long label is the only thing
// that can overflow.
func clipReason(s string) string {
	if utf8.RuneCountInString(s) <= maxReasonLen {
		return s
	}
	r := []rune(s)
	return string(r[:maxReasonLen-1]) + "…"
}

// ShopPurchaseSource marks writes made under ctx as a shop purchase by userID,
// so a money change they cause reads "<label>" in the character's history. A
// shop calls this around the sheet write it makes.
func ShopPurchaseSource(ctx context.Context, userID, label string) context.Context {
	return changesource.With(ctx, changesource.Source{Kind: changesource.KindShop, Label: label, UserID: userID})
}
