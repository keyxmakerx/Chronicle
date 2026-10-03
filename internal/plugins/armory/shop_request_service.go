// shop_request_service.go holds a player's basket for the GM while downtime is
// closed, and applies it later through the same applyBasket path a live buy
// uses, so a request is never judged by looser rules than a purchase.
//
// A request stores listing ids and quantities only. Everything that decides
// the outcome (prices, currency, stock, the buyer's coins, who owns the
// character, whether the shop is still visible) is read again when the request
// is applied, under the campaign lock. The apply runs as the requester with
// player visibility: the approval lifts only the downtime gate, never the
// requester's other limits.
package armory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

const (
	// maxRequestReason is the width of the reason column.
	maxRequestReason = 255
	// internalFailReason is shown instead of a raw error when an apply fails
	// for a reason the player or GM cannot act on.
	internalFailReason = "Something went wrong, so nothing was bought. Ask for it again."
	// itemGoneReason stands for every 404 the apply path can give: a missing,
	// hidden or no-longer-sold listing.
	itemGoneReason = "An item is no longer for sale."
)

// queueRequest stores the basket for the GM. The caller holds the campaign
// lock and has authorised the buyer. The basket is priced once only to refuse
// a basket that could never be bought (unknown or unpriced listing, mixed
// currencies); the total is kept as the quote the request may not exceed, and coins are not checked because
// they are checked when the request is applied.
func (s *shopBuyService) queueRequest(ctx context.Context, campaignID, shopEntityID string, a Actor, buyer *EntityRef, in BuyInput) (*BuyResult, error) {
	if s.requests == nil {
		return nil, apperror.NewInternal(errors.New("purchase requests are not wired"))
	}
	_, currency, total, err := s.priceBasket(ctx, campaignID, shopEntityID, a, in)
	if err != nil {
		return nil, err
	}
	waiting, err := s.requests.CountPendingBy(ctx, campaignID, a.UserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if waiting >= maxPendingPurchasesPerUser {
		return nil, apperror.NewBadRequest(fmt.Sprintf("You have %d purchase requests waiting for the GM already.", maxPendingPurchasesPerUser))
	}
	// Only ids and quantities are copied: a price in the request body is
	// ignored by the type. The quote is the server's own total, never the client's.
	basket := make([]BuyItemInput, len(in.Items))
	for i, it := range in.Items {
		basket[i] = BuyItemInput{RelationID: it.RelationID, Quantity: it.Quantity}
	}
	req := &PurchaseRequest{
		CampaignID: campaignID, ShopEntityID: shopEntityID, BuyerEntityID: buyer.ID,
		RequestedBy: a.UserID, Basket: basket, QuotedTotal: total, QuotedCurrency: currency,
	}
	if err := s.requests.Insert(ctx, req); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return &BuyResult{Status: BuyStatusRequested}, nil
}

// applyStored applies a request's basket as the requester. The caller holds
// the campaign lock. Each check a live buy makes is made again, because the
// shop, the character or the claim may have changed while the request waited.
func (s *shopBuyService) applyStored(ctx context.Context, req *PurchaseRequest) error {
	// Player visibility regardless of what the requester was when they asked:
	// a role is not stored, and the safe reading is the narrowest one.
	a := Actor{UserID: req.RequestedBy, Role: permissions.RolePlayer}
	if err := requireViewableShop(ctx, s.shops, s.stash.Visibility, req.CampaignID, req.ShopEntityID, a.Role, a.UserID); err != nil {
		return stageError(err, "That shop is no longer available.")
	}
	buyer, err := s.stash.loadCharacter(ctx, req.CampaignID, req.BuyerEntityID)
	if err != nil {
		return stageError(err, "That character is no longer available.")
	}
	if err := s.authorizeBuyer(ctx, req.CampaignID, a, buyer.ID); err != nil {
		return stageError(err, "That character is no longer yours to buy for.")
	}
	if err := checkBuyerSheet(buyer); err != nil {
		return err
	}
	in := BuyInput{BuyerEntityID: buyer.ID, Items: req.Basket}
	if err := checkBuyInput(in); err != nil {
		return err
	}
	_, err = s.applyBasket(ctx, req.CampaignID, req.ShopEntityID, a, buyer, in, &basketQuote{total: req.QuotedTotal, currency: req.QuotedCurrency})
	return err
}

// stageError replaces a lookup refusal with a sentence a GM can read, keeping
// real faults (5xx) as they are so they are logged, not shown.
func stageError(err error, plain string) error {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code >= 400 && ae.Code < 500 {
		return apperror.NewConflict(plain)
	}
	return err
}

// failReason turns an apply error into the stored, player-readable reason.
func failReason(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code >= 400 && ae.Code < 500 {
		msg := ae.Message
		if ae.Code == 404 {
			msg = itemGoneReason
		}
		if utf8.RuneCountInString(msg) > maxRequestReason {
			msg = string([]rune(msg)[:maxRequestReason])
		}
		return msg
	}
	return internalFailReason
}

// settleApproved runs a pending request and records how it went. The caller
// holds the campaign lock.
//
// The row is marked applied BEFORE the basket runs, and rewritten to failed if
// the basket cannot go through. Coins cannot be rolled back together with a
// status write, so the order decides what a crash between them costs: this
// order can lose a purchase (the row says applied, nothing was charged, the
// player asks again) but can never charge twice, which the reverse order could
// when a later sweep found the row still pending.
func (s *shopBuyService) settleApproved(ctx context.Context, req *PurchaseRequest, decidedBy, okReason string) error {
	wrote, err := s.requests.Settle(ctx, req.CampaignID, req.ID, PurchasePending, PurchaseApplied, okReason, decidedBy)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !wrote {
		return apperror.NewConflict("That request has already been answered.")
	}
	req.Status, req.Reason, req.DecidedBy = PurchaseApplied, okReason, decidedBy

	applyErr := s.applyStored(ctx, req)
	if applyErr == nil {
		return nil
	}
	var ae *apperror.AppError
	if !errors.As(applyErr, &ae) || ae.Code >= 500 {
		slog.Error("purchase request: apply failed", slog.Int64("request_id", req.ID),
			slog.String("campaign_id", req.CampaignID), slog.Any("error", applyErr))
	}
	reason := failReason(applyErr)
	wrote, err = s.requests.Settle(ctx, req.CampaignID, req.ID, PurchaseApplied, PurchaseFailed, reason, decidedBy)
	if err != nil || !wrote {
		slog.Error("purchase request: FAILED BUT STILL MARKED APPLIED, nothing was charged",
			slog.Int64("request_id", req.ID), slog.String("campaign_id", req.CampaignID), slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("recording failed request %d: %v", req.ID, err))
	}
	req.Status, req.Reason = PurchaseFailed, reason
	return nil
}

// answerableRequest loads a request that is still waiting. The caller holds
// the campaign lock, so the status cannot change before it is answered.
func (s *shopBuyService) answerableRequest(ctx context.Context, campaignID string, a Actor, id int64) (*PurchaseRequest, error) {
	if !a.IsOwner() {
		return nil, forbidden()
	}
	if s.requests == nil {
		return nil, apperror.NewInternal(errors.New("purchase requests are not wired"))
	}
	req, err := s.requests.Get(ctx, campaignID, id)
	if err != nil {
		return nil, asAppError(err)
	}
	if req.Status != PurchasePending {
		return nil, apperror.NewConflict("That request has already been answered.")
	}
	return req, nil
}

func (s *shopBuyService) ApproveRequest(ctx context.Context, campaignID string, a Actor, requestID int64) (*PurchaseRequest, error) {
	if !a.IsOwner() {
		return nil, forbidden()
	}
	unlock := s.stash.locks.lock(campaignID)
	defer unlock()
	req, err := s.answerableRequest(ctx, campaignID, a, requestID)
	if err != nil {
		return nil, err
	}
	// The apply must survive the GM closing the tab mid-way.
	if err := s.settleApproved(context.WithoutCancel(ctx), req, a.UserID, ""); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *shopBuyService) DeclineRequest(ctx context.Context, campaignID string, a Actor, requestID int64) (*PurchaseRequest, error) {
	if !a.IsOwner() {
		return nil, forbidden()
	}
	unlock := s.stash.locks.lock(campaignID)
	defer unlock()
	req, err := s.answerableRequest(ctx, campaignID, a, requestID)
	if err != nil {
		return nil, err
	}
	wrote, err := s.requests.Settle(ctx, campaignID, req.ID, PurchasePending, PurchaseDeclined, "", a.UserID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if !wrote {
		return nil, apperror.NewConflict("That request has already been answered.")
	}
	req.Status, req.DecidedBy = PurchaseDeclined, a.UserID
	return req, nil
}

// sweepRequests applies every waiting request, oldest first, when downtime
// opens. The caller (SetDowntime) holds the campaign lock. One request that
// cannot go through is marked failed and the rest carry on.
func (s *shopBuyService) sweepRequests(ctx context.Context, campaignID, decidedBy string) (applied, failed int) {
	if s.requests == nil {
		return 0, 0
	}
	pending, err := s.requests.ListPending(ctx, campaignID)
	if err != nil {
		slog.Error("downtime sweep: listing purchase requests failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return 0, 0
	}
	for i := range pending {
		req := pending[i]
		if err := s.settleApproved(context.WithoutCancel(ctx), &req, decidedBy, autoAppliedReason); err != nil {
			slog.Error("downtime sweep: settling a purchase request failed", slog.Int64("request_id", req.ID), slog.Any("error", err))
			failed++
			continue
		}
		if req.Status == PurchaseApplied {
			applied++
		} else {
			failed++
		}
	}
	return applied, failed
}

// --- display ---

// shopLabel names the shop for a viewer: the real name when they may see it.
func (s *shopBuyService) shopLabel(ctx context.Context, campaignID string, a Actor, shopID string) string {
	if ok, err := s.stash.viewable(ctx, campaignID, a, shopID); err != nil || !ok {
		return "a shop"
	}
	ref, err := s.stash.Directory.GetEntity(ctx, campaignID, shopID)
	if err != nil || ref == nil {
		return "a shop"
	}
	return ref.Name
}

func (s *shopBuyService) userNames(ctx context.Context, campaignID string, reqs []PurchaseRequest) map[string]string {
	if s.stash.UserNames == nil || len(reqs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, r := range reqs {
		if !seen[r.RequestedBy] {
			seen[r.RequestedBy] = true
			ids = append(ids, r.RequestedBy)
		}
	}
	names, _ := s.stash.UserNames.DisplayNames(ctx, campaignID, ids)
	return names
}

func (s *shopBuyService) characterName(ctx context.Context, campaignID, id string) string {
	ref, err := s.stash.Directory.GetEntity(ctx, campaignID, id)
	if err != nil || ref == nil {
		return "a character that was removed"
	}
	return ref.Name
}

// pendingLines lists waiting requests for the Owner, each priced from the
// listings as they are now (the same read an approval would make). A basket
// that can no longer be priced shows without a figure rather than hiding.
func (s *shopBuyService) pendingLines(ctx context.Context, campaignID string, a Actor) ([]PurchaseRequestLine, error) {
	if !a.IsOwner() {
		return nil, forbidden()
	}
	if s.requests == nil {
		return nil, nil
	}
	pending, err := s.requests.ListPending(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	names := s.userNames(ctx, campaignID, pending)
	out := make([]PurchaseRequestLine, 0, len(pending))
	for _, r := range pending {
		l := PurchaseRequestLine{
			PurchaseRequest: r,
			ShopName:        s.shopLabel(ctx, campaignID, a, r.ShopEntityID),
			BuyerName:       s.characterName(ctx, campaignID, r.BuyerEntityID),
			RequesterName:   names[r.RequestedBy],
		}
		if l.RequesterName == "" {
			l.RequesterName = "A player"
		}
		in := BuyInput{BuyerEntityID: r.BuyerEntityID, Items: r.Basket}
		if _, cur, total, err := s.priceBasket(ctx, campaignID, r.ShopEntityID, a, in); err == nil {
			l.Total, l.Currency = &total, cur
			if cur != r.QuotedCurrency || total > r.QuotedTotal {
				q := r.QuotedTotal
				l.Total, l.Currency, l.PriceRose = &q, r.QuotedCurrency, true
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// buyerHistory shows a character's purchase requests among its history.
// Another player's waiting request is hidden from a non-GM, as with moves.
func (s *shopBuyService) buyerHistory(ctx context.Context, campaignID string, a Actor, buyerID string, limit int) ([]MoveLine, error) {
	if s.requests == nil {
		return nil, nil
	}
	reqs, err := s.requests.ListForBuyer(ctx, campaignID, buyerID, limit)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	names := s.userNames(ctx, campaignID, reqs)
	shops := map[string]string{}
	out := make([]MoveLine, 0, len(reqs))
	for _, r := range reqs {
		if r.Status == PurchasePending && r.RequestedBy != a.UserID && !a.IsGM() {
			continue
		}
		shop, ok := shops[r.ShopEntityID]
		if !ok {
			shop = s.shopLabel(ctx, campaignID, a, r.ShopEntityID)
			shops[r.ShopEntityID] = shop
		}
		who := names[r.RequestedBy]
		if who == "" {
			who = "A player"
		}
		out = append(out, MoveLine{
			Move: Move{
				ID: r.ID, CampaignID: r.CampaignID, Status: r.Status, Reason: r.Reason,
				RequestedBy: r.RequestedBy, DecidedBy: r.DecidedBy, CreatedAt: r.CreatedAt, DecidedAt: r.DecidedAt,
			},
			RequesterName: who,
			Summary:       purchaseSummary(r, who, shop),
		})
	}
	return out, nil
}

// purchaseSummary renders one request as a sentence in the wording of
// MoveSummary.
func purchaseSummary(r PurchaseRequest, who, shop string) string {
	what := fmt.Sprintf("%s at %s", plural(r.ItemCount(), "item"), shop)
	switch r.Status {
	case PurchasePending:
		return fmt.Sprintf("%s asked to buy %s. Waiting for the GM.", who, what)
	case PurchaseDeclined:
		return fmt.Sprintf("The GM turned down %s's request to buy %s.", who, what)
	case PurchaseFailed:
		return strings.TrimSpace(fmt.Sprintf("%s's purchase of %s didn't go through. %s", who, what, r.Reason))
	}
	switch {
	case r.Reason != "":
		return fmt.Sprintf("%s bought %s. %s", who, what, r.Reason)
	case r.DecidedBy != "" && r.DecidedBy != r.RequestedBy:
		return fmt.Sprintf("The GM approved %s's request to buy %s.", who, what)
	}
	return fmt.Sprintf("%s bought %s.", who, what)
}
