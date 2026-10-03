// shop_request_model.go holds the types for purchase requests: a basket a
// player asked for while downtime was closed, waiting for the GM.
package armory

import "time"

// A purchase request reuses the move statuses because the stashes page and the
// history lines show both kinds side by side.
const (
	PurchasePending  = MovePending
	PurchaseApplied  = MoveApplied
	PurchaseDeclined = MoveDeclined
	PurchaseFailed   = MoveFailed
)

// maxPendingPurchasesPerUser bounds how many baskets one player can leave
// waiting; each one is re-priced and re-checked when the GM answers.
const maxPendingPurchasesPerUser = 10

// PurchaseRequest is one stored basket. It holds listing ids and quantities
// only: prices, currency and stock are read from the listings when the request
// is applied, so a stale figure can never be charged.
type PurchaseRequest struct {
	ID            int64
	CampaignID    string
	ShopEntityID  string
	BuyerEntityID string
	RequestedBy   string
	Basket        []BuyItemInput
	// QuotedTotal and QuotedCurrency are the server-computed basket total at
	// request time. Apply refuses a request whose repriced total exceeds it.
	QuotedTotal    Cents
	QuotedCurrency string
	Status         string
	Reason         string
	DecidedBy      string
	CreatedAt      time.Time
	DecidedAt      *time.Time
}

// ItemCount is the total quantity across the basket.
func (r PurchaseRequest) ItemCount() int {
	n := 0
	for _, it := range r.Basket {
		n += it.Quantity
	}
	return n
}

// PurchaseRequestLine is a request with names resolved for display. Total is
// nil when the basket can no longer be priced from the listings (a listing was
// removed or edited to have no price); the row then simply omits the price.
type PurchaseRequestLine struct {
	PurchaseRequest
	ShopName      string
	BuyerName     string
	RequesterName string
	Total         *Cents
	Currency      string
	// PriceRose means today's prices add up to more than the player was
	// quoted, so approving will fail; the row shows the quote and says so.
	PriceRose bool
}
