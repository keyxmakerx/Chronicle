package armory

// Row-level tests for the purchase request table: the JSON basket round trip,
// the compare-and-set Settle that makes two answers unable to both win, the
// per-player pending count, and the cascade when a shop is deleted. Skips when
// no test database is configured (see stash_repository_integration_test.go).

import (
	"context"
	"testing"
)

func TestPurchaseRequestRepoIntegration(t *testing.T) {
	db := newStashScratchDB(t)
	repo := NewPurchaseRequestRepository(db)
	ctx := context.Background()
	camp := seedStashCampaign(t, db)
	other := seedStashCampaign(t, db)
	shop := seedStashEntity(t, db, camp, "Cup")
	buyer := seedStashEntity(t, db, camp, "Thorin")

	req := func(user string) *PurchaseRequest {
		r := &PurchaseRequest{CampaignID: camp, ShopEntityID: shop, BuyerEntityID: buyer, RequestedBy: user, QuotedTotal: 1234, QuotedCurrency: "gp",
			Basket: []BuyItemInput{{RelationID: 7, Quantity: 2}, {RelationID: 9, Quantity: 1}}}
		if err := repo.Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, b, c := req("u1"), req("u1"), req("u2")

	got, err := repo.Get(ctx, camp, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != PurchasePending || got.ItemCount() != 3 || len(got.Basket) != 2 || got.Basket[0].RelationID != 7 || got.RequestedBy != "u1" || got.QuotedTotal != 1234 || got.QuotedCurrency != "gp" {
		t.Errorf("round trip = %+v", got)
	}
	if _, err := repo.Get(ctx, other, a.ID); code(err) != 404 {
		t.Errorf("another campaign's request: %v", err)
	}

	if n, _ := repo.CountPendingBy(ctx, camp, "u1"); n != 2 {
		t.Errorf("u1 pending = %d", n)
	}
	pending, err := repo.ListPending(ctx, camp)
	if err != nil || len(pending) != 3 || pending[0].ID != a.ID || pending[2].ID != c.ID {
		t.Fatalf("pending = %+v err %v", pending, err)
	}

	// Only one of two racing answers wins, and a settled row can be claimed no more.
	if ok, err := repo.Settle(ctx, camp, a.ID, PurchasePending, PurchaseApplied, "why", "gm"); err != nil || !ok {
		t.Fatalf("first settle: %v %v", ok, err)
	}
	if ok, _ := repo.Settle(ctx, camp, a.ID, PurchasePending, PurchaseDeclined, "", "gm"); ok {
		t.Error("a settled request was settled again")
	}
	if ok, _ := repo.Settle(ctx, other, b.ID, PurchasePending, PurchaseDeclined, "", "gm"); ok {
		t.Error("settled another campaign's request")
	}
	if ok, err := repo.Settle(ctx, camp, a.ID, PurchaseApplied, PurchaseFailed, "short", "gm"); err != nil || !ok {
		t.Fatalf("applied to failed: %v %v", ok, err)
	}
	got, _ = repo.Get(ctx, camp, a.ID)
	if got.Status != PurchaseFailed || got.Reason != "short" || got.DecidedBy != "gm" || got.DecidedAt == nil {
		t.Errorf("settled = %+v", got)
	}
	if n, _ := repo.CountPendingBy(ctx, camp, "u1"); n != 1 {
		t.Errorf("u1 pending after settle = %d", n)
	}

	// A withdrawal removes only a still-pending row of its own campaign.
	if ok, _ := repo.DeletePending(ctx, camp, a.ID); ok {
		t.Error("deleted a settled request")
	}
	if ok, _ := repo.DeletePending(ctx, other, b.ID); ok {
		t.Error("deleted another campaign's request")
	}

	hist, err := repo.ListForBuyer(ctx, camp, buyer, 2)
	if err != nil || len(hist) != 2 || hist[0].ID != c.ID {
		t.Fatalf("history = %+v err %v", hist, err)
	}

	// Deleting the shop takes its requests with it.
	if _, err := db.Exec(`DELETE FROM entities WHERE id = ?`, shop); err != nil {
		t.Fatal(err)
	}
	if left, _ := repo.ListPending(ctx, camp); len(left) != 0 {
		t.Errorf("requests outlived their shop: %+v", left)
	}
}
