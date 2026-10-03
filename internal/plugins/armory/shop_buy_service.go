// shop_buy_service.go lets a player spend a character's coins on a shop's
// wares. One basket is applied as a unit under the same per-campaign lock the
// stash moves use, so a purchase and a move can never both spend the same
// coins. Stock, coins and items change in a fixed order and every step that
// already happened is undone if a later one fails, so a basket is never
// applied in part.
//
// Prices and currency come only from the shop's listing (the relation
// metadata); the client names goods and quantities, never amounts.
package armory

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
)

const (
	noCoinFieldMessage   = "This sheet has no coin field"
	notEnoughCoinMessage = "Not enough coin"
	mixedCurrencyMessage = "Items are priced in different currencies"
	// buyClosedMessage is shown to a player while downtime is closed. Only the
	// Owner may buy then.
	buyClosedMessage = "Buying opens when the GM opens downtime."
)

// ShopBuyService reads who can buy at a shop and applies a basket.
type ShopBuyService interface {
	// Buyers lists the characters the caller may buy for and whether a buy
	// would apply at once. A shop the caller cannot see is NotFound.
	Buyers(ctx context.Context, campaignID, shopEntityID string, a Actor) (*BuyersView, error)
	// Buy charges the buyer's coins, takes the stock, gives the items and
	// records the purchases.
	Buy(ctx context.Context, campaignID, shopEntityID string, a Actor, in BuyInput) (*BuyResult, error)
}

type shopBuyService struct {
	stash *stashService
	tx    *transactionService
	shops ShopEntityChecker
}

// NewShopBuyService wires the buy flow. It takes the concrete stash and
// transaction services because it must share the stash service's campaign
// lock and its money and inventory helpers; a second copy of those would let
// the two paths drift apart.
func NewShopBuyService(stash *stashService, tx *transactionService, shops ShopEntityChecker) ShopBuyService {
	return &shopBuyService{stash: stash, tx: tx, shops: shops}
}

// buyLine is one priced basket line.
type buyLine struct {
	relationID int
	itemID     string
	quantity   int
	unit       Cents
	input      CreateTransactionInput
}

func (s *shopBuyService) Buyers(ctx context.Context, campaignID, shopEntityID string, a Actor) (*BuyersView, error) {
	if err := requireViewableShop(ctx, s.shops, s.stash.Visibility, campaignID, shopEntityID, a.Role, a.UserID); err != nil {
		return nil, err
	}
	open, err := s.stash.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	refs, err := s.candidates(ctx, campaignID, a)
	if err != nil {
		return nil, err
	}
	view := &BuyersView{DowntimeOpen: open, CanBuyNow: open || a.IsOwner(), Buyers: []Buyer{}}
	for i := range refs {
		b := Buyer{ID: refs[i].ID, Name: refs[i].Name, MoneyKey: refs[i].MoneyKey}
		if refs[i].MoneyKey != "" {
			// A sheet whose coin field isn't a number reads as "no figure"
			// rather than failing the whole list.
			if c, err := s.stash.characterMoney(ctx, &refs[i]); err == nil {
				v := centsToFloat(c)
				b.Money = &v
			}
		}
		view.Buyers = append(view.Buyers, b)
	}
	return view, nil
}

