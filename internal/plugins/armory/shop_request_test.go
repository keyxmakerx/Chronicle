package armory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakePurchaseRepo is an in-memory PurchaseRequestRepository with the same
// compare-and-set Settle as the SQL one.
type fakePurchaseRepo struct {
	mu   sync.Mutex
	rows []*PurchaseRequest
	next int64
	now  time.Time
}

func newFakePurchaseRepo() *fakePurchaseRepo {
	return &fakePurchaseRepo{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (r *fakePurchaseRepo) Insert(_ context.Context, p *PurchaseRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	c := *p
	c.ID, c.Status = r.next, PurchasePending
	r.now = r.now.Add(time.Minute)
	c.CreatedAt = r.now
	c.Basket = append([]BuyItemInput(nil), p.Basket...)
	r.rows = append(r.rows, &c)
	p.ID, p.Status = c.ID, c.Status
	return nil
}

func (r *fakePurchaseRepo) find(campaignID string, id int64) *PurchaseRequest {
	for _, p := range r.rows {
		if p.ID == id && p.CampaignID == campaignID {
			return p
		}
	}
	return nil
}

func (r *fakePurchaseRepo) Get(_ context.Context, campaignID string, id int64) (*PurchaseRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.find(campaignID, id)
	if p == nil {
		return nil, apperror.NewNotFound("request")
	}
	c := *p
	return &c, nil
}

func (r *fakePurchaseRepo) Settle(_ context.Context, campaignID string, id int64, from, to, reason, decidedBy string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.find(campaignID, id)
	if p == nil || p.Status != from {
		return false, nil
	}
	p.Status, p.Reason, p.DecidedBy = to, reason, decidedBy
	return true, nil
}

func (r *fakePurchaseRepo) DeletePending(_ context.Context, campaignID string, id int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, p := range r.rows {
		if p.ID == id && p.CampaignID == campaignID && p.Status == PurchasePending {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (r *fakePurchaseRepo) ListPending(_ context.Context, campaignID string) ([]PurchaseRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []PurchaseRequest
	for _, p := range r.rows {
		if p.CampaignID == campaignID && p.Status == PurchasePending {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *fakePurchaseRepo) CountPendingBy(_ context.Context, campaignID, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, p := range r.rows {
		if p.CampaignID == campaignID && p.RequestedBy == userID && p.Status == PurchasePending {
			n++
		}
	}
	return n, nil
}

func (r *fakePurchaseRepo) ListForBuyer(_ context.Context, campaignID, buyerID string, limit int) ([]PurchaseRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []PurchaseRequest
	for i := len(r.rows) - 1; i >= 0; i-- {
		p := r.rows[i]
		if p.CampaignID == campaignID && p.BuyerEntityID == buyerID && len(out) < limit {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *fakePurchaseRepo) status(id int64) string {
	for _, p := range r.rows {
		if p.ID == id {
			return p.Status
		}
	}
	return ""
}

func (r *fakePurchaseRepo) reason(id int64) string {
	for _, p := range r.rows {
		if p.ID == id {
			return p.Reason
		}
	}
	return ""
}

// untouched snapshots everything a request must not change until it is applied.
type untouched struct {
	stock1, stock2 int
	gp1            any
	held           int
	txRows         int
}

func (f *buyFx) snapshot() untouched {
	stock := func(id int) int {
		if _, ok := f.listings.rels[id]; !ok {
			return -2
		}
		return f.listings.stock(id)
	}
	return untouched{stock(1), stock(2), f.money("c1"), f.carried("c1", "i1"), len(f.txRows)}
}

func (f *buyFx) assertUnchanged(t *testing.T, want untouched) {
	t.Helper()
	if got := f.snapshot(); got != want {
		t.Errorf("state changed: %+v, want %+v", got, want)
	}
}

// ask queues a request as a player while downtime is closed.
func (f *buyFx) ask(t *testing.T, actor Actor, in BuyInput) int64 {
	t.Helper()
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", actor, in)
	if err != nil {
		t.Fatalf("queueing: %v", err)
	}
	if res.Status != BuyStatusRequested || res.Spent != nil || res.MoneyLeft != nil {
		t.Fatalf("result = %+v, want a bare requested status", res)
	}
	return f.reqs.next
}

func TestShopRequest_QueuedWithNoSideEffects(t *testing.T) {
	f := newBuyFx()
	before := f.snapshot()
	id := f.ask(t, pU1, basket("c1", [2]int{1, 2}, [2]int{2, 3}))

	f.assertUnchanged(t, before)
	if f.reqs.status(id) != PurchasePending {
		t.Errorf("status = %q", f.reqs.status(id))
	}
	row := f.reqs.rows[0]
	if row.RequestedBy != "u1" || row.BuyerEntityID != "c1" || row.ShopEntityID != "shop-1" {
		t.Errorf("row = %+v", row)
	}
	// Only listing ids and quantities are stored, never a price.
	raw, _ := json.Marshal(row.Basket)
	if string(raw) != `[{"relationId":1,"quantity":2},{"relationId":2,"quantity":3}]` {
		t.Errorf("basket = %s", raw)
	}
}

func TestShopRequest_OwnerStillBuysAtOnceWhileClosed(t *testing.T) {
	f := newBuyFx()
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", owner, basket("c1", [2]int{1, 1}))
	if err != nil || res.Status != BuyStatusBought {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(f.reqs.rows) != 0 {
		t.Errorf("owner purchase was queued")
	}
}

func TestShopRequest_OpenDowntimeStillBuysAtOnce(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}))
	if err != nil || res.Status != BuyStatusBought || len(f.reqs.rows) != 0 {
		t.Fatalf("res=%+v err=%v rows=%d", res, err, len(f.reqs.rows))
	}
}

func TestShopRequest_QueueRefusals(t *testing.T) {
	cases := []struct {
		name     string
		actor    Actor
		in       BuyInput
		prep     func(*buyFx)
		wantCode int
	}{
		{"unknown listing", pU1, basket("c1", [2]int{99, 1}), nil, 404},
		{"listing of another shop", pU1, basket("c1", [2]int{4, 1}), nil, 404},
		{"dm-only listing", pU1, basket("c1", [2]int{7, 1}), nil, 404},
		{"unpriced listing", pU1, basket("c1", [2]int{8, 1}), nil, 400},
		{"mixed currency", pU1, basket("c1", [2]int{1, 1}, [2]int{3, 1}), nil, 400},
		{"someone else's character", pU1, basket("c2", [2]int{1, 1}), nil, 403},
		{"sheet without coin field", pU3, basket("c3", [2]int{1, 1}), nil, 400},
		{"empty basket", pU1, BuyInput{BuyerEntityID: "c1"}, nil, 400},
		{"shop hidden from the player", pU1, basket("c1", [2]int{1, 1}), func(f *buyFx) { f.vis.hidden["shop-1"] = true }, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			if tc.prep != nil {
				tc.prep(f)
			}
			_, err := f.svc.Buy(context.Background(), "camp", "shop-1", tc.actor, tc.in)
			if code(err) != tc.wantCode {
				t.Fatalf("code = %d (%v), want %d", code(err), err, tc.wantCode)
			}
			if len(f.reqs.rows) != 0 {
				t.Errorf("a refused basket was stored")
			}
		})
	}
}

func TestShopRequest_PerPlayerCap(t *testing.T) {
	f := newBuyFx()
	for i := 0; i < maxPendingPurchasesPerUser; i++ {
		f.ask(t, pU1, basket("c1", [2]int{2, 1}))
	}
	_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{2, 1}))
	if code(err) != 400 || !strings.Contains(err.Error(), "waiting for the GM already") {
		t.Fatalf("err = %v, want a plain 400", err)
	}
	if len(f.reqs.rows) != maxPendingPurchasesPerUser {
		t.Errorf("rows = %d", len(f.reqs.rows))
	}
	// The cap is per player, and answered requests free a place.
	f.ask(t, pU2, basket("c2", [2]int{2, 1}))
	if _, err := f.svc.DeclineRequest(context.Background(), "camp", owner, 1); err != nil {
		t.Fatal(err)
	}
	f.ask(t, pU1, basket("c1", [2]int{2, 1}))
}

func TestShopRequest_ApproveAppliesOnce(t *testing.T) {
	f := newBuyFx()
	id := f.ask(t, pU1, basket("c1", [2]int{1, 2}, [2]int{2, 3}))

	req, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != PurchaseApplied || f.reqs.status(id) != PurchaseApplied || f.reqs.rows[0].DecidedBy != "gm" {
		t.Fatalf("request = %+v stored %q", req, f.reqs.status(id))
	}
	// 2 x 10 + 3 x 2.5 = 27.5 spent, from the requester's own character.
	if got, _ := f.money("c1").(float64); got != 22.5 {
		t.Errorf("coins = %v, want 22.5", f.money("c1"))
	}
	if f.listings.stock(1) != 3 || f.carried("c1", "i1") != 5 || f.carried("c1", "i2") != 3 {
		t.Errorf("stock %d, holds i1 %d i2 %d", f.listings.stock(1), f.carried("c1", "i1"), f.carried("c1", "i2"))
	}
	if len(f.txRows) != 2 {
		t.Fatalf("transactions = %d", len(f.txRows))
	}
	for _, row := range f.txRows {
		if row.TransactionType != TxPurchase || row.BuyerEntityID == nil || *row.BuyerEntityID != "c1" {
			t.Errorf("bad row %+v", row)
		}
	}

	// A second approval (a double click) changes nothing.
	after := f.snapshot()
	if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); code(err) != 409 {
		t.Fatalf("second approve: %v", err)
	}
	f.assertUnchanged(t, after)
}

func TestShopRequest_ApproveFailsWithoutSideEffects(t *testing.T) {
	cases := []struct {
		name       string
		basket     BuyInput
		prep       func(*buyFx)
		wantReason string
	}{
		{"not enough coin", basket("c1", [2]int{1, 2}), func(f *buyFx) { f.fields.data["c1"]["gp"] = 5.0 }, "Not enough coin"},
		{"sold out", basket("c1", [2]int{1, 2}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"price":10,"currency":"gp","quantity":0}`)
		}, "stock"},
		{"less in stock than asked", basket("c1", [2]int{1, 2}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"price":10,"currency":"gp","quantity":1}`)
		}, "stock"},
		{"price rose past the coins", basket("c1", [2]int{1, 2}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"price":30,"currency":"gp","quantity":5}`)
		}, "The price went up since you asked."},
		{"price rose but still affordable", basket("c1", [2]int{1, 1}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"price":20,"currency":"gp","quantity":5}`)
		}, "The price went up since you asked."},
		{"price rose by one cent", basket("c1", [2]int{1, 1}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"price":10.01,"currency":"gp","quantity":5}`)
		}, "The price went up since you asked."},
		{"currency changed", basket("c1", [2]int{1, 1}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"price":1,"currency":"sp","quantity":5}`)
		}, "The price went up since you asked."},
		{"price removed", basket("c1", [2]int{1, 1}), func(f *buyFx) {
			f.listings.rels[1].Metadata = json.RawMessage(`{"quantity":5}`)
		}, "no price"},
		{"currency changed under a mixed basket", basket("c1", [2]int{1, 1}, [2]int{2, 1}), func(f *buyFx) {
			f.listings.rels[2].Metadata = json.RawMessage(`{"price":2.5,"currency":"sp"}`)
		}, "different currencies"},
		{"listing removed", basket("c1", [2]int{1, 1}), func(f *buyFx) { delete(f.listings.rels, 1) }, "no longer for sale"},
		{"listing became dm-only", basket("c1", [2]int{1, 1}), func(f *buyFx) { f.listings.rels[1].DmOnly = true }, "no longer for sale"},
		{"good became hidden", basket("c1", [2]int{2, 1}), func(f *buyFx) { f.vis.hidden["i2"] = true }, "no longer for sale"},
		{"shop became hidden", basket("c1", [2]int{1, 1}), func(f *buyFx) { f.vis.hidden["shop-1"] = true }, "shop is no longer available"},
		{"character no longer the requester's", basket("c1", [2]int{1, 1}), func(f *buyFx) { f.dir.ents["c1"].OwnerUserID = "u2" }, "no longer yours"},
		{"character lost its coin field", basket("c1", [2]int{1, 1}), func(f *buyFx) { f.dir.ents["c1"].MoneyKey = "" }, "no coin field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			id := f.ask(t, pU1, tc.basket)
			tc.prep(f)
			stock3 := f.listings.stock(3)
			before := f.snapshot()

			req, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id)
			if err != nil {
				t.Fatalf("a failing request is a recorded outcome, got error %v", err)
			}
			if req.Status != PurchaseFailed || f.reqs.status(id) != PurchaseFailed {
				t.Fatalf("status = %q / %q", req.Status, f.reqs.status(id))
			}
			if !strings.Contains(f.reqs.reason(id), tc.wantReason) {
				t.Errorf("reason = %q, want it to mention %q", f.reqs.reason(id), tc.wantReason)
			}
			f.assertUnchanged(t, before)
			if f.listings.stock(3) != stock3 || len(f.txRows) != 0 {
				t.Errorf("side effects: stock3 %d, rows %d", f.listings.stock(3), len(f.txRows))
			}
			// A failed request is final; approving it again does nothing.
			if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); code(err) != 409 {
				t.Errorf("second approve: %v", err)
			}
		})
	}
}

