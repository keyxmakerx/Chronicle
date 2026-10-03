package armory

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func readerFixture() []Transaction {
	buyer := "buyer-1"
	return []Transaction{
		{ID: 1, ShopEntityID: "shop-pub", ItemEntityID: "item-pub", BuyerEntityID: &buyer, ShopName: "Open Shop", ItemName: "Sword", BuyerName: "Ann"},
		{ID: 2, ShopEntityID: "shop-priv", ItemEntityID: "item-priv", ShopName: "Secret Shop", ItemName: "Wand"},
	}
}

// Names of entities the viewer cannot see are blanked in list rows; visible
// names are untouched, and Owners are unrestricted.
func TestTransactionReader_ListTransactions_Redaction(t *testing.T) {
	tests := []struct {
		name     string
		role     int
		vis      EntityVisibilityFilter
		wantRow1 [3]string // shop, item, buyer
		wantRow2 [3]string
	}{
		{"owner sees all", permissions.RoleOwner, nil, [3]string{"Open Shop", "Sword", "Ann"}, [3]string{"Secret Shop", "Wand", ""}},
		{"player partial", permissions.RolePlayer,
			&mockVisibilityFilter{viewable: map[string]bool{"shop-pub": true, "item-pub": true}},
			[3]string{"Open Shop", "Sword", ""}, [3]string{"", "", ""}},
		{"player all visible", permissions.RolePlayer,
			&mockVisibilityFilter{viewable: map[string]bool{"shop-pub": true, "item-pub": true, "buyer-1": true, "shop-priv": true, "item-priv": true}},
			[3]string{"Open Shop", "Sword", "Ann"}, [3]string{"Secret Shop", "Wand", ""}},
		{"no filter fails closed", permissions.RolePlayer, nil, [3]string{}, [3]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockTransactionRepo{listByCampaignFn: func(context.Context, string, TransactionListOptions) ([]Transaction, int, error) {
				return readerFixture(), 2, nil
			}}
			r := NewTransactionReader(NewTransactionService(repo), tc.vis)
			txs, total, err := r.ListTransactions(context.Background(), "camp", tc.role, "u1", DefaultTransactionListOptions())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != 2 || len(txs) != 2 {
				t.Fatalf("total=%d rows=%d, want 2/2", total, len(txs))
			}
			if got := [3]string{txs[0].ShopName, txs[0].ItemName, txs[0].BuyerName}; got != tc.wantRow1 {
				t.Errorf("row1 names = %v, want %v", got, tc.wantRow1)
			}
			if got := [3]string{txs[1].ShopName, txs[1].ItemName, txs[1].BuyerName}; got != tc.wantRow2 {
				t.Errorf("row2 names = %v, want %v", got, tc.wantRow2)
			}
		})
	}
}

// A shop the viewer cannot see is a 404, and its transactions are never read.
func TestTransactionReader_ListShopTransactions(t *testing.T) {
	tests := []struct {
		name     string
		role     int
		vis      EntityVisibilityFilter
		wantCode int
		wantRead bool
	}{
		{"visible shop", permissions.RolePlayer, &mockVisibilityFilter{viewable: map[string]bool{"shop-pub": true}}, 0, true},
		{"hidden shop", permissions.RolePlayer, &mockVisibilityFilter{viewable: map[string]bool{}}, http.StatusNotFound, false},
		{"no filter fails closed", permissions.RolePlayer, nil, http.StatusNotFound, false},
		{"owner bypasses filter", permissions.RoleOwner, nil, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			read := false
			repo := &mockTransactionRepo{listByShopFn: func(context.Context, string, string, TransactionListOptions) ([]Transaction, int, error) {
				read = true
				return nil, 0, nil
			}}
			r := NewTransactionReader(NewTransactionService(repo), tc.vis)
			_, _, err := r.ListShopTransactions(context.Background(), "camp", "shop-pub", tc.role, "u1", DefaultTransactionListOptions())
			if tc.wantCode == 0 && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantCode != 0 {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tc.wantCode {
					t.Fatalf("err = %v, want app error %d", err, tc.wantCode)
				}
			}
			if read != tc.wantRead {
				t.Errorf("repo read = %v, want %v", read, tc.wantRead)
			}
		})
	}
}
