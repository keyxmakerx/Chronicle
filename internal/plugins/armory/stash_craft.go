// stash_craft.go is the Armory's side of crafting: what a character carries
// and how they pay, taking a project's materials and cost when it starts, and
// putting the made item in their pack. The crafting plugin reaches it through
// an adapter in internal/app and decides who may craft what; these methods
// still check that the actor may act for the character, as every move does.
package armory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
)

// maxCraftItems caps the distinct materials one project may take.
const maxCraftItems = 24

// CraftItem is one Armory item and how many. Pick names remembered copies
// to use first (copies.go); without it plain ones go first.
type CraftItem struct {
	ItemID   string
	Quantity int
	Pick     []CopyPick
}

// CraftPack is what a character carries and how they pay, as the actor may
// see it. PayKind is "" when the sheet has no money field, else BuyKindWealth,
// BuyKindPurse or BuyKindCoins; Have is the Wealth level, the coins, or for a
// purse its whole value in copper (gp-cents are copper), all in cents.
type CraftPack struct {
	Name       string
	Items      []HeldItem
	PayKind    string
	Have       Cents
	MoneyKey   string
	MoneyLabel string
}

// CraftCharge is what a cost of total in currency comes to against the pack:
// the Wealth a hero needs, the copper a purse pays, or the coins a single
// money field pays. Compare it with Have.
func CraftCharge(p *CraftPack, total Cents, currency string) Cents {
	switch p.PayKind {
	case BuyKindPurse:
		if cp, ok := toCp(total, currency); ok {
			return Cents(cp)
		}
		return total
	case BuyKindCoins:
		return chargeCents(&EntityRef{MoneyKey: p.MoneyKey}, total, currency)
	}
	return total
}

// CraftSpendInput is what starting a project takes from a character. Label
// names what is being made, for the history lines.
type CraftSpendInput struct {
	CharacterID string
	Items       []CraftItem
	Cost        Cents
	Currency    string
	Label       string
	// Purpose, when set, says what the items went to in the history in
	// place of "to make <Label>": "in an experiment".
	Purpose string
}

// CraftSpendOutcome says what was paid ("25 gp", "1.50 gp"); empty when no coins
// changed hands, as for a Wealth threshold. Used is every remembered copy
// that went into the project, so the roll can read their marks.
type CraftSpendOutcome struct {
	Paid string
	Used []Copy
}

// craftActor loads the character and refuses an actor who may not act for it.
func (s *stashService) craftActor(ctx context.Context, campaignID string, a Actor, characterID string) (*EntityRef, error) {
	char, err := s.loadCharacter(ctx, campaignID, strings.TrimSpace(characterID))
	if err != nil {
		return nil, err
	}
	ok, err := s.canAct(ctx, campaignID, a, char.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, forbidden()
	}
	return char, nil
}