// A player is never charged more than the total quoted when they asked, and
// a lower price applies at the lower price.
func TestShopRequest_QuoteCapsThePrice(t *testing.T) {
	cases := []struct {
		name       string
		listing    string
		wantStatus string
		wantCoins  float64
	}{
		{"raised: refused", `{"price":20,"currency":"gp","quantity":5}`, PurchaseFailed, 50},
		{"lowered: applied at the lower price", `{"price":4,"currency":"gp","quantity":5}`, PurchaseApplied, 46},
		{"unchanged: applied", `{"price":10,"currency":"gp","quantity":5}`, PurchaseApplied, 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
			if q := f.reqs.rows[0]; q.QuotedTotal != 1000 || q.QuotedCurrency != "gp" {
				t.Fatalf("quote = %v %q", q.QuotedTotal, q.QuotedCurrency)
			}
			f.listings.rels[1].Metadata = json.RawMessage(tc.listing)
			if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
				t.Fatal(err)
			}
			if f.reqs.status(id) != tc.wantStatus {
				t.Fatalf("status = %q (%s)", f.reqs.status(id), f.reqs.reason(id))
			}
			if got, _ := f.money("c1").(float64); got != tc.wantCoins {
				t.Errorf("coins = %v, want %v", got, tc.wantCoins)
			}
			if tc.wantStatus == PurchaseFailed && (len(f.txRows) != 0 || f.listings.stock(1) != 5 || f.reqs.reason(id) != "The price went up since you asked.") {
				t.Errorf("failed request had effects: rows %d stock %d reason %q", len(f.txRows), f.listings.stock(1), f.reqs.reason(id))
			}
		})
	}
}