// candidates are the characters the caller may buy for: the Owner every
// character, anyone else only the characters they have claimed.
func (s *shopBuyService) candidates(ctx context.Context, campaignID string, a Actor) ([]EntityRef, error) {
	var out []EntityRef
	if a.IsOwner() {
		all, err := s.stash.Directory.ListCharacters(ctx, campaignID, a.Role, a.UserID)
		if err != nil {
			return nil, err
		}
		out = all
	} else {
		owned, err := s.stash.Directory.OwnedCharacterIDs(ctx, campaignID, a.UserID)
		if err != nil {
			return nil, err
		}
		for id := range owned {
			ref, err := s.stash.Directory.GetEntity(ctx, campaignID, id)
			if err != nil {
				return nil, err
			}
			if ref != nil && ref.IsCharacter {
				out = append(out, *ref)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ni, nj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if ni != nj {
			return ni < nj
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// authorizeBuyer lets the Owner buy for any character and everyone else only
// for a character they have claimed and may act as. A Scribe's wider edit
// rights deliberately do not extend to spending another player's coins.
func (s *shopBuyService) authorizeBuyer(ctx context.Context, campaignID string, a Actor, characterID string) error {
	if a.IsOwner() {
		return nil
	}
	deny := apperror.NewForbidden("You can only buy for your own character.")
	owned, err := s.stash.Directory.OwnedCharacterIDs(ctx, campaignID, a.UserID)
	if err != nil {
		return err
	}
	if !owned[characterID] || s.stash.Actor == nil {
		return deny
	}
	ok, err := s.stash.Actor.CanUserActAsBuyer(ctx, campaignID, characterID, a.UserID, a.Role)
	if err != nil {
		return err
	}
	if !ok {
		return deny
	}
	return nil
}

// checkBuyInput validates the request shape before any lookup.
func checkBuyInput(in BuyInput) error {
	if strings.TrimSpace(in.BuyerEntityID) == "" {
		return apperror.NewBadRequest("Choose who is buying.")
	}
	if len(in.Items) == 0 {
		return apperror.NewBadRequest("Your basket is empty.")
	}
	if len(in.Items) > maxBuyItems {
		return apperror.NewBadRequest("That is too many items in one purchase.")
	}
	seen := make(map[RelationRef]bool, len(in.Items))
	for _, it := range in.Items {
		if it.RelationID <= 0 {
			return apperror.NewBadRequest("An item in your basket is not valid.")
		}
		if it.Quantity < 1 || it.Quantity > maxBuyQuantity {
			return apperror.NewBadRequest("Choose a quantity between 1 and 99.")
		}
		if seen[it.RelationID] {
			return apperror.NewBadRequest("Each item can only appear once in a purchase.")
		}
		seen[it.RelationID] = true
	}
	return nil
}

func (s *shopBuyService) Buy(ctx context.Context, campaignID, shopEntityID string, a Actor, in BuyInput) (*BuyResult, error) {
	if err := checkBuyInput(in); err != nil {
		return nil, err
	}
	if err := requireViewableShop(ctx, s.shops, s.stash.Visibility, campaignID, shopEntityID, a.Role, a.UserID); err != nil {
		return nil, err
	}
	buyer, err := s.stash.loadCharacter(ctx, campaignID, strings.TrimSpace(in.BuyerEntityID))
	if err != nil {
		return nil, err
	}
	if err := s.authorizeBuyer(ctx, campaignID, a, buyer.ID); err != nil {
		return nil, err
	}
	if buyer.MoneyKey == "" {
		return nil, apperror.NewBadRequest(noCoinFieldMessage)
	}

	unlock := s.stash.locks.lock(campaignID)
	defer unlock()

	// The switch is read under the lock, like a stash move, so a basket cannot
	// be judged against a downtime state that has since changed.
	open, err := s.stash.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if !open && !a.IsOwner() {
		// A waiting purchase would need somewhere to be stored; the request
		// queue (item_moves) can only hold an item or money move between
		// characters and stashes, so for now the player is told to wait.
		return nil, apperror.NewConflict(buyClosedMessage)
	}

	lines, currency, total, err := s.priceBasket(ctx, campaignID, shopEntityID, a, in)
	if err != nil {
		return nil, err
	}
	// Read the coins under the lock so two baskets can't both spend them.
	have, err := s.stash.characterMoney(ctx, buyer)
	if err != nil {
		return nil, err
	}
	if have < total {
		return nil, apperror.NewBadRequest(notEnoughCoinMessage)
	}

	// From here on the basket is applied even if the client disconnects: a
	// cancelled request must not strand a half-applied purchase.
	mctx := context.WithoutCancel(ctx)
	// The coin change reads "at a shop" in the character's money history,
	// unless a caller (the Foundry route) already labelled it as a purchase.
	if src, ok := changesource.From(mctx); !ok || src.Kind != changesource.KindShop {
		mctx = ShopPurchaseSource(mctx, a.UserID, "")
	}
	var undo []func()
	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}

	for i := range lines {
		l := &lines[i]
		l.input = CreateTransactionInput{
			ShopEntityID:    shopEntityID,
			ItemEntityID:    l.itemID,
			BuyerEntityID:   buyer.ID,
			RelationID:      l.relationID,
			Quantity:        l.quantity,
			TransactionType: TxPurchase,
		}
		remaining, err := s.tx.reserveStock(mctx, campaignID, &l.input, true)
		if err != nil {
			rollback()
			return nil, asAppError(err)
		}
		if remaining >= 0 {
			rid, qty := l.relationID, l.quantity
			undo = append(undo, func() {
				if err := s.tx.releaseStock(mctx, rid, qty); err != nil {
					slog.Error("shop buy: could not give stock back", slog.Int("relation_id", rid),
						slog.Int("quantity", qty), slog.String("campaign_id", campaignID), slog.Any("error", err))
				}
			})
		}
		// The listing may have been edited between pricing and reserving;
		// never charge a figure other than the one the coins were checked for.
		// Compare unit prices: a sub-cent price times a quantity rounds differently
		// from the rounded unit price times that quantity.
		if Cents(math.Round(l.input.PriceNumeric/float64(l.quantity)*100)) != l.unit || normCurrency(l.input.Currency) != currency {
			rollback()
			return nil, apperror.NewConflict("The shop changed its prices. Try again.")
		}
	}

	ok, err := s.stash.adjustCharacterMoney(mctx, buyer, -total)
	if err != nil {
		rollback()
		return nil, asAppError(err)
	}
	if !ok {
		rollback()
		return nil, apperror.NewBadRequest(notEnoughCoinMessage)
	}
	undo = append(undo, func() {
		if _, err := s.stash.adjustCharacterMoney(mctx, buyer, total); err != nil {
			slog.Error("shop buy: RESTORE FAILED, character is short",
				slog.String("character_id", buyer.ID), slog.String("campaign_id", campaignID), slog.Any("error", err))
		}
	})

	for i := range lines {
		l := lines[i]
		ok, err := s.stash.adjustCarried(mctx, campaignID, buyer.ID, l.itemID, a.UserID, l.quantity, a.IsGM(), false)
		if err != nil || !ok {
			rollback()
			if err == nil {
				err = apperror.NewConflict("Could not add the item to that character. Try again.")
			}
			return nil, asAppError(err)
		}
		itemID, qty := l.itemID, l.quantity
		undo = append(undo, func() {
			if _, err := s.stash.adjustCarried(mctx, campaignID, buyer.ID, itemID, a.UserID, -qty, a.IsGM(), false); err != nil {
				slog.Error("shop buy: could not take a credited item back",
					slog.String("character_id", buyer.ID), slog.String("item_id", itemID), slog.Any("error", err))
			}
		})
	}

	// The log comes last because rows cannot be withdrawn. By now the goods
	// and the charge are done, so a failed row is reported loudly instead of
	// turning a completed purchase into an error the player would retry.
	for i := range lines {
		if err := s.tx.recordPurchase(mctx, campaignID, a.UserID, lines[i].input); err != nil {
			slog.Error("shop buy: APPLIED BUT NOT RECORDED",
				slog.String("campaign_id", campaignID), slog.String("shop_id", shopEntityID),
				slog.String("buyer_id", buyer.ID), slog.Int("relation_id", lines[i].relationID), slog.Any("error", err))
		}
	}

	spent, left := centsToFloat(total), centsToFloat(have-total)
	return &BuyResult{Status: BuyStatusBought, Spent: &spent, Currency: currency, MoneyLeft: &left}, nil
}

// priceBasket validates every line against the shop's listings and totals
// them. It changes nothing. A relation that is not one of this shop's
// "sells" listings in this campaign, a hidden listing, or a good the caller
// cannot see all read as not found.
func (s *shopBuyService) priceBasket(ctx context.Context, campaignID, shopEntityID string, a Actor, in BuyInput) ([]buyLine, string, Cents, error) {
	if s.tx.relationFinder == nil {
		return nil, "", 0, apperror.NewInternal(errors.New("shop relations are not wired"))
	}
	lines := make([]buyLine, 0, len(in.Items))
	itemIDs := make([]string, 0, len(in.Items))
	var total Cents
	currency := ""
	for _, it := range in.Items {
		rel, err := s.tx.relationFinder.GetByID(ctx, int(it.RelationID))
		if err != nil {
			var ae *apperror.AppError
			if errors.As(err, &ae) && ae.Code == 404 {
				return nil, "", 0, notFound("shop item")
			}
			return nil, "", 0, apperror.NewInternal(err)
		}
		if rel == nil || rel.CampaignID != campaignID || rel.SourceEntityID != shopEntityID ||
			rel.RelationType != shopSellsRelation || (rel.DmOnly && !a.IsGM()) {
			return nil, "", 0, notFound("shop item")
		}
		meta := parseShopMeta(rel.Metadata)
		unit, err := unitPriceCents(meta.Price)
		if err != nil {
			return nil, "", 0, err
		}
		cur := normCurrency(meta.Currency)
		if currency == "" {
			currency = cur
		} else if cur != currency {
			return nil, "", 0, apperror.NewBadRequest(mixedCurrencyMessage)
		}
		total += unit * Cents(it.Quantity)
		itemIDs = append(itemIDs, rel.TargetEntityID)
		lines = append(lines, buyLine{relationID: int(it.RelationID), itemID: rel.TargetEntityID, quantity: it.Quantity, unit: unit})
	}
	// Owner visibility sees every entity, so only others are filtered.
	if !a.IsOwner() {
		if s.stash.Visibility == nil {
			return nil, "", 0, notFound("shop item")
		}
		viewable, err := s.stash.Visibility.FilterViewableEntityIDs(ctx, campaignID, itemIDs, a.Role, a.UserID)
		if err != nil {
			return nil, "", 0, apperror.NewInternal(err)
		}
		for _, id := range itemIDs {
			if !viewable[id] {
				return nil, "", 0, notFound("shop item")
			}
		}
	}
	return lines, currency, total, nil
}

// unitPriceCents turns a listed price into cents. A listing with no usable
// price is refused rather than sold for nothing: an unpriced good would
// otherwise be free to any player.
func unitPriceCents(price float64) (Cents, error) {
	if math.IsNaN(price) || math.IsInf(price, 0) || price <= 0 {
		return 0, apperror.NewBadRequest("An item in your basket has no price.")
	}
	c := Cents(math.Round(price * 100))
	if c <= 0 || c > maxUnitPriceCents {
		return 0, apperror.NewBadRequest("An item in your basket has no valid price.")
	}
	return c, nil
}

// normCurrency matches reserveStock's default and ignores case and spacing,
// so "GP" and "gp " price the same coin.
func normCurrency(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" {
		return "gp"
	}
	return c
}

func centsToFloat(c Cents) float64 { return float64(c) / 100 }

// asAppError keeps a domain error as it is and hides any other error behind a
// generic internal one, so a raw database error never reaches the client.
func asAppError(err error) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae
	}
	return apperror.NewInternal(err)
}
