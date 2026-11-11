package armory

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func TestPurchase_RecordsWhatBuyCharges(t *testing.T) {
	tests := []struct {
		name      string
		price     string
		qty       int
		wantTotal float64
	}{
		{"sub-cent price rounds per unit", `0.333`, 3, 0.99},
		{"whole price", `5`, 2, 10},
		{"half cent rounds up", `0.005`, 1, 0.01},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got *Transaction
			repo := &mockTransactionRepo{createFn: func(_ context.Context, tx *Transaction) error { got = tx; return nil }}
			svc := NewTransactionService(repo)
			svc.relationFinder = shopRelation(`{"in_stock":true,"price":` + tc.price + `,"quantity":-1}`)
			in := buyOne()
			in.Quantity = tc.qty
			if _, err := svc.Purchase(context.Background(), "camp-1", "u", 3, in); err != nil {
				t.Fatal(err)
			}
			if got == nil || got.PriceNumeric == nil || *got.PriceNumeric != tc.wantTotal {
				t.Fatalf("recorded %v, want %v", got.PriceNumeric, tc.wantTotal)
			}
		})
	}
}

func TestTransactionText_LengthChecked(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	tests := []struct {
		name string
		in   CreateTransactionInput
		ok   bool
	}{
		{"type at limit", CreateTransactionInput{TransactionType: long(20)}, true},
		{"type 21 chars", CreateTransactionInput{TransactionType: long(21)}, false},
		{"currency 51 chars", CreateTransactionInput{TransactionType: "sale", Currency: long(51)}, false},
		{"currency at limit", CreateTransactionInput{TransactionType: "sale", Currency: long(50)}, true},
		{"price paid 101 chars", CreateTransactionInput{TransactionType: "sale", PricePaid: long(101)}, false},
		{"multibyte counts characters", CreateTransactionInput{TransactionType: strings.Repeat("é", 20)}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewTransactionService(&mockTransactionRepo{})
			_, err := svc.CreateTransaction(context.Background(), "camp-1", "u", tc.in)
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && code(err) != 400 {
				t.Fatalf("code %d (%v), want 400", code(err), err)
			}
		})
	}
}

// A listing whose currency cannot be recorded is refused before stock moves.
func TestPurchase_LongListingCurrencyRefusedBeforeStockTaken(t *testing.T) {
	updated := false
	svc := NewTransactionService(&mockTransactionRepo{})
	svc.relationFinder = shopRelation(`{"in_stock":true,"price":1,"quantity":5,"currency":"` + strings.Repeat("c", 51) + `"}`)
	svc.metadataUpdater = &mockMetadataUpdater{ifFn: func(context.Context, int, json.RawMessage, json.RawMessage) (bool, error) {
		updated = true
		return true, nil
	}}
	_, err := svc.Purchase(context.Background(), "camp-1", "u", 3, buyOne())
	if code(err) != 400 || updated {
		t.Fatalf("code %d, stock written %v", code(err), updated)
	}
}

// Rows created inside one second must page without repeats or gaps.
func TestTransactionRepoIntegration_SameSecondTiePages(t *testing.T) {
	db := newStashScratchDB(t)
	cid := seedStashCampaign(t, db)
	shop, item := seedStashEntity(t, db, cid, "Shop"), seedStashEntity(t, db, cid, "Item")
	repo := NewTransactionRepository(db)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		tx := &Transaction{CampaignID: cid, ShopEntityID: shop, ItemEntityID: item, Quantity: 1, Currency: "gp", TransactionType: TxPurchase}
		if err := repo.Create(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	setSameSecond(t, db, cid)
	seen := map[int]bool{}
	var order []int
	for page := 1; page <= 3; page++ {
		rows, total, err := repo.ListByCampaign(ctx, cid, TransactionListOptions{Page: page, PerPage: 2})
		if err != nil {
			t.Fatal(err)
		}
		if total != 5 {
			t.Fatalf("total %d", total)
		}
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatalf("id %d repeated on page %d", r.ID, page)
			}
			seen[r.ID] = true
			order = append(order, r.ID)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d rows, want 5", len(seen))
	}
	for i := 1; i < len(order); i++ {
		if order[i] > order[i-1] {
			t.Fatalf("not newest first: %v", order)
		}
	}
}

func setSameSecond(t *testing.T, db *sql.DB, campaignID string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE shop_transactions SET created_at = '2026-01-01 12:00:00' WHERE campaign_id = ?`, campaignID); err != nil {
		t.Fatal(err)
	}
}