func TestShopRequest_Decline(t *testing.T) {
	f := newBuyFx()
	id := f.ask(t, pU1, basket("c1", [2]int{1, 2}))
	before := f.snapshot()

	req, err := f.svc.DeclineRequest(context.Background(), "camp", owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != PurchaseDeclined || f.reqs.status(id) != PurchaseDeclined || f.reqs.rows[0].DecidedBy != "gm" {
		t.Errorf("request = %+v stored %q", req, f.reqs.status(id))
	}
	f.assertUnchanged(t, before)
	for name, answer := range map[string]func() error{
		"decline again":    func() error { _, err := f.svc.DeclineRequest(context.Background(), "camp", owner, id); return err },
		"approve declined": func() error { _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); return err },
	} {
		if err := answer(); code(err) != 409 {
			t.Errorf("%s: %v", name, err)
		}
	}
	f.assertUnchanged(t, before)
}

func TestShopRequest_OnlyTheOwnerAnswers(t *testing.T) {
	scribe := Actor{UserID: "s", Role: rScribe}
	for name, who := range map[string]Actor{"requester": pU1, "other player": pU2, "scribe": scribe} {
		t.Run(name, func(t *testing.T) {
			f := newBuyFx()
			id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
			before := f.snapshot()
			if _, err := f.svc.ApproveRequest(context.Background(), "camp", who, id); code(err) != 403 {
				t.Errorf("approve: %v", err)
			}
			if _, err := f.svc.DeclineRequest(context.Background(), "camp", who, id); code(err) != 403 {
				t.Errorf("decline: %v", err)
			}
			if f.reqs.status(id) != PurchasePending {
				t.Errorf("status = %q", f.reqs.status(id))
			}
			f.assertUnchanged(t, before)
		})
	}
	t.Run("unknown and foreign request", func(t *testing.T) {
		f := newBuyFx()
		f.ask(t, pU1, basket("c1", [2]int{1, 1}))
		if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, 99); code(err) != 404 {
			t.Errorf("unknown: %v", err)
		}
		if _, err := f.svc.ApproveRequest(context.Background(), "other-camp", owner, 1); code(err) != 404 {
			t.Errorf("another campaign's request: %v", err)
		}
	})
}

