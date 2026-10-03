// shop_buy_model.go holds the wire and domain types for buying from a shop's
// wares with a character's coins.
package armory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Limits on one basket. They bound the work done under the campaign lock and
// stop a typo from becoming a huge charge.
const (
	maxBuyItems    = 50
	maxBuyQuantity = 99
	// maxUnitPriceCents bounds a listed price (10,000,000 coins) so no
	// quantity times price can overflow or be a sane-looking surprise.
	maxUnitPriceCents = Cents(1_000_000_000)
)

// Buy statuses reported to the client.
const (
	BuyStatusBought    = "bought"
	BuyStatusRequested = "requested"
)

// RelationRef is a shop relation id as the client sends it. The widget may
// send the id as a JSON number or as a numeral string (relation ids are
// integers server-side but travel as either in the widget's data).
type RelationRef int

// UnmarshalJSON accepts a whole number or a string holding one.
func (r *RelationRef) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	var n int
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("relationId must be a whole number")
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("relationId must be a whole number")
		}
		n = v
	} else if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("relationId must be a whole number")
	}
	*r = RelationRef(n)
	return nil
}

// BuyItemInput is one basket line. It carries no price: the price always comes
// from the listing.
type BuyItemInput struct {
	RelationID RelationRef `json:"relationId"`
	Quantity   int         `json:"quantity"`
}

// BuyInput is the buy request body.
type BuyInput struct {
	BuyerEntityID string         `json:"buyerEntityId"`
	Items         []BuyItemInput `json:"items"`
}

// BuyResult is the buy response. A bought basket reports what was charged; a
// request reports only its status.
type BuyResult struct {
	Status    string   `json:"status"`
	Spent     *float64 `json:"spent,omitempty"`
	Currency  string   `json:"currency,omitempty"`
	MoneyLeft *float64 `json:"moneyLeft,omitempty"`
}

// Buyer is one character the caller may buy for. Money is null when the sheet
// has no money field (or holds a non-number).
type Buyer struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	MoneyKey string   `json:"moneyKey"`
	Money    *float64 `json:"money"`
}

// BuyersView is the response of the buyers endpoint.
type BuyersView struct {
	DowntimeOpen bool    `json:"downtimeOpen"`
	CanBuyNow    bool    `json:"canBuyNow"`
	Buyers       []Buyer `json:"buyers"`
}
