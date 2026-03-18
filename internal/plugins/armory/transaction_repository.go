// transaction_repository.go provides data access for shop transactions.
// All SQL lives here — services never construct queries directly.
package armory

import (
	"context"
	"database/sql"
	"fmt"
)

// TransactionRepository defines the data access contract for shop transactions.
type TransactionRepository interface {
	// Create inserts a new transaction and sets the auto-generated ID.
	Create(ctx context.Context, tx *Transaction) error

	// ListByCampaign returns transactions with entity name joins,
	// filtered by the given options.
	ListByCampaign(ctx context.Context, campaignID string, opts TransactionListOptions) ([]Transaction, int, error)

	// ListByShop returns transactions for a specific shop entity.
	ListByShop(ctx context.Context, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error)

	// ListByBuyer returns transactions for a specific buyer entity.
	ListByBuyer(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error)
}

// transactionRepository implements TransactionRepository with MariaDB.
type transactionRepository struct {
	db *sql.DB
}

// NewTransactionRepository creates a new transaction repository.
func NewTransactionRepository(db *sql.DB) TransactionRepository {
	return &transactionRepository{db: db}
}

// Create inserts a new transaction record.
func (r *transactionRepository) Create(ctx context.Context, tx *Transaction) error {
	query := `INSERT INTO shop_transactions
		(campaign_id, instance_id, shop_entity_id, item_entity_id, buyer_entity_id,
		 relation_id, quantity, price_paid, currency, price_numeric,
		 transaction_type, notes, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		tx.CampaignID, nullInt(tx.InstanceID), tx.ShopEntityID, tx.ItemEntityID, nullString(tx.BuyerEntityID),
		nullInt(tx.RelationID), tx.Quantity, nullString(tx.PricePaid), tx.Currency, nullFloat(tx.PriceNumeric),
		tx.TransactionType, nullString(tx.Notes), nullString(tx.CreatedBy),
	)
	if err != nil {
		return fmt.Errorf("inserting transaction: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	tx.ID = int(id)
	return nil
}

// txSelectColumns is the column set for transaction list queries.
const txSelectColumns = `t.id, t.campaign_id, t.instance_id, t.shop_entity_id, t.item_entity_id,
	t.buyer_entity_id, t.relation_id, t.quantity, t.price_paid, t.currency,
	t.price_numeric, t.transaction_type, t.notes, t.created_at, t.created_by,
	COALESCE(es.name, ''), COALESCE(ei.name, ''), COALESCE(eb.name, '')`

// txFromJoins provides entity name lookups for display.
const txFromJoins = `FROM shop_transactions t
	LEFT JOIN entities es ON es.id = t.shop_entity_id
	LEFT JOIN entities ei ON ei.id = t.item_entity_id
	LEFT JOIN entities eb ON eb.id = t.buyer_entity_id`

// ListByCampaign returns transactions for a campaign with entity name joins.
func (r *transactionRepository) ListByCampaign(ctx context.Context, campaignID string, opts TransactionListOptions) ([]Transaction, int, error) {
	where, args := buildTxWhere(campaignID, opts)
	return r.queryTransactions(ctx, where, args, opts)
}

// ListByShop returns transactions for a specific shop.
func (r *transactionRepository) ListByShop(ctx context.Context, shopEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	opts.ShopEntityID = shopEntityID
	// Derive campaign from the first result or use an empty campaign filter.
	where := "WHERE t.shop_entity_id = ?"
	args := []any{shopEntityID}
	where, args = appendTxFilters(where, args, opts)
	return r.queryTransactions(ctx, where, args, opts)
}

// ListByBuyer returns transactions for a specific buyer entity.
func (r *transactionRepository) ListByBuyer(ctx context.Context, buyerEntityID string, opts TransactionListOptions) ([]Transaction, int, error) {
	opts.BuyerEntityID = buyerEntityID
	where := "WHERE t.buyer_entity_id = ?"
	args := []any{buyerEntityID}
	where, args = appendTxFilters(where, args, opts)
	return r.queryTransactions(ctx, where, args, opts)
}

// queryTransactions executes a paginated transaction query.
func (r *transactionRepository) queryTransactions(ctx context.Context, where string, args []any, opts TransactionListOptions) ([]Transaction, int, error) {
	// Count total.
	countQuery := fmt.Sprintf("SELECT COUNT(*) %s %s", txFromJoins, where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting transactions: %w", err)
	}

	// Fetch page.
	query := fmt.Sprintf("SELECT %s %s %s ORDER BY t.created_at DESC LIMIT ? OFFSET ?",
		txSelectColumns, txFromJoins, where)
	pageArgs := append(args, opts.PerPage, opts.Offset())

	rows, err := r.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing transactions: %w", err)
	}
	defer rows.Close()

	var txs []Transaction
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, 0, err
		}
		txs = append(txs, *t)
	}
	return txs, total, rows.Err()
}

// scanTransaction reads a single transaction row.
func scanTransaction(rows *sql.Rows) (*Transaction, error) {
	t := &Transaction{}
	var buyerID, pricePaid, notes, createdBy sql.NullString
	var instanceID, relationID sql.NullInt64
	var priceNum sql.NullFloat64

	err := rows.Scan(
		&t.ID, &t.CampaignID, &instanceID, &t.ShopEntityID, &t.ItemEntityID,
		&buyerID, &relationID, &t.Quantity, &pricePaid, &t.Currency,
		&priceNum, &t.TransactionType, &notes, &t.CreatedAt, &createdBy,
		&t.ShopName, &t.ItemName, &t.BuyerName,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning transaction: %w", err)
	}

	if instanceID.Valid {
		v := int(instanceID.Int64)
		t.InstanceID = &v
	}
	if buyerID.Valid {
		t.BuyerEntityID = &buyerID.String
	}
	if relationID.Valid {
		v := int(relationID.Int64)
		t.RelationID = &v
	}
	if pricePaid.Valid {
		t.PricePaid = &pricePaid.String
	}
	if priceNum.Valid {
		t.PriceNumeric = &priceNum.Float64
	}
	if notes.Valid {
		t.Notes = &notes.String
	}
	if createdBy.Valid {
		t.CreatedBy = &createdBy.String
	}
	return t, nil
}

// buildTxWhere constructs the WHERE clause for campaign-scoped queries.
func buildTxWhere(campaignID string, opts TransactionListOptions) (string, []any) {
	where := "WHERE t.campaign_id = ?"
	args := []any{campaignID}
	return appendTxFilters(where, args, opts)
}

// appendTxFilters adds optional filters to a WHERE clause.
func appendTxFilters(where string, args []any, opts TransactionListOptions) (string, []any) {
	if opts.ShopEntityID != "" {
		where += " AND t.shop_entity_id = ?"
		args = append(args, opts.ShopEntityID)
	}
	if opts.BuyerEntityID != "" {
		where += " AND t.buyer_entity_id = ?"
		args = append(args, opts.BuyerEntityID)
	}
	if opts.ItemEntityID != "" {
		where += " AND t.item_entity_id = ?"
		args = append(args, opts.ItemEntityID)
	}
	if opts.TransactionType != "" {
		where += " AND t.transaction_type = ?"
		args = append(args, opts.TransactionType)
	}
	return where, args
}

// nullString returns nil for empty strings, the pointer value otherwise.
func nullString(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

// nullInt returns nil for nil pointers, the value otherwise.
func nullInt(v *int) any {
	if v == nil || *v == 0 {
		return nil
	}
	return *v
}

// nullFloat returns nil for nil pointers, the value otherwise.
func nullFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// strPtr returns a pointer to s, or nil if empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// intPtr returns a pointer to v, or nil if zero.
func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

// floatPtr returns a pointer to v, or nil if zero.
func floatPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