func TestShopRequest_DowntimeSweep(t *testing.T) {
	f := newBuyFx()
	stash := f.fx.svc.(*stashService)
	ok1 := f.ask(t, pU1, basket("c1", [2]int{1, 2})) // 20 gp
	bad := f.ask(t, pU2, basket("c2", [2]int{1, 2})) // c2 holds 10 gp
	ok2 := f.ask(t, pU1, basket("c1", [2]int{2, 2})) // 5 gp
	declined := f.ask(t, pU1, basket("c1", [2]int{2, 1}))
	if _, err := f.svc.DeclineRequest(context.Background(), "camp", owner, declined); err != nil {
		t.Fatal(err)
	}

	// Closing downtime applies nothing.
	if _, err := stash.SetDowntime(context.Background(), "camp", owner, false); err != nil {
		t.Fatal(err)
	}
	if f.reqs.status(ok1) != PurchasePending {
		t.Fatalf("closing swept a request")
	}

	if _, err := stash.SetDowntime(context.Background(), "camp", pU1, true); code(err) != 403 {
		t.Fatalf("a player opened downtime: %v", err)
	}
	res, err := stash.SetDowntime(context.Background(), "camp", owner, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 2 || res.Failed != 1 {
		t.Errorf("result = %+v, want 2 applied 1 failed", res)
	}
	want := map[int64]string{ok1: PurchaseApplied, ok2: PurchaseApplied, bad: PurchaseFailed, declined: PurchaseDeclined}
	for id, status := range want {
		if f.reqs.status(id) != status {
			t.Errorf("request %d = %q, want %q", id, f.reqs.status(id), status)
		}
	}
	if f.reqs.reason(ok1) != autoAppliedReason || f.reqs.reason(bad) != "Not enough coin" {
		t.Errorf("reasons: %q / %q", f.reqs.reason(ok1), f.reqs.reason(bad))
	}
	// 50 - 20 - 5; the failed request left c2 alone.
	if got, _ := f.money("c1").(float64); got != 25 {
		t.Errorf("c1 coins = %v, want 25", f.money("c1"))
	}
	if f.money("c2") != "10" {
		t.Errorf("c2 coins = %v, want untouched", f.money("c2"))
	}
	// Each applied request wrote its purchase rows once.
	if len(f.txRows) != 2 {
		t.Errorf("transactions = %d, want 2", len(f.txRows))
	}
	// Opening again finds nothing left to apply.
	if res, _ := stash.SetDowntime(context.Background(), "camp", owner, true); res.Applied != 0 || res.Failed != 0 {
		t.Errorf("second open = %+v", res)
	}
}

func TestShopRequest_ConcurrentApprovalsChargeOnce(t *testing.T) {
	f := newBuyFx()
	id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var wins int
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("approvals that went through = %d", wins)
	}
	if got, _ := f.money("c1").(float64); got != 40 || len(f.txRows) != 1 {
		t.Errorf("coins %v rows %d, want one charge of 10", f.money("c1"), len(f.txRows))
	}
}