// CraftPack reads what the character carries and how they pay.
func (s *stashService) CraftPack(ctx context.Context, campaignID string, a Actor, characterID string) (*CraftPack, error) {
	char, err := s.craftActor(ctx, campaignID, a, characterID)
	if err != nil {
		return nil, err
	}
	held, err := s.carried(ctx, campaignID, a, char.ID)
	if err != nil {
		return nil, err
	}
	p := &CraftPack{Name: char.Name, Items: held, PayKind: buyKind(char), MoneyKey: char.MoneyKey, MoneyLabel: char.MoneyLabel}
	switch p.PayKind {
	case BuyKindPurse:
		counts, err := s.purseCounts(ctx, char)
		if err != nil {
			return nil, err
		}
		p.Have = Cents(purseValueCp(counts))
	case BuyKindWealth, BuyKindCoins:
		if p.Have, err = s.characterMoney(ctx, char); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// mergeCraftItems adds up repeated items and refuses a quantity out of range.
func mergeCraftItems(items []CraftItem) ([]CraftItem, error) {
	var out []CraftItem
	at := map[string]int{}
	for _, it := range items {
		id := strings.TrimSpace(it.ItemID)
		if id == "" || it.Quantity < 1 || it.Quantity > maxMoveQuantity {
			return nil, apperror.NewBadRequest(giveQuantityMessage)
		}
		if i, ok := at[id]; ok {
			out[i].Quantity += it.Quantity
			out[i].Pick = append(out[i].Pick, it.Pick...)
			if out[i].Quantity > maxMoveQuantity {
				return nil, apperror.NewBadRequest(giveQuantityMessage)
			}
			continue
		}
		at[id] = len(out)
		out = append(out, CraftItem{ItemID: id, Quantity: it.Quantity, Pick: it.Pick})
	}
	if len(out) > maxCraftItems {
		return nil, apperror.NewBadRequest("That is too many different materials for one project.")
	}
	return out, nil
}

// CraftSpend takes a project's materials and cost from the character, all or
// nothing, under the campaign lock. A Wealth sheet needs the cost as a level
// and spends nothing, as at a shop. Each material is a history line on the
// character; a coin change is logged by the money history, labelled with what
// is being made.
func (s *stashService) CraftSpend(ctx context.Context, campaignID string, a Actor, in CraftSpendInput) (*CraftSpendOutcome, error) {
	items, err := mergeCraftItems(in.Items)
	if err != nil {
		return nil, err
	}
	if in.Cost < 0 {
		return nil, apperror.NewBadRequest("A cost is zero or more.")
	}
	char, err := s.craftActor(ctx, campaignID, a, in.CharacterID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(items))
	for _, it := range items {
		ref, err := s.loadItem(ctx, campaignID, it.ItemID)
		if err != nil {
			return nil, err
		}
		names[it.ItemID] = ref.Name
	}
	label := strings.TrimSpace(in.Label)
	purpose := "to make " + label
	if p := strings.TrimSpace(in.Purpose); p != "" {
		purpose = p
	}

	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()

	heldBefore := map[string]int{}
	for _, it := range items {
		rel, err := s.hasItem(ctx, campaignID, char.ID, it.ItemID, a.SeesDmOnly())
		if err != nil {
			return nil, err
		}
		held := 0
		if rel != nil {
			held, _ = parseCarried(rel.Metadata)
		}
		heldBefore[it.ItemID] = held
		if held < it.Quantity {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%s holds %d × %s, and this needs %d.", char.Name, held, names[it.ItemID], it.Quantity))
		}
	}

	kind := buyKind(char)
	var (
		charge Cents
		deltas map[string]int64
	)
	if in.Cost > 0 {
		switch kind {
		case "":
			return nil, apperror.NewBadRequest(noMoneyMessage + " The cost can't be paid.")
		case BuyKindWealth:
			wealth, err := s.characterMoney(ctx, char)
			if err != nil {
				return nil, err
			}
			if wealth < in.Cost {
				return nil, apperror.NewBadRequest(fmt.Sprintf(needsWealthFormat, in.Cost))
			}
		case BuyKindPurse:
			counts, err := s.purseCounts(ctx, char)
			if err != nil {
				return nil, err
			}
			var ok bool
			if deltas, _, ok = payFromPurse(counts, int64(chargeCents(char, in.Cost, in.Currency))); !ok {
				return nil, apperror.NewBadRequest(notEnoughCoinMessage)
			}
		default:
			have, err := s.characterMoney(ctx, char)
			if err != nil {
				return nil, err
			}
			if charge = chargeCents(char, in.Cost, in.Currency); have < charge {
				return nil, apperror.NewBadRequest(notEnoughCoinMessage)
			}
		}
	}

	// From here the spend completes even if the client goes away, and the
	// coin change reads as made for this project in the money history.
	mctx := changesource.With(context.WithoutCancel(ctx), changesource.Source{Kind: changesource.KindCraft, Label: purpose, UserID: a.UserID})
	var undo []func()
	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}
	// A line taken to zero stays until the spend is certain, so a rollback
	// gives it back with its notes and flags.
	for _, it := range items {
		ok, err := s.adjustCarried(mctx, campaignID, char.ID, it.ItemID, a.UserID, -it.Quantity, a.SeesDmOnly(), true)
		if err != nil || !ok {
			rollback()
			if err == nil {
				err = apperror.NewConflict("That character's items changed. Try again.")
			}
			return nil, err
		}
		it := it
		undo = append(undo, func() {
			if _, err := s.adjustCarried(mctx, campaignID, char.ID, it.ItemID, a.UserID, it.Quantity, a.SeesDmOnly(), false); err != nil {
				slog.Error("craft spend: RESTORE FAILED, character is short an item",
					slog.String("character_id", char.ID), slog.String("item_id", it.ItemID), slog.Any("error", err))
			}
		})
	}
	paid := ""
	switch {
	case in.Cost == 0 || kind == BuyKindWealth:
	case kind == BuyKindPurse:
		ok, err := s.adjustPurse(mctx, char, deltas)
		if err != nil || !ok {
			rollback()
			if err == nil {
				err = apperror.NewBadRequest(notEnoughCoinMessage)
			}
			return nil, err
		}
		paid = fmt.Sprintf("%s %s", in.Cost, strings.ToLower(strings.TrimSpace(in.Currency)))
	default:
		ok, err := s.adjustCharacterMoney(mctx, char, -charge)
		if err != nil || !ok {
			rollback()
			if err == nil {
				err = apperror.NewBadRequest(notEnoughCoinMessage)
			}
			return nil, err
		}
		unit := char.MoneyLabel
		if unit == "" {
			unit = char.MoneyKey
		}
		paid = fmt.Sprintf("%s %s", charge, strings.ToLower(unit))
	}
	var used []Copy
	for _, it := range items {
		used = append(used, s.takeCopies(mctx, campaignID, Endpoint{Kind: EndpointCharacter, ID: char.ID}, nil, it.ItemID, heldBefore[it.ItemID], it.Quantity, it.Pick)...)
		s.dropEmptyCarried(mctx, campaignID, char.ID, it.ItemID)
	}

	// The lines come last because they cannot be withdrawn; a failed line is
	// logged rather than failing a spend that has already happened.
	self := Endpoint{Kind: EndpointCharacter, ID: char.ID}
	for _, it := range items {
		m := &Move{
			CampaignID: campaignID, Kind: MoveKindItem, ItemEntityID: it.ItemID, Quantity: it.Quantity,
			From: self, To: self, Status: MoveApplied, RequestedBy: a.UserID, seesDmOnly: a.SeesDmOnly(),
			Reason: clipReason(fmt.Sprintf("Used %d × %s %s", it.Quantity, names[it.ItemID], purpose)),
		}
		if err := s.Repo.InsertMove(mctx, m); err != nil {
			slog.Error("craft spend: SPENT BUT NOT RECORDED in the move history",
				slog.String("campaign_id", campaignID), slog.String("character_id", char.ID),
				slog.String("item_id", it.ItemID), slog.Any("error", err))
			continue
		}
		ev.moved(m)
	}
	return &CraftSpendOutcome{Paid: paid, Used: used}, nil
}

// CraftGive puts what a finished project made into the character's pack,
// under the campaign lock, and writes it in their history. Like a give, a
// private item becomes visible to the character's player first. What the
// made copies remember (fine, marked, who made them, when they go off) is
// kept with them.
func (s *stashService) CraftGive(ctx context.Context, campaignID string, a Actor, characterID, itemID string, qty int, label string, m Memory) error {
	return s.craftAdd(ctx, campaignID, a, characterID, itemID, qty, m, func(item string) string {
		reason := fmt.Sprintf("Made %d × %s", qty, item)
		if label = strings.TrimSpace(label); label != "" && label != item {
			reason += " (" + label + ")"
		}
		return reason
	})
}

// SpoilGive puts what a hero took from the spoils tray into their pack, as
// CraftGive does, with what a harvest remembers (fine or poor parts). The
// history names only the item, never where it dropped, since the player may
// not be able to see that page.
func (s *stashService) SpoilGive(ctx context.Context, campaignID string, a Actor, characterID, itemID string, qty int, harvested bool, m Memory) error {
	return s.craftAdd(ctx, campaignID, a, characterID, itemID, qty, m, func(item string) string {
		if harvested {
			return fmt.Sprintf("Harvested %d × %s", qty, item)
		}
		return fmt.Sprintf("Took %d × %s from the spoils", qty, item)
	})
}

// craftAdd adds an item to a character's pack for crafting, with the
// history line reason gives for the item's name.
func (s *stashService) craftAdd(ctx context.Context, campaignID string, a Actor, characterID, itemID string, qty int, mem Memory, reason func(item string) string) error {
	if qty < 1 || qty > maxMoveQuantity {
		return apperror.NewBadRequest(giveQuantityMessage)
	}
	char, err := s.craftActor(ctx, campaignID, a, characterID)
	if err != nil {
		return err
	}
	return s.craftAddTo(ctx, campaignID, a, char, itemID, qty, mem, reason)
}

// CraftGiveFor puts an item in a character's pack on behalf of a hero the
// actor may act for, with the history line given: what a helper's roll
// finished goes to the project's owner, and what breaking an item down gives
// back goes to the hero who broke it. Adding to a pack takes nothing from
// anyone, so acting for the hero is enough; crafting has already checked the
// hero may help with that project.
func (s *stashService) CraftGiveFor(ctx context.Context, campaignID string, a Actor, actingFor, characterID, itemID string, qty int, why string, m Memory) error {
	if qty < 1 || qty > maxMoveQuantity {
		return apperror.NewBadRequest(giveQuantityMessage)
	}
	if _, err := s.craftActor(ctx, campaignID, a, actingFor); err != nil {
		return err
	}
	char, err := s.loadCharacter(ctx, campaignID, strings.TrimSpace(characterID))
	if err != nil {
		return err
	}
	return s.craftAddTo(ctx, campaignID, a, char, itemID, qty, m, func(string) string { return why })
}

// craftAddTo adds the item to a character already checked.
func (s *stashService) craftAddTo(ctx context.Context, campaignID string, a Actor, char *EntityRef, itemID string, qty int, mem Memory, reason func(item string) string) error {
	item, err := s.loadItem(ctx, campaignID, itemID)
	if err != nil {
		return err
	}
	var ev eventBatch
	defer ev.flush(s.Events, campaignID)
	unlock := s.locks.lock(campaignID)
	defer unlock()

	mctx := context.WithoutCancel(ctx)
	if s.Handouts != nil && char.OwnerUserID != "" {
		if _, err := s.Handouts.AllowViewers(mctx, campaignID, item.ID, []string{char.OwnerUserID}); err != nil {
			return err
		}
	}
	ok, err := s.adjustCarried(mctx, campaignID, char.ID, item.ID, a.UserID, qty, true, false)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewConflict("Could not add it to that character.")
	}
	self := Endpoint{Kind: EndpointCharacter, ID: char.ID}
	s.remember(mctx, campaignID, self, item.ID, qty, mem)
	m := &Move{
		CampaignID: campaignID, Kind: MoveKindItem, ItemEntityID: item.ID, Quantity: qty,
		From: self, To: self, Status: MoveApplied, RequestedBy: a.UserID, seesDmOnly: true, Reason: clipReason(reason(item.Name)),
	}
	if err := s.Repo.InsertMove(mctx, m); err != nil {
		slog.Error("craft give: ADDED BUT NOT RECORDED in the move history",
			slog.String("campaign_id", campaignID), slog.String("character_id", char.ID),
			slog.String("item_id", item.ID), slog.Any("error", err))
		return nil
	}
	ev.moved(m)
	return nil
}
