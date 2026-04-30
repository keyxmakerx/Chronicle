package armory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// --- Mocks ---

type mockTransactionRepo struct {
	createFn        func(ctx context.Context, tx *Transaction) error
	listByCampaignFn func(ctx context.Context, campaignID string, opts TransactionListOptions) ([]Transaction, int, error)
	listByShopFn    func(ctx context.Context, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error)
	listByBuyerFn   func(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error)
}

func (m *mockTransactionRepo) Create(ctx context.Context, tx *Transaction) error {
	if m.createFn != nil {
		return m.createFn(ctx, tx)
	}
	tx.ID = 1
	return nil
}

func (m *mockTransactionRepo) ListByCampaign(ctx context.Context, campaignID string, opts TransactionListOptions) ([]Transaction, int, error) {
	if m.listByCampaignFn != nil {
		return m.listByCampaignFn(ctx, campaignID, opts)
	}
	return nil, 0, nil
}

func (m *mockTransactionRepo) ListByShop(ctx context.Context, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	if m.listByShopFn != nil {
		return m.listByShopFn(ctx, shopEntityID, opts)
	}
	return nil, 0, nil
}

func (m *mockTransactionRepo) ListByBuyer(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	if m.listByBuyerFn != nil {
		return m.listByBuyerFn(ctx, buyerEntityID, opts)
	}
	return nil, 0, nil
}

type mockRelationFinder struct {
	getByIDFn func(ctx context.Context, id int) (*RelationInfo, error)
}

func (m *mockRelationFinder) GetByID(ctx context.Context, id int) (*RelationInfo, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

type mockMetadataUpdater struct {
	updateFn func(ctx context.Context, id int, metadata json.RawMessage) error
}

func (m *mockMetadataUpdater) UpdateMetadata(ctx context.Context, id int, metadata json.RawMessage) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, metadata)
	}
	return nil
}

type mockBuyerAccess struct {
	canFn func(ctx context.Context, entityID, userID string, role int) (bool, error)
}

func (m *mockBuyerAccess) CanUserActAsBuyer(ctx context.Context, entityID, userID string, role int) (bool, error) {
	if m.canFn != nil {
		return m.canFn(ctx, entityID, userID, role)
	}
	return true, nil
}

// --- Tests ---

func TestPurchase_Success(t *testing.T) {
	repo := &mockTransactionRepo{}
	svc := NewTransactionService(repo)
	result, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID: "shop-1",
		ItemEntityID: "item-1",
		Quantity:     1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Transaction.Currency != "gp" {
		t.Errorf("currency = %q, want %q", result.Transaction.Currency, "gp")
	}
	if result.Transaction.TransactionType != TxPurchase {
		t.Errorf("type = %q, want %q", result.Transaction.TransactionType, TxPurchase)
	}
}

func TestPurchase_ZeroQuantity(t *testing.T) {
	svc := NewTransactionService(&mockTransactionRepo{})
	_, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID: "shop-1",
		ItemEntityID: "item-1",
		Quantity:     0,
	})
	if err == nil {
		t.Error("expected error for zero quantity")
	}
}

func TestPurchase_MissingEntityIDs(t *testing.T) {
	svc := NewTransactionService(&mockTransactionRepo{})
	_, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		Quantity: 1,
	})
	if err == nil {
		t.Error("expected error for missing entity IDs")
	}
}

func TestPurchase_InsufficientStock(t *testing.T) {
	svc := NewTransactionService(&mockTransactionRepo{})
	svc.SetRelationFinder(&mockRelationFinder{
		getByIDFn: func(_ context.Context, _ int) (*RelationInfo, error) {
			meta, _ := json.Marshal(shopMeta{Quantity: 2})
			return &RelationInfo{ID: 1, CampaignID: "camp-1", Metadata: meta}, nil
		},
	})
	_, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID: "shop-1",
		ItemEntityID: "item-1",
		RelationID:   1,
		Quantity:     5,
	})
	if err == nil {
		t.Error("expected insufficient stock error")
	}
}