func TestShopRequest_PageAndHistory(t *testing.T) {
	f := newBuyFx()
	stash := f.fx.svc.(*stashService)
	id := f.ask(t, pU1, basket("c1", [2]int{1, 2}, [2]int{2, 6})) // 20 + 15 = 35 gp
	f.ask(t, pU2, basket("c2", [2]int{2, 1}))

	view, err := stash.StashesPage(context.Background(), "camp", owner)
	if err != nil {
		t.Fatal(err)
	}
	if view.WaitingCount() != 2 || len(view.PendingPurchases) != 2 {
		t.Fatalf("waiting = %d", view.WaitingCount())
	}
	l := view.PendingPurchases[0]
	if got := purchaseRequestText(l); got != "wants to buy 8 items at The Gilded Cup for 35 gp" {
		t.Errorf("row text = %q", got)
	}
	if l.RequesterName != "A player" || l.BuyerName != "Thorin" {
		t.Errorf("names = %q / %q", l.RequesterName, l.BuyerName)
	}

	// Players never get the Owner's list.
	pv, err := stash.StashesPage(context.Background(), "camp", pU1)
	if err != nil || len(pv.PendingPurchases) != 0 {
		t.Fatalf("player page: %v %d", err, len(pv.PendingPurchases))
	}

	// The requester sees the request, then its outcome, in their history.
	panel, err := stash.CharacterPanel(context.Background(), "camp", pU1, "c1")
	if err != nil || len(panel.History) != 1 || panel.History[0].Status != PurchasePending {
		t.Fatalf("panel = %+v err %v", panel, err)
	}
	if got := MoveSummary(panel.History[0]); got != "A player asked to buy 8 items at The Gilded Cup. Waiting for the GM." {
		t.Errorf("summary = %q", got)
	}
	f.fields.data["c1"]["gp"] = 1.0
	if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
		t.Fatal(err)
	}
	hist, err := stash.CharacterHistory(context.Background(), "camp", pU1, "c1")
	if err != nil || len(hist) != 1 {
		t.Fatalf("history = %+v err %v", hist, err)
	}
	if got := MoveSummary(hist[0]); got != "A player's purchase of 8 items at The Gilded Cup didn't go through. Not enough coin" {
		t.Errorf("summary = %q", got)
	}
	// Another player's character history never shows this one's request.
	if _, err := stash.CharacterHistory(context.Background(), "camp", pU2, "c1"); code(err) != 404 {
		t.Errorf("foreign history: %v", err)
	}
	// And the Owner's page no longer lists it.
	view, _ = stash.StashesPage(context.Background(), "camp", owner)
	if len(view.PendingPurchases) != 1 {
		t.Errorf("pending after answer = %d", len(view.PendingPurchases))
	}
}

