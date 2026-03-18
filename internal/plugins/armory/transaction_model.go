// transaction_model.go defines data structures for shop transactions.
// Transactions record purchases, sales, transfers, and gifts between entities.
package armory

import "time"

// TransactionType enumerates the kinds of shop transactions.
const (
	TxPurchase = "purchase" // Character buys from shop.
	TxSale     = "sale"     // Character sells to shop.
	TxTransfer = "transfer" // Item moves between characters.
	TxGift     = "gift"     // Free item transfer.
	TxRestock  = "restock"  // Shop restocking (no buyer).
)

// Transaction represents a single shop transaction record.
type Transaction struct {
	ID             int        `json:"id"`
	CampaignID     string     `json:"campaign_id"`
	InstanceID     *int       `json:"instance_id,omitempty"`
	ShopEntityID   string     `json:"shop_entity_id"`
	ItemEntityID   string     `json:"item_entity_id"`
	BuyerEntityID  *string    `json:"buyer_entity_id,omitempty"`
	RelationID     *int       `json:"relation_id,omitempty"`
	Quantity       int        `json:"quantity"`
	PricePaid      *string    `json:"price_paid,omitempty"`
	Currency       string     `json:"currency"`
	PriceNumeric   *float64   `json:"price_numeric,omitempty"`
	TransactionType string   `json:"transaction_type"`
	Notes          *string    `json:"notes,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CreatedBy      *string    `json:"created_by,omitempty"`

	// Joined display fields (populated by list queries).
	ShopName  string `json:"shop_name,omitempty"`
	ItemName  string `json:"item_name,omitempty"`
	BuyerName string `json:"buyer_name,omitempty"`
}

// CreateTransactionInput is the request payload for creating a transaction.
type CreateTransactionInput struct {
	InstanceID      int      `json:"instance_id"`
	ShopEntityID    string   `json:"shop_entity_id"`
	ItemEntityID    string   `json:"item_entity_id"`
	BuyerEntityID   string   `json:"buyer_entity_id"`
	RelationID      int      `json:"relation_id"`
	Quantity        int      `json:"quantity"`
	PricePaid       string   `json:"price_paid"`
	Currency        string   `json:"currency"`
	PriceNumeric    float64  `json:"price_numeric"`
	TransactionType string   `json:"transaction_type"`
	Notes           string   `json:"notes"`
}

// TransactionListOptions controls filtering and pagination for transaction queries.
type TransactionListOptions struct {
	Page            int    // 1-indexed page number.
	PerPage         int    // Items per page (default 20).
	ShopEntityID    string // Filter by shop entity.
	BuyerEntityID   string // Filter by buyer entity.
	ItemEntityID    string // Filter by item entity.
	TransactionType string // Filter by type (purchase, sale, etc.).
}

// DefaultTransactionListOptions returns sensible defaults.
func DefaultTransactionListOptions() TransactionListOptions {
	return TransactionListOptions{Page: 1, PerPage: 20}
}

// Offset returns the SQL offset for the current page.
func (o TransactionListOptions) Offset() int {
	if o.Page < 1 {
		return 0
	}
	return (o.Page - 1) * o.PerPage
}

// PurchaseResult is returned after a successful purchase to inform
// the caller of the outcome.
type PurchaseResult struct {
	Transaction   *Transaction `json:"transaction"`
	StockRemaining int         `json:"stock_remaining"` // -1 means unlimited.
}
