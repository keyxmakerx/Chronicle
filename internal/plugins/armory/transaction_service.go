// transaction_service.go contains business logic for shop transactions.
// Handles purchase validation, stock management, and currency deduction.
package armory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RelationMetadataUpdater updates relation metadata by ID.
// Implemented by the relations widget service — injected to avoid circular imports.
type RelationMetadataUpdater interface {
	UpdateMetadata(ctx context.Context, id int, metadata json.RawMessage) error
}

// RelationFinder retrieves a relation by ID.
// Implemented by the relations widget service.
type RelationFinder interface {
	GetByID(ctx context.Context, id int) (*RelationInfo, error)
}

// RelationInfo is a minimal view of a relation, used by the transaction service.
// Avoids importing the full relations package.
type RelationInfo struct {
	ID         int
	Metadata   json.RawMessage
	CampaignID string
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
	CanUserActAsBuyer(ctx context.Context, entityID, userID string, role int) (bool, error)
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

	// ListShopTransactions returns transactions for a specific shop.
	ListShopTransactions(ctx context.Context, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error)

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
		ok, err := s.buyerAccess.CanUserActAsBuyer(ctx, input.BuyerEntityID, userID, role)
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

	// Validate stock if a relation ID is provided (shop inventory relation).
	stockRemaining := -1
	if input.RelationID > 0 && s.relationFinder != nil {
		rel, err := s.relationFinder.GetByID(ctx, input.RelationID)
		if err != nil {
			return nil, fmt.Errorf("finding shop relation: %w", err)
		}
		if rel.CampaignID != campaignID {
			return nil, apperror.NewNotFound("shop relation not found in this campaign")
		}

		// Parse metadata to check stock.
		meta := parseShopMeta(rel.Metadata)
		if meta.Quantity >= 0 && meta.Quantity < input.Quantity {
			return nil, apperror.NewBadRequest(
				fmt.Sprintf("insufficient stock: %d available, %d requested", meta.Quantity, input.Quantity),
			)
		}

		// Decrement stock (unless unlimited: quantity = -1).
		if meta.Quantity >= 0 && s.metadataUpdater != nil {
			meta.Quantity -= input.Quantity
			stockRemaining = meta.Quantity
			updated, _ := json.Marshal(meta)
			if err := s.metadataUpdater.UpdateMetadata(ctx, input.RelationID, updated); err != nil {
				return nil, fmt.Errorf("updating stock: %w", err)
			}
		}
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

// ListShopTransactions returns transactions for a specific shop.
func (s *transactionService) ListShopTransactions(ctx context.Context, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	return s.repo.ListByShop(ctx, shopEntityID, opts)
}

// ListBuyerTransactions returns transactions for a specific buyer.
func (s *transactionService) ListBuyerTransactions(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	return s.repo.ListByBuyer(ctx, buyerEntityID, opts)
}

// shopMeta represents the metadata stored on shop→item relations.
type shopMeta struct {
	Price       string `json:"price,omitempty"`
	Quantity    int    `json:"quantity"`
	Currency    string `json:"currency,omitempty"`
	MaxStock    int    `json:"max_stock,omitempty"`
	Unlimited   bool   `json:"unlimited,omitempty"`
}

// parseShopMeta extracts shop metadata from relation JSON.
func parseShopMeta(raw json.RawMessage) shopMeta {
	var m shopMeta
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	// Default quantity to unlimited (-1) when not specified.
	if m.Quantity == 0 && !m.Unlimited {
		m.Quantity = -1
	}
	return m
}