// A price rise after the ask shows the quote with a warning, never the new
// total as if the player had agreed to it.
func TestShopRequest_PriceRoseRow(t *testing.T) {
	f := newBuyFx()
	stash := f.fx.svc.(*stashService)
	f.ask(t, pU1, basket("c1", [2]int{1, 2})) // 20 gp
	f.listings.rels[1].Metadata = []byte(`{"price":15,"currency":"gp","quantity":5}`)
	view, err := stash.StashesPage(context.Background(), "camp", owner)
	if err != nil || len(view.PendingPurchases) != 1 {
		t.Fatalf("page: %v", err)
	}
	want := "wants to buy 2 items at The Gilded Cup for 20 gp, but the price has gone up since they asked"
	if got := purchaseRequestText(view.PendingPurchases[0]); got != want {
		t.Errorf("row text = %q", got)
	}
}

// Purchase requests share the web history but stay out of the sync API, whose
// ids are move ids.
func TestAPILines_SkipsPurchaseRows(t *testing.T) {
	in := []MoveLine{
		{Move: Move{ID: 3, Kind: MoveKindItem, Quantity: 1}},
		{Move: Move{ID: 9}},
	}
	out := apiLines(in)
	if len(out) != 1 || out[0].ID != 3 {
		t.Fatalf("lines = %+v", out)
	}
}