func TestPurchase_StockDecrement(t *testing.T) {
	var updatedMeta json.RawMessage
	svc := NewTransactionService(&mockTransactionRepo{})
	svc.SetRelationFinder(&mockRelationFinder{
		getByIDFn: func(_ context.Context, _ int) (*RelationInfo, error) {
			meta, _ := json.Marshal(shopMeta{Quantity: 10})
			return &RelationInfo{ID: 1, CampaignID: "camp-1", Metadata: meta}, nil
		},
	})
	svc.SetRelationMetadataUpdater(&mockMetadataUpdater{
		updateFn: func(_ context.Context, _ int, meta json.RawMessage) error {
			updatedMeta = meta
			return nil
		},
	})
	result, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID: "shop-1",
		ItemEntityID: "item-1",
		RelationID:   1,
		Quantity:     3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StockRemaining != 7 {
		t.Errorf("stock remaining = %d, want 7", result.StockRemaining)
	}
	// Verify updated metadata.
	var m shopMeta
	if err := json.Unmarshal(updatedMeta, &m); err != nil {
		t.Fatalf("bad metadata: %v", err)
	}
	if m.Quantity != 7 {
		t.Errorf("metadata quantity = %d, want 7", m.Quantity)
	}
}

func TestPurchase_WrongCampaign(t *testing.T) {
	svc := NewTransactionService(&mockTransactionRepo{})
	svc.SetRelationFinder(&mockRelationFinder{
		getByIDFn: func(_ context.Context, _ int) (*RelationInfo, error) {
			return &RelationInfo{ID: 1, CampaignID: "camp-other"}, nil
		},
	})
	_, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID: "shop-1",
		ItemEntityID: "item-1",
		RelationID:   1,
		Quantity:     1,
	})
	if err == nil {
		t.Error("expected IDOR error for wrong campaign")
	}
}

func TestCreateTransaction_Success(t *testing.T) {
	svc := NewTransactionService(&mockTransactionRepo{})
	tx, err := svc.CreateTransaction(context.Background(), "camp-1", "user-1", CreateTransactionInput{
		TransactionType: TxGift,
		ShopEntityID:    "shop-1",
		ItemEntityID:    "item-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.Currency != "gp" {
		t.Errorf("currency = %q, want default %q", tx.Currency, "gp")
	}
	if tx.Quantity != 1 {
		t.Errorf("quantity = %d, want default 1", tx.Quantity)
	}
}

func TestCreateTransaction_MissingType(t *testing.T) {
	svc := NewTransactionService(&mockTransactionRepo{})
	_, err := svc.CreateTransaction(context.Background(), "camp-1", "user-1", CreateTransactionInput{})
	if err == nil {
		t.Error("expected error for missing transaction type")
	}
}

func TestCreateTransaction_RepoError(t *testing.T) {
	repo := &mockTransactionRepo{
		createFn: func(_ context.Context, _ *Transaction) error {
			return errors.New("db error")
		},
	}
	svc := NewTransactionService(repo)
	_, err := svc.CreateTransaction(context.Background(), "camp-1", "user-1", CreateTransactionInput{
		TransactionType: TxGift,
	})
	if err == nil {
		t.Error("expected repo error to propagate")
	}
}

func TestListTransactions(t *testing.T) {
	repo := &mockTransactionRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ TransactionListOptions) ([]Transaction, int, error) {
			return []Transaction{{ID: 1}}, 1, nil
		},
	}
	svc := NewTransactionService(repo)
	txs, total, err := svc.ListTransactions(context.Background(), "camp-1", DefaultTransactionListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(txs) != 1 {
		t.Errorf("expected 1 transaction, got %d (total=%d)", len(txs), total)
	}
}

func TestParseShopMeta(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected int // quantity
	}{
		{"with quantity", json.RawMessage(`{"quantity":5}`), 5},
		{"zero defaults to unlimited", json.RawMessage(`{}`), -1},
		{"explicit unlimited", json.RawMessage(`{"quantity":-1}`), -1},
		{"nil input", nil, -1},
		{"invalid json", json.RawMessage(`{bad`), -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := parseShopMeta(tt.input)
			if m.Quantity != tt.expected {
				t.Errorf("quantity = %d, want %d", m.Quantity, tt.expected)
			}
		})
	}
}

