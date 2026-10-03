// transaction_service.go contains business logic for shop transactions.
// Handles purchase validation, stock management, and currency deduction.
package armory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RelationMetadataUpdater updates relation metadata by ID.
// Implemented by the relations widget service — injected to avoid circular imports.
type RelationMetadataUpdater interface {
	UpdateMetadata(ctx context.Context, id int, metadata json.RawMessage) error
	// UpdateMetadataIf writes metadata only while the stored value still
	// equals expected, reporting whether it wrote.
	UpdateMetadataIf(ctx context.Context, id int, expected, metadata json.RawMessage) (bool, error)
}

// RelationFinder retrieves a relation by ID.
// Implemented by the relations widget service.
type RelationFinder interface {
	GetByID(ctx context.Context, id int) (*RelationInfo, error)
}

// RelationInfo is a minimal view of a relation, used by the transaction service.
// Avoids importing the full relations package.
type RelationInfo struct {
	ID             int
	Metadata       json.RawMessage
	CampaignID     string
	SourceEntityID string
	TargetEntityID string
}

// EntityFieldUpdater updates entity fields. Used to deduct currency from buyer.
// Implemented by entities.EntityService.
type EntityFieldUpdater interface {
	GetEntityFields(ctx context.Context, entityID string) (map[string]any, error)
	UpdateEntityFields(ctx context.Context, entityID string, fields map[string]any) error
}

// BuyerAccessChecker validates that the calling user can act on the buyer
// entity (i.e. the character making the purchase belongs to them, or is
// shared/visible per the campaign's per-entity ACL).
//
// Implemented by an adapter over entities.EntityService.CheckEntityAccess
// — wired in routes.go. Without this guard, Foundry — which authenticates
// with a single per-campaign API key — could spoof `buyer_entity_id` to
// purchase on behalf of any character in the campaign.
type BuyerAccessChecker interface {
	// CanUserActAsBuyer is false for an entity outside campaignID, whatever
	// the role: a campaign role grants nothing in another campaign.
	CanUserActAsBuyer(ctx context.Context, campaignID, entityID, userID string, role int) (bool, error)
}

// TransactionService defines the business logic contract for shop transactions.
type TransactionService interface {
	// Purchase executes a purchase: validates stock, creates transaction,
	// decrements stock, and optionally deducts buyer currency. The role is
	// the caller's campaign role — passed through to the buyer access check.
	Purchase(ctx context.Context, campaignID, userID string, role int, input CreateTransactionInput) (*PurchaseResult, error)

	// CreateTransaction records a transaction without stock/currency logic.
	// Used for manual logging (gifts, transfers, restocks).
	CreateTransaction(ctx context.Context, campaignID, userID string, input CreateTransactionInput) (*Transaction, error)

	// ListTransactions returns paginated transactions for a campaign.
	ListTransactions(ctx context.Context, campaignID string, opts TransactionListOptions) ([]Transaction, int, error)

	// ListShopTransactions returns transactions for a specific shop, scoped to
	// the caller's campaign so a foreign shop id leaks nothing (SEC-IDOR-3).
	ListShopTransactions(ctx context.Context, campaignID, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error)

	// ListBuyerTransactions returns transactions for a specific buyer.
	ListBuyerTransactions(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error)
}

// transactionService implements TransactionService.
type transactionService struct {
	repo            TransactionRepository
	metadataUpdater RelationMetadataUpdater
	relationFinder  RelationFinder
	entityFields    EntityFieldUpdater
	buyerAccess     BuyerAccessChecker
}

// NewTransactionService creates a new transaction service. Returns the concrete
// type so callers can inject optional dependencies via Set* methods.
func NewTransactionService(repo TransactionRepository) *transactionService {
	return &transactionService{repo: repo}
}

// SetRelationMetadataUpdater injects the relation metadata updater.
func (s *transactionService) SetRelationMetadataUpdater(u RelationMetadataUpdater) {
	s.metadataUpdater = u
}

// SetRelationFinder injects the relation finder.
func (s *transactionService) SetRelationFinder(f RelationFinder) {
	s.relationFinder = f
}

// SetEntityFieldUpdater injects the entity field updater.
func (s *transactionService) SetEntityFieldUpdater(u EntityFieldUpdater) {
	s.entityFields = u
}

// SetBuyerAccessChecker injects the buyer-access check used by Purchase.
// When unset (e.g. unit tests that don't exercise the buyer path), the
// check is skipped — same behavior as before this guard was added.
func (s *transactionService) SetBuyerAccessChecker(b BuyerAccessChecker) {
	s.buyerAccess = b
}