func TestPurchaseSummary(t *testing.T) {
	cases := []struct {
		name string
		req  PurchaseRequest
		want string
	}{
		{"approved by the GM", PurchaseRequest{Status: PurchaseApplied, RequestedBy: "u1", DecidedBy: "gm", Basket: []BuyItemInput{{Quantity: 1}}},
			"The GM approved Kaela's request to buy 1 item at The Cup."},
		{"swept", PurchaseRequest{Status: PurchaseApplied, RequestedBy: "u1", DecidedBy: "gm", Reason: autoAppliedReason, Basket: []BuyItemInput{{Quantity: 2}}},
			"Kaela bought 2 items at The Cup. " + autoAppliedReason},
		{"declined", PurchaseRequest{Status: PurchaseDeclined, Basket: []BuyItemInput{{Quantity: 2}}},
			"The GM turned down Kaela's request to buy 2 items at The Cup."},
	}
	for _, tc := range cases {
		if got := purchaseSummary(tc.req, "Kaela", "The Cup"); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestShopRequestHandler(t *testing.T) {
	cases := []struct {
		name       string
		call       func(*ShopBuyHandler, echo.Context) error
		rid        string
		htmx       bool
		status     string
		err        error
		wantCode   int
		wantNotify string
		wantID     int64
	}{
		{"approve", (*ShopBuyHandler).ApproveRequest, "7", true, PurchaseApplied, nil, 204, `"type":"success"`, 7},
		{"approve that failed", (*ShopBuyHandler).ApproveRequest, "7", true, PurchaseFailed, nil, 204, `"type":"error"`, 7},
		{"decline", (*ShopBuyHandler).DeclineRequest, "8", true, "", nil, 204, `"type":"success"`, 8},
		{"approve without htmx redirects", (*ShopBuyHandler).ApproveRequest, "7", false, PurchaseApplied, nil, 303, "", 7},
		{"withdraw", (*ShopBuyHandler).WithdrawRequest, "9", true, "", nil, 204, `"type":"success"`, 9},
		{"withdraw conflict passes through", (*ShopBuyHandler).WithdrawRequest, "9", true, "", apperror.NewConflict("done"), 409, "", 9},
		{"bad id", (*ShopBuyHandler).ApproveRequest, "abc", true, "", nil, 404, "", 0},
		{"zero id", (*ShopBuyHandler).DeclineRequest, "0", true, "", nil, 404, "", 0},
		{"service refusal passes through", (*ShopBuyHandler).ApproveRequest, "7", true, "", apperror.NewForbidden("no"), 403, "", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeBuySvc{err: tc.err, answerStatus: tc.status}
			h := NewShopBuyHandler(svc)
			c, rec := roomCtx(http.MethodPost, "", campaigns.RolePlayer)
			c.SetParamNames("id", "rid")
			c.SetParamValues("camp-1", tc.rid)
			if tc.htmx {
				c.Request().Header.Set("HX-Request", "true")
			}
			err := tc.call(h, c)
			got := rec.Code
			if err != nil {
				got = code(err)
			}
			if got != tc.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tc.wantCode)
			}
			if svc.answered != tc.wantID {
				t.Errorf("answered request %d, want %d", svc.answered, tc.wantID)
			}
			if tc.wantNotify != "" && !strings.Contains(rec.Header().Get("HX-Trigger"), tc.wantNotify) {
				t.Errorf("HX-Trigger = %q, want %s", rec.Header().Get("HX-Trigger"), tc.wantNotify)
			}
		})
	}
	t.Run("answers as the caller's visibility role", func(t *testing.T) {
		svc := &fakeBuySvc{}
		c, _ := roomCtx(http.MethodPost, "", campaigns.RolePlayer)
		c.SetParamNames("id", "rid")
		c.SetParamValues("camp-1", "3")
		_ = NewShopBuyHandler(svc).DeclineRequest(c)
		if svc.actor.Role != rPlayer {
			t.Errorf("role = %d", svc.actor.Role)
		}
	})
}

