// transaction_reader.go is the viewer-aware read path for transaction lists.
// The repository joins entity names for display without any visibility check,
// so this layer blanks the name of every entity the viewer may not see before
// a list leaves the plugin.
package armory

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TransactionReader lists transactions for a specific viewer. role must be the
// visibility role (a co-DM already promoted), as the gallery uses.
type TransactionReader interface {
	// ListTransactions returns a campaign's transactions with names redacted.
	ListTransactions(ctx context.Context, campaignID string, role int, userID string, opts TransactionListOptions) ([]Transaction, int, error)

	// ListShopTransactions returns a shop's transactions with names redacted,
	// or a not-found error when the viewer cannot see the shop itself, so a
	// private shop's existence is not confirmed to them.
	ListShopTransactions(ctx context.Context, campaignID, shopEntityID string, role int, userID string, opts TransactionListOptions) ([]Transaction, int, error)
}

// transactionReader implements TransactionReader over the raw
// TransactionService list methods plus the canonical visibility filter.
type transactionReader struct {
	svc TransactionService
	vis EntityVisibilityFilter
}

// NewTransactionReader creates a reader. With a nil filter every non-Owner
// viewer is treated as unable to see any entity (fail closed).
func NewTransactionReader(svc TransactionService, vis EntityVisibilityFilter) TransactionReader {
	return &transactionReader{svc: svc, vis: vis}
}

// ListTransactions lists and redacts.
func (r *transactionReader) ListTransactions(ctx context.Context, campaignID string, role int, userID string, opts TransactionListOptions) ([]Transaction, int, error) {
	txs, total, err := r.svc.ListTransactions(ctx, campaignID, opts)
	if err != nil {
		return nil, 0, err
	}
	if err := r.redact(ctx, campaignID, role, userID, txs); err != nil {
		return nil, 0, err
	}
	return txs, total, nil
}

// ListShopTransactions checks the viewer can see the shop, then lists and redacts.
func (r *transactionReader) ListShopTransactions(ctx context.Context, campaignID, shopEntityID string, role int, userID string, opts TransactionListOptions) ([]Transaction, int, error) {
	if role < permissions.RoleOwner {
		viewable, err := r.viewable(ctx, campaignID, []string{shopEntityID}, role, userID)
		if err != nil {
			return nil, 0, err
		}
		if !viewable[shopEntityID] {
			return nil, 0, apperror.NewNotFound("shop")
		}
	}
	txs, total, err := r.svc.ListShopTransactions(ctx, campaignID, shopEntityID, opts)
	if err != nil {
		return nil, 0, err
	}
	if err := r.redact(ctx, campaignID, role, userID, txs); err != nil {
		return nil, 0, err
	}
	return txs, total, nil
}

// viewable resolves which ids the viewer may see, failing closed without a filter.
func (r *transactionReader) viewable(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]bool, error) {
	if r.vis == nil || len(ids) == 0 {
		return map[string]bool{}, nil
	}
	v, err := r.vis.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
	if err != nil {
		return nil, fmt.Errorf("filtering transaction visibility: %w", err)
	}
	return v, nil
}

// redact blanks the shop, item and buyer names of entities the viewer cannot
// see. Owners are unrestricted, matching the gallery's bypass point.
func (r *transactionReader) redact(ctx context.Context, campaignID string, role int, userID string, txs []Transaction) error {
	if role >= permissions.RoleOwner || len(txs) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for i := range txs {
		add(txs[i].ShopEntityID)
		add(txs[i].ItemEntityID)
		if txs[i].BuyerEntityID != nil {
			add(*txs[i].BuyerEntityID)
		}
	}
	viewable, err := r.viewable(ctx, campaignID, ids, role, userID)
	if err != nil {
		return err
	}
	for i := range txs {
		if !viewable[txs[i].ShopEntityID] {
			txs[i].ShopName = ""
		}
		if !viewable[txs[i].ItemEntityID] {
			txs[i].ItemName = ""
		}
		if txs[i].BuyerEntityID == nil || !viewable[*txs[i].BuyerEntityID] {
			txs[i].BuyerName = ""
		}
	}
	return nil
}