// Purchase validates stock, creates the transaction, decrements shop stock,
// and optionally deducts currency from the buyer entity.
func (s *transactionService) Purchase(ctx context.Context, campaignID, userID string, role int, input CreateTransactionInput) (*PurchaseResult, error) {
	if input.Quantity < 1 {
		return nil, apperror.NewBadRequest("quantity must be at least 1")
	}
	if input.ShopEntityID == "" || input.ItemEntityID == "" {
		return nil, apperror.NewBadRequest("shop and item entity IDs are required")
	}

	// Buyer cross-check: when a buyer entity is named, the calling user
	// must be able to act on it (own / shared / Owner / Scribe-via-grant).
	// Mitigates buyer_entity_id spoofing from clients that authenticate
	// with a single per-campaign identity (Foundry module's API key).
	if input.BuyerEntityID != "" && s.buyerAccess != nil {
		ok, err := s.buyerAccess.CanUserActAsBuyer(ctx, campaignID, input.BuyerEntityID, userID, role)
		if err != nil {
			return nil, fmt.Errorf("verify buyer access: %w", err)
		}
		if !ok {
			return nil, apperror.NewForbidden("you cannot purchase as that character")
		}
	}
	if input.TransactionType == "" {
		input.TransactionType = TxPurchase
	}

	// The shop listing is the source of truth for stock and price; a
	// purchase that names none can't be checked, so it is refused.
	stockRemaining := -1
	if s.relationFinder != nil && input.RelationID <= 0 {
		return nil, apperror.NewBadRequest("relation_id is required to buy from a shop")
	}
	if input.RelationID > 0 && s.relationFinder != nil {
		remaining, err := s.reserveStock(ctx, campaignID, &input)
		if err != nil {
			return nil, err
		}
		stockRemaining = remaining
	}

	// Create the transaction record.
	tx := &Transaction{
		CampaignID:      campaignID,
		ShopEntityID:    input.ShopEntityID,
		ItemEntityID:    input.ItemEntityID,
		BuyerEntityID:   strPtr(input.BuyerEntityID),
		RelationID:      intPtr(input.RelationID),
		Quantity:        input.Quantity,
		PricePaid:       strPtr(input.PricePaid),
		Currency:        input.Currency,
		PriceNumeric:    floatPtr(input.PriceNumeric),
		TransactionType: input.TransactionType,
		Notes:           strPtr(input.Notes),
		CreatedBy:       strPtr(userID),
	}
	if tx.Currency == "" {
		tx.Currency = "gp"
	}

	if err := s.repo.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}

	return &PurchaseResult{
		Transaction:    tx,
		StockRemaining: stockRemaining,
	}, nil
}

// CreateTransaction records a transaction without automated stock/currency changes.
func (s *transactionService) CreateTransaction(ctx context.Context, campaignID, userID string, input CreateTransactionInput) (*Transaction, error) {
	if input.TransactionType == "" {
		return nil, apperror.NewBadRequest("transaction type is required")
	}

	tx := &Transaction{
		CampaignID:      campaignID,
		ShopEntityID:    input.ShopEntityID,
		ItemEntityID:    input.ItemEntityID,
		BuyerEntityID:   strPtr(input.BuyerEntityID),
		RelationID:      intPtr(input.RelationID),
		Quantity:        input.Quantity,
		PricePaid:       strPtr(input.PricePaid),
		Currency:        input.Currency,
		PriceNumeric:    floatPtr(input.PriceNumeric),
		TransactionType: input.TransactionType,
		Notes:           strPtr(input.Notes),
		CreatedBy:       strPtr(userID),
	}
	if tx.Currency == "" {
		tx.Currency = "gp"
	}
	if tx.Quantity < 1 {
		tx.Quantity = 1
	}

	if err := s.repo.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("creating transaction: %w", err)
	}
	return tx, nil
}

// ListTransactions returns paginated transactions for a campaign.
func (s *transactionService) ListTransactions(ctx context.Context, campaignID string, opts TransactionListOptions) ([]Transaction, int, error) {
	return s.repo.ListByCampaign(ctx, campaignID, opts)
}

// ListShopTransactions returns transactions for a specific shop, scoped to
// campaignID (SEC-IDOR-3).
func (s *transactionService) ListShopTransactions(ctx context.Context, campaignID, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	return s.repo.ListByShop(ctx, campaignID, shopEntityID, opts)
}

// ListBuyerTransactions returns transactions for a specific buyer.
func (s *transactionService) ListBuyerTransactions(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	return s.repo.ListByBuyer(ctx, buyerEntityID, opts)
}

// stockAttempts bounds how often a purchase re-reads a listing that another
// purchase changed between its read and its write.
const stockAttempts = 5