// TestPurchase_BuyerAccessAllowed pins the happy path for the buyer
// cross-check: when the access checker returns true, Purchase succeeds and
// the transaction record carries the buyer entity ID through unchanged.
func TestPurchase_BuyerAccessAllowed(t *testing.T) {
	var checkedID, checkedUser string
	svc := NewTransactionService(&mockTransactionRepo{})
	svc.SetBuyerAccessChecker(&mockBuyerAccess{
		canFn: func(_ context.Context, entityID, userID string, _ int) (bool, error) {
			checkedID = entityID
			checkedUser = userID
			return true, nil
		},
	})
	result, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID:  "shop-1",
		ItemEntityID:  "item-1",
		BuyerEntityID: "buyer-1",
		Quantity:      1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checkedID != "buyer-1" {
		t.Errorf("checker called with entityID=%q, want %q", checkedID, "buyer-1")
	}
	if checkedUser != "user-1" {
		t.Errorf("checker called with userID=%q, want %q", checkedUser, "user-1")
	}
	if result.Transaction.BuyerEntityID == nil || *result.Transaction.BuyerEntityID != "buyer-1" {
		t.Errorf("transaction did not preserve buyer entity ID")
	}
}

// TestPurchase_BuyerAccessDenied pins the spoofing-defence path: the
// transaction service must reject Purchase when the access checker says no,
// without writing a transaction or decrementing stock. This is the test
// that pins the C-Phase-2 ownership cross-check.
func TestPurchase_BuyerAccessDenied(t *testing.T) {
	createCalled := false
	repo := &mockTransactionRepo{createFn: func(_ context.Context, _ *Transaction) error {
		createCalled = true
		return nil
	}}
	stockUpdated := false
	svc := NewTransactionService(repo)
	svc.SetRelationFinder(&mockRelationFinder{
		getByIDFn: func(_ context.Context, _ int) (*RelationInfo, error) {
			meta, _ := json.Marshal(shopMeta{Quantity: 5})
			return &RelationInfo{ID: 1, CampaignID: "camp-1", Metadata: meta}, nil
		},
	})
	svc.SetRelationMetadataUpdater(&mockMetadataUpdater{
		updateFn: func(_ context.Context, _ int, _ json.RawMessage) error {
			stockUpdated = true
			return nil
		},
	})
	svc.SetBuyerAccessChecker(&mockBuyerAccess{
		canFn: func(_ context.Context, _, _ string, _ int) (bool, error) {
			return false, nil
		},
	})

	_, err := svc.Purchase(context.Background(), "camp-1", "user-attacker", 1, CreateTransactionInput{
		ShopEntityID:  "shop-1",
		ItemEntityID:  "item-1",
		BuyerEntityID: "victim-character",
		RelationID:    1,
		Quantity:      1,
	})
	if err == nil {
		t.Fatal("expected forbidden error for buyer the user cannot act as")
	}
	if createCalled {
		t.Error("transaction was created despite access denial")
	}
	// NOTE: stock-decrement happens BEFORE the buyer check today only because
	// stock is keyed off the relation, not the buyer. The buyer check is
	// the first thing executed in Purchase(), so stock must NOT be touched.
	if stockUpdated {
		t.Error("shop stock was decremented despite access denial")
	}
}

// TestPurchase_BuyerAccessSkippedWhenNoBuyer pins that the cross-check is
// only invoked when a buyer entity is named — purchases without a buyer
// (e.g. GM-mediated cash sale) must not trip the check.
func TestPurchase_BuyerAccessSkippedWhenNoBuyer(t *testing.T) {
	checkerCalled := false
	svc := NewTransactionService(&mockTransactionRepo{})
	svc.SetBuyerAccessChecker(&mockBuyerAccess{
		canFn: func(_ context.Context, _, _ string, _ int) (bool, error) {
			checkerCalled = true
			return false, nil // would fail if invoked
		},
	})

	_, err := svc.Purchase(context.Background(), "camp-1", "user-1", 1, CreateTransactionInput{
		ShopEntityID: "shop-1",
		ItemEntityID: "item-1",
		// BuyerEntityID intentionally empty.
		Quantity: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checkerCalled {
		t.Error("access checker was invoked for a no-buyer purchase")
	}
}