func TestShopRequest_Withdraw(t *testing.T) {
	scribe := Actor{UserID: "s", Role: rScribe}
	cases := []struct {
		name     string
		actor    Actor
		campaign string
		prep     func(*buyFx, int64)
		wantCode int
		wantGone bool
	}{
		{"requester withdraws", pU1, "camp", nil, 0, true},
		{"owner withdraws", owner, "camp", nil, 0, true},
		{"other player is forbidden", pU2, "camp", nil, 403, false},
		{"scribe is forbidden", scribe, "camp", nil, 403, false},
		{"already approved", pU1, "camp", func(f *buyFx, id int64) {
			if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
				panic(err)
			}
		}, 409, false},
		{"already declined", pU1, "camp", func(f *buyFx, id int64) {
			if _, err := f.svc.DeclineRequest(context.Background(), "camp", owner, id); err != nil {
				panic(err)
			}
		}, 409, false},
		{"wrong campaign", pU1, "other-camp", nil, 404, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
			if tc.prep != nil {
				tc.prep(f, id)
			}
			err := f.svc.WithdrawRequest(context.Background(), tc.campaign, tc.actor, id)
			if code(err) != tc.wantCode {
				t.Fatalf("err = %v, want code %d", err, tc.wantCode)
			}
			if gone := f.reqs.find("camp", id) == nil; gone != tc.wantGone {
				t.Errorf("row gone = %v, want %v", gone, tc.wantGone)
			}
		})
	}

	t.Run("withdrawn request frees the player's slot and leaves history", func(t *testing.T) {
		f := newBuyFx()
		id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
		if err := f.svc.WithdrawRequest(context.Background(), "camp", pU1, id); err != nil {
			t.Fatal(err)
		}
		if n, _ := f.reqs.CountPendingBy(context.Background(), "camp", "u1"); n != 0 {
			t.Errorf("pending = %d", n)
		}
		if err := f.svc.WithdrawRequest(context.Background(), "camp", pU1, id); code(err) != 404 {
			t.Errorf("second withdraw: %v", err)
		}
	})
}

func TestShopRequest_HistoryWithdrawFlag(t *testing.T) {
	cases := []struct {
		name  string
		actor Actor
		want  bool
	}{
		{"requester", pU1, true},
		{"owner", owner, true},
		{"other player sees no waiting line", pU2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			f.ask(t, pU1, basket("c1", [2]int{1, 1}))
			lines, err := f.svc.(*shopBuyService).buyerHistory(context.Background(), "camp", tc.actor, "c1", 10)
			if err != nil {
				t.Fatal(err)
			}
			got := len(lines) == 1 && lines[0].CanWithdraw && lines[0].Purchase
			if got != tc.want {
				t.Errorf("lines = %+v, want withdrawable %v", lines, tc.want)
			}
		})
	}
}
