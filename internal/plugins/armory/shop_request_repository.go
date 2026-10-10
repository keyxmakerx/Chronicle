// shop_request_repository.go provides data access for purchase requests. All
// SQL for the table lives here, and every statement is scoped by campaign_id so
// an id from another campaign matches nothing.
package armory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PurchaseRequestRepository defines the data access contract for requests.
type PurchaseRequestRepository interface {
	// Insert stores a pending request and sets its ID.
	Insert(ctx context.Context, r *PurchaseRequest) error
	// Get returns one request in the campaign, or NotFound.
	Get(ctx context.Context, campaignID string, id int64) (*PurchaseRequest, error)
	// Settle moves a request from one status to another and reports false when
	// it was not in `from`, so two answers can't both win.
	Settle(ctx context.Context, campaignID string, id int64, from, to, reason, decidedBy string) (bool, error)
	// DeletePending removes a request only while it is still pending and
	// reports whether it did, so a withdrawal cannot race an approval or the
	// downtime sweep: whichever statement runs second matches no row.
	DeletePending(ctx context.Context, campaignID string, id int64) (bool, error)
	// ListPending returns the campaign's pending requests, oldest first.
	ListPending(ctx context.Context, campaignID string) ([]PurchaseRequest, error)
	// CountPendingBy counts one player's pending requests.
	CountPendingBy(ctx context.Context, campaignID, userID string) (int, error)
	// ListForBuyer returns a character's requests, newest first.
	ListForBuyer(ctx context.Context, campaignID, buyerEntityID string, limit int) ([]PurchaseRequest, error)
}

type purchaseRequestRepository struct {
	db *sql.DB
}

// NewPurchaseRequestRepository creates a purchase request repository.
func NewPurchaseRequestRepository(db *sql.DB) PurchaseRequestRepository {
	return &purchaseRequestRepository{db: db}
}

func (r *purchaseRequestRepository) Insert(ctx context.Context, p *PurchaseRequest) error {
	basket, err := json.Marshal(p.Basket)
	if err != nil {
		return fmt.Errorf("encoding basket: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO shop_purchase_requests
		   (campaign_id, shop_entity_id, buyer_entity_id, requested_by, basket, quoted_total, quoted_currency, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')`,
		p.CampaignID, p.ShopEntityID, p.BuyerEntityID, p.RequestedBy, string(basket),
		p.QuotedTotal.Decimal(), p.QuotedCurrency)
	if err != nil {
		return fmt.Errorf("inserting purchase request: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting purchase request id: %w", err)
	}
	p.ID, p.Status = id, PurchasePending
	return nil
}

const purchaseRequestSelect = `SELECT id, campaign_id, shop_entity_id, buyer_entity_id, requested_by,
	basket, CAST(ROUND(quoted_total * 100) AS SIGNED), quoted_currency, status, COALESCE(reason, ''), COALESCE(decided_by, ''), created_at, decided_at
	FROM shop_purchase_requests`

func scanPurchaseRequest(sc interface{ Scan(...any) error }) (*PurchaseRequest, error) {
	var p PurchaseRequest
	var basket []byte
	var quoted int64
	var decided sql.NullTime
	if err := sc.Scan(&p.ID, &p.CampaignID, &p.ShopEntityID, &p.BuyerEntityID, &p.RequestedBy,
		&basket, &quoted, &p.QuotedCurrency, &p.Status, &p.Reason, &p.DecidedBy, &p.CreatedAt, &decided); err != nil {
		return nil, err
	}
	p.QuotedTotal = Cents(quoted)
	if err := json.Unmarshal(basket, &p.Basket); err != nil {
		return nil, fmt.Errorf("decoding basket of request %d: %w", p.ID, err)
	}
	if decided.Valid {
		t := decided.Time
		p.DecidedAt = &t
	}
	return &p, nil
}

func (r *purchaseRequestRepository) Get(ctx context.Context, campaignID string, id int64) (*PurchaseRequest, error) {
	p, err := scanPurchaseRequest(r.db.QueryRowContext(ctx, purchaseRequestSelect+` WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("request")
	}
	if err != nil {
		return nil, fmt.Errorf("getting purchase request: %w", err)
	}
	return p, nil
}

func (r *purchaseRequestRepository) Settle(ctx context.Context, campaignID string, id int64, from, to, reason, decidedBy string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE shop_purchase_requests SET status = ?, reason = ?, decided_by = ?, decided_at = CURRENT_TIMESTAMP
		  WHERE campaign_id = ? AND id = ? AND status = ?`,
		to, nullStr(reason), nullStr(decidedBy), campaignID, id, from)
	if err != nil {
		return false, fmt.Errorf("settling purchase request: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *purchaseRequestRepository) DeletePending(ctx context.Context, campaignID string, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM shop_purchase_requests WHERE id = ? AND campaign_id = ? AND status = 'pending'`,
		id, campaignID)
	if err != nil {
		return false, fmt.Errorf("withdrawing purchase request: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *purchaseRequestRepository) ListPending(ctx context.Context, campaignID string) ([]PurchaseRequest, error) {
	return r.query(ctx, purchaseRequestSelect+` WHERE campaign_id = ? AND status = 'pending' ORDER BY created_at ASC, id ASC`, campaignID)
}

func (r *purchaseRequestRepository) CountPendingBy(ctx context.Context, campaignID, userID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM shop_purchase_requests WHERE campaign_id = ? AND requested_by = ? AND status = 'pending'`,
		campaignID, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting pending purchase requests: %w", err)
	}
	return n, nil
}

func (r *purchaseRequestRepository) ListForBuyer(ctx context.Context, campaignID, buyerEntityID string, limit int) ([]PurchaseRequest, error) {
	if limit < 1 || limit > 200 {
		limit = historyPageSize
	}
	return r.query(ctx, purchaseRequestSelect+
		` WHERE campaign_id = ? AND buyer_entity_id = ? ORDER BY created_at DESC, id DESC LIMIT `+strconv.Itoa(limit),
		campaignID, buyerEntityID)
}

func (r *purchaseRequestRepository) query(ctx context.Context, q string, args ...any) ([]PurchaseRequest, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing purchase requests: %w", err)
	}
	defer rows.Close()
	var out []PurchaseRequest
	for rows.Next() {
		p, err := scanPurchaseRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning purchase request: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}