// reserveStock checks the shop listing named by input, sets input's price
// fields from it, and takes the stock. The write only lands if the listing
// is unchanged since it was read, so two buyers can't both take the last
// unit. Returns the stock left, or -1 when the listing is unlimited.
func (s *transactionService) reserveStock(ctx context.Context, campaignID string, input *CreateTransactionInput) (int, error) {
	for attempt := 0; attempt < stockAttempts; attempt++ {
		rel, err := s.relationFinder.GetByID(ctx, input.RelationID)
		if err != nil {
			return 0, fmt.Errorf("finding shop relation: %w", err)
		}
		if rel.CampaignID != campaignID {
			return 0, apperror.NewNotFound("shop relation not found in this campaign")
		}
		if rel.SourceEntityID != input.ShopEntityID || rel.TargetEntityID != input.ItemEntityID {
			return 0, apperror.NewNotFound("shop relation not found for this shop and item")
		}

		meta := parseShopMeta(rel.Metadata)
		if !meta.InStock {
			return 0, apperror.NewBadRequest("this item is out of stock")
		}
		if meta.Quantity >= 0 && meta.Quantity < input.Quantity {
			return 0, apperror.NewBadRequest(
				fmt.Sprintf("insufficient stock: %d available, %d requested", meta.Quantity, input.Quantity),
			)
		}

		// Charge the listed price, never one the client sends.
		input.PriceNumeric = meta.Price * float64(input.Quantity)
		input.Currency = meta.Currency
		if input.Currency == "" {
			input.Currency = "gp"
		}
		input.PricePaid = strconv.FormatFloat(input.PriceNumeric, 'f', -1, 64) + " " + input.Currency

		if meta.Quantity < 0 || s.metadataUpdater == nil {
			return -1, nil
		}
		remaining := meta.Quantity - input.Quantity
		updated, err := meta.withQuantity(remaining)
		if err != nil {
			return 0, fmt.Errorf("encoding stock: %w", err)
		}
		wrote, err := s.metadataUpdater.UpdateMetadataIf(ctx, input.RelationID, rel.Metadata, updated)
		if err != nil {
			return 0, fmt.Errorf("updating stock: %w", err)
		}
		if wrote {
			return remaining, nil
		}
	}
	return 0, apperror.NewConflict("the shop is busy; try the purchase again")
}

// shopMeta is the stock state Purchase reads from a shop→item relation's
// metadata. The metadata object also carries fields Purchase doesn't own
// (price, custom names, anything an editor adds), so the stored object is
// kept whole and only "quantity" is ever rewritten.
type shopMeta struct {
	Quantity int     // -1 means unlimited.
	InStock  bool    // false when an editor marked the item out of stock.
	Price    float64 // unit price as listed; 0 when unset or unreadable.
	Currency string  // listed currency; empty when unset.
	fields   map[string]json.RawMessage
}

// parseShopMeta extracts stock state from relation JSON. An absent or null
// quantity is unlimited; 0 is sold out, never unlimited, so the last unit
// sold can't turn the item into endless stock. The legacy "unlimited": true
// flag still means unlimited.
func parseShopMeta(raw json.RawMessage) shopMeta {
	m := shopMeta{Quantity: -1, InStock: true, fields: map[string]json.RawMessage{}}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m.fields); err != nil || m.fields == nil {
			m.fields = map[string]json.RawMessage{}
		}
	}
	var unlimited bool
	if v, ok := m.fields["unlimited"]; ok {
		_ = json.Unmarshal(v, &unlimited)
	}
	if v, ok := m.fields["quantity"]; ok && !unlimited {
		var q *int
		if err := json.Unmarshal(v, &q); err == nil && q != nil && *q >= 0 {
			m.Quantity = *q
		}
	}
	if v, ok := m.fields["price"]; ok {
		// The shop widget stores a number; older listings may hold a string.
		var n float64
		var str string
		if err := json.Unmarshal(v, &n); err == nil {
			m.Price = n
		} else if err := json.Unmarshal(v, &str); err == nil {
			m.Price, _ = strconv.ParseFloat(strings.TrimSpace(str), 64)
		}
	}
	if v, ok := m.fields["currency"]; ok {
		_ = json.Unmarshal(v, &m.Currency)
	}
	if v, ok := m.fields["in_stock"]; ok {
		var in *bool
		if err := json.Unmarshal(v, &in); err == nil && in != nil {
			m.InStock = *in
		}
	}
	return m
}

// withQuantity returns the stored metadata object with only "quantity"
// replaced, so a purchase never drops the price or other fields.
func (m shopMeta) withQuantity(q int) (json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(m.fields)+1)
	for k, v := range m.fields {
		out[k] = v
	}
	qJSON, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	out["quantity"] = qJSON
	return json.Marshal(out)
}
