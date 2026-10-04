package armory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeShopRels is an in-memory shop listing store that is both the relation
// finder and the compare-and-set metadata updater, like the real relations
// service.
type fakeShopRels struct {
	mu   sync.Mutex
	rels map[int]*RelationInfo
}

func (f *fakeShopRels) GetByID(_ context.Context, id int) (*RelationInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rels[id]
	if !ok {
		return nil, apperror.NewNotFound("relation not found")
	}
	c := *r
	c.Metadata = append(json.RawMessage(nil), r.Metadata...)
	return &c, nil
}

func (f *fakeShopRels) UpdateMetadata(_ context.Context, id int, m json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rels[id].Metadata = m
	return nil
}

func (f *fakeShopRels) UpdateMetadataIf(_ context.Context, id int, exp, m json.RawMessage) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.rels[id]
	if string(r.Metadata) != string(exp) {
		return false, nil
	}
	r.Metadata = m
	return true, nil
}

// stock reads the listing's remaining quantity; -1 is unlimited.
func (f *fakeShopRels) stock(id int) int { return parseShopMeta(f.rels[id].Metadata).Quantity }

type buyShops struct{}

func (buyShops) IsShopInCampaign(_ context.Context, campaignID, id string) (bool, error) {
	return campaignID == "camp" && (id == "shop-1" || id == "shop-2"), nil
}

type buyFx struct {
	*fx
	listings *fakeShopRels
	txRows   []*Transaction
	svc      ShopRequestService
	reqs     *fakePurchaseRepo
}

func sells(source, target, meta string) *RelationInfo {
	return &RelationInfo{CampaignID: "camp", SourceEntityID: source, TargetEntityID: target, RelationType: "sells", Metadata: json.RawMessage(meta)}
}

func newBuyFx() *buyFx {
	f := &buyFx{fx: newFx()}
	f.listings = &fakeShopRels{rels: map[int]*RelationInfo{
		1: sells("shop-1", "i1", `{"price":10,"currency":"gp","quantity":5,"note":"keep me"}`),
		2: sells("shop-1", "i2", `{"price":2.5,"currency":"gp"}`),
		3: sells("shop-1", "i1", `{"price":1,"currency":"sp","quantity":5}`),
		4: sells("shop-2", "i1", `{"price":1,"currency":"gp","quantity":5}`),
		6: sells("shop-1", "i1", `{"price":5,"currency":"gp","quantity":0}`),
		7: sells("shop-1", "i1", `{"price":5,"currency":"gp","quantity":5}`),
		8: sells("shop-1", "i1", `{"quantity":5}`),
		9: sells("shop-1", "i1", `{"price":1,"quantity":5}`),
	}}
	f.listings.rels[5] = sells("shop-1", "i1", `{"price":1,"quantity":5}`)
	f.listings.rels[5].RelationType = "owns"
	f.listings.rels[7].DmOnly = true
	f.listings.rels[10] = sells("shop-1", "i1", `{"price":1,"quantity":5}`)
	f.listings.rels[10].CampaignID = "other"

	txSvc := NewTransactionService(&mockTransactionRepo{createFn: func(_ context.Context, tx *Transaction) error {
		c := *tx
		f.txRows = append(f.txRows, &c)
		return nil
	}})
	txSvc.SetRelationFinder(f.listings)
	txSvc.SetRelationMetadataUpdater(f.listings)
	f.reqs = newFakePurchaseRepo()
	f.dir.ents["shop-1"] = &EntityRef{ID: "shop-1", Name: "The Gilded Cup"}
	f.svc = NewShopBuyService(f.fx.svc.(*stashService), txSvc, buyShops{}, f.reqs)
	return f
}

var (
	pU1   = Actor{UserID: "u1", Role: rPlayer}
	pU2   = Actor{UserID: "u2", Role: rPlayer}
	pU3   = Actor{UserID: "u3", Role: rPlayer}
	owner = Actor{UserID: "gm", Role: rOwner}
)

func basket(buyer string, lines ...[2]int) BuyInput {
	in := BuyInput{BuyerEntityID: buyer}
	for _, l := range lines {
		in.Items = append(in.Items, BuyItemInput{RelationID: RelationRef(l[0]), Quantity: l[1]})
	}
	return in
}

func (f *buyFx) openDowntime() { f.repo.downtime = true }

func (f *buyFx) money(id string) any { return f.fields.data[id]["gp"] }

// carried returns how many of an item the character holds.
func (f *buyFx) carried(char, item string) int {
	for _, r := range f.rels.rels[char] {
		if r.ItemEntityID == item {
			q, _ := parseCarried(r.Metadata)
			return q
		}
	}
	return 0
}

func TestShopBuy_Refusals(t *testing.T) {
	cases := []struct {
		name     string
		actor    Actor
		downtime bool
		in       BuyInput
		prep     func(*buyFx)
		wantCode int
		wantMsg  string
	}{
		{"not enough coin", pU1, true, basket("c1", [2]int{1, 6}), nil, 400, "Not enough coin"},
		{"mixed currency", pU1, true, basket("c1", [2]int{1, 1}, [2]int{3, 1}), nil, 400, "Items are priced in different currencies"},
		{"relation of another shop", pU1, true, basket("c1", [2]int{4, 1}), nil, 404, ""},
		{"relation in another campaign", pU1, true, basket("c1", [2]int{10, 1}), nil, 404, ""},
		{"not a sells relation", pU1, true, basket("c1", [2]int{5, 1}), nil, 404, ""},
		{"unknown relation", pU1, true, basket("c1", [2]int{99, 1}), nil, 404, ""},
		{"dm-only listing hidden from player", pU1, true, basket("c1", [2]int{7, 1}), nil, 404, ""},
		{"good the player cannot see", pU1, true, basket("c1", [2]int{2, 1}), func(f *buyFx) { f.vis.hidden["i2"] = true }, 404, ""},
		{"shop the player cannot see", pU1, true, basket("c1", [2]int{1, 1}), func(f *buyFx) { f.vis.hidden["shop-1"] = true }, 404, ""},
		{"player buying for someone else", pU1, true, basket("c2", [2]int{1, 1}), nil, 403, ""},
		{"scribe buying for someone else's character", Actor{UserID: "s", Role: rScribe}, true, basket("c2", [2]int{1, 1}), nil, 403, ""},
		{"buyer not a character", pU1, true, basket("i1", [2]int{1, 1}), nil, 404, ""},
		{"buyer in no campaign", pU1, true, basket("ghost", [2]int{1, 1}), nil, 404, ""},
		{"sheet without coin field", pU3, true, basket("c3", [2]int{1, 1}), nil, 400, "This sheet has no coin field"},
		{"wealth is never spent like coins", pU1, true, basket("c1", [2]int{1, 1}), func(f *buyFx) { f.dir.ents["c1"].MoneyKey = "wealth"; f.fields.data["c1"]["wealth"] = 50.0 }, 400, "Wealth isn’t spent like coins"},
		{"sold out", pU1, true, basket("c1", [2]int{6, 1}), nil, 400, "stock"},
		{"more than in stock", pU1, true, basket("c1", [2]int{1, 6}), func(f *buyFx) { f.fields.data["c1"]["gp"] = 5000.0 }, 400, "insufficient stock"},
		{"unpriced good", pU1, true, basket("c1", [2]int{8, 1}), nil, 400, "no price"},
		{"empty basket", pU1, true, BuyInput{BuyerEntityID: "c1"}, nil, 400, "basket is empty"},
		{"no buyer", pU1, true, basket("", [2]int{1, 1}), nil, 400, "Choose who"},
		{"quantity zero", pU1, true, basket("c1", [2]int{1, 0}), nil, 400, "quantity"},
		{"quantity over cap", pU1, true, basket("c1", [2]int{1, 100}), nil, 400, "quantity"},
		{"same good twice", pU1, true, basket("c1", [2]int{1, 1}, [2]int{1, 1}), nil, 400, "only appear once"},
		{"relation id zero", pU1, true, basket("c1", [2]int{0, 1}), nil, 400, "not valid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			f.repo.downtime = tc.downtime
			if tc.prep != nil {
				tc.prep(f)
			}
			stock := map[int]int{}
			for id := range f.listings.rels {
				stock[id] = f.listings.stock(id)
			}
			gp1, gp2 := f.money("c1"), f.money("c2")
			held := f.carried("c1", "i1")

			res, err := f.svc.Buy(context.Background(), "camp", "shop-1", tc.actor, tc.in)
			if err == nil {
				t.Fatalf("want refusal, got %+v", res)
			}
			if code(err) != tc.wantCode {
				t.Fatalf("code = %d (%v), want %d", code(err), err, tc.wantCode)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("message %q lacks %q", err.Error(), tc.wantMsg)
			}
			// A refusal leaves stock, coins, items and the log untouched.
			for id, want := range stock {
				if got := f.listings.stock(id); got != want {
					t.Errorf("stock of listing %d = %d, want %d", id, got, want)
				}
			}
			if f.money("c1") != gp1 || f.money("c2") != gp2 {
				t.Errorf("coins changed: c1 %v->%v c2 %v->%v", gp1, f.money("c1"), gp2, f.money("c2"))
			}
			if f.carried("c1", "i1") != held {
				t.Errorf("items changed")
			}
			if len(f.txRows) != 0 {
				t.Errorf("recorded %d transactions", len(f.txRows))
			}
		})
	}
}

func TestShopBuy_Success(t *testing.T) {
	cases := []struct {
		name      string
		actor     Actor
		downtime  bool
		buyer     string
		in        BuyInput
		wantSpent float64
		wantLeft  float64
		wantMoney float64
	}{
		{"player, downtime open, two goods", pU1, true, "c1", basket("c1", [2]int{1, 2}, [2]int{2, 3}), 27.5, 22.5, 22.5},
		{"owner buys for any character while closed", owner, false, "c2", basket("c2", [2]int{1, 1}), 10, 0, 0},
		{"string numeral coin balance", pU2, true, "c2", basket("c2", [2]int{2, 4}), 10, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			f.repo.downtime = tc.downtime
			stock1, stock2 := f.listings.stock(1), f.listings.stock(2)

			res, err := f.svc.Buy(context.Background(), "camp", "shop-1", tc.actor, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != "bought" || res.Currency != "gp" || *res.Spent != tc.wantSpent || *res.MoneyLeft != tc.wantLeft {
				t.Errorf("result = %+v spent=%v left=%v", res, *res.Spent, *res.MoneyLeft)
			}
			// Deducted exactly once, as a number, and only the money key changed.
			if got, ok := f.money(tc.buyer).(float64); !ok || got != tc.wantMoney {
				t.Errorf("coins = %#v, want %v", f.money(tc.buyer), tc.wantMoney)
			}
			if len(f.fields.data[tc.buyer]) != 1 {
				t.Errorf("fields = %v, only the money key may be written", f.fields.data[tc.buyer])
			}
			for _, it := range tc.in.Items {
				if got, want := f.carried(tc.buyer, f.listings.rels[int(it.RelationID)].TargetEntityID), it.Quantity; got < want {
					t.Errorf("holds %d, want at least %d", got, want)
				}
			}
			if len(f.txRows) != len(tc.in.Items) {
				t.Fatalf("transactions = %d, want %d", len(f.txRows), len(tc.in.Items))
			}
			for _, row := range f.txRows {
				if row.TransactionType != TxPurchase || row.BuyerEntityID == nil || *row.BuyerEntityID != tc.buyer || row.ShopEntityID != "shop-1" {
					t.Errorf("bad transaction row %+v", row)
				}
			}
			for _, it := range tc.in.Items {
				if it.RelationID == 1 && f.listings.stock(1) != stock1-it.Quantity {
					t.Errorf("stock 1 = %d", f.listings.stock(1))
				}
			}
			if f.listings.stock(2) != stock2 {
				t.Errorf("unlimited listing changed to %d", f.listings.stock(2))
			}
			// Untouched listing fields survive a stock write.
			if !strings.Contains(string(f.listings.rels[1].Metadata), "keep me") {
				t.Errorf("listing metadata lost a field: %s", f.listings.rels[1].Metadata)
			}
		})
	}
}

func TestShopBuy_PriceComesFromListing(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	// Extra client fields, including a price, are ignored.
	var in BuyInput
	if err := json.Unmarshal([]byte(`{"buyerEntityId":"c1","price":0.01,"items":[{"relationId":"1","quantity":1,"price":0.01}]}`), &in); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, in)
	if err != nil || *res.Spent != 10 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestShopBuy_LaterFailureRollsEverythingBack(t *testing.T) {
	t.Run("later good sold out gives stock back", func(t *testing.T) {
		f := newBuyFx()
		f.openDowntime()
		_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 2}, [2]int{6, 1}))
		if code(err) != 400 {
			t.Fatalf("err = %v", err)
		}
		if f.listings.stock(1) != 5 {
			t.Errorf("first listing stock = %d, want the 5 it had", f.listings.stock(1))
		}
		if f.money("c1") != 50.0 || len(f.txRows) != 0 || f.carried("c1", "i1") != 3 {
			t.Errorf("side effects: money %v rows %d held %d", f.money("c1"), len(f.txRows), f.carried("c1", "i1"))
		}
	})
	t.Run("item credit fails after coins were taken", func(t *testing.T) {
		f := newBuyFx()
		f.openDowntime()
		f.rels.fail = true // the new line for i2 cannot be created
		_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}, [2]int{2, 1}))
		if err == nil {
			t.Fatal("want failure")
		}
		if f.listings.stock(1) != 5 {
			t.Errorf("stock = %d, want 5", f.listings.stock(1))
		}
		if f.money("c1") != 50.0 {
			t.Errorf("coins = %v, want 50 restored", f.money("c1"))
		}
		if f.carried("c1", "i1") != 3 {
			t.Errorf("held i1 = %d, want the original 3 (first credit undone)", f.carried("c1", "i1"))
		}
		if len(f.txRows) != 0 {
			t.Errorf("transactions = %d", len(f.txRows))
		}
	})
	t.Run("listing repriced between check and reserve", func(t *testing.T) {
		f := newBuyFx()
		f.openDowntime()
		// The finder sees the old price once, then the editor raises it.
		calls := 0
		orig := f.listings.rels[1].Metadata
		finder := &repriceFinder{fakeShopRels: f.listings, after: func() {
			calls++
			if calls == 1 {
				f.listings.rels[1].Metadata = json.RawMessage(strings.Replace(string(orig), `"price":10`, `"price":40`, 1))
			}
		}}
		f.tx().SetRelationFinder(finder)
		_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}))
		if code(err) != 409 {
			t.Fatalf("err = %v", err)
		}
		if f.listings.stock(1) != 5 || f.money("c1") != 50.0 || len(f.txRows) != 0 {
			t.Errorf("not rolled back: stock %d money %v rows %d", f.listings.stock(1), f.money("c1"), len(f.txRows))
		}
	})
}

// repriceFinder runs a hook after its first lookup returns.
type repriceFinder struct {
	*fakeShopRels
	after func()
}

func (r *repriceFinder) GetByID(ctx context.Context, id int) (*RelationInfo, error) {
	rel, err := r.fakeShopRels.GetByID(ctx, id)
	r.after()
	return rel, err
}

func (f *buyFx) tx() *transactionService { return f.svc.(*shopBuyService).tx }

func TestShopBuy_ConcurrentBasketsSpendCoinsOnce(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	f.fields.data["c1"]["gp"] = 10.0 // exactly one 10 gp good

	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	won := 0
	for err := range results {
		if err == nil {
			won++
		} else if code(err) != 400 {
			t.Errorf("unexpected error %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d baskets succeeded, want exactly 1", won)
	}
	if f.money("c1") != 0.0 || f.listings.stock(1) != 4 || len(f.txRows) != 1 {
		t.Errorf("money %v stock %d rows %d", f.money("c1"), f.listings.stock(1), len(f.txRows))
	}
}

func TestShopBuy_FractionalPricesAreExact(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	f.listings.rels[2].Metadata = json.RawMessage(`{"price":0.1,"currency":"gp"}`)
	f.fields.data["c1"]["gp"] = 1.0
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{2, 3}))
	if err != nil {
		t.Fatal(err)
	}
	if *res.Spent != 0.3 || *res.MoneyLeft != 0.7 || f.money("c1") != 0.7 {
		t.Errorf("spent %v left %v sheet %v", *res.Spent, *res.MoneyLeft, f.money("c1"))
	}
}

// A sub-cent price rounds to whole cents per unit; buying two must not be
// mistaken for a price change mid-purchase.
func TestShopBuy_SubCentPriceBuysInQuantity(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	f.listings.rels[2].Metadata = json.RawMessage(`{"price":0.125,"currency":"gp"}`)
	f.fields.data["c1"]["gp"] = 1.0
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{2, 2}))
	if err != nil {
		t.Fatal(err)
	}
	if *res.Spent != 0.26 || f.money("c1") != 0.74 {
		t.Errorf("spent %v sheet %v", *res.Spent, f.money("c1"))
	}
}

func TestShopBuy_Buyers(t *testing.T) {
	cases := []struct {
		name     string
		actor    Actor
		downtime bool
		prep     func(*buyFx)
		wantIDs  []string
		wantNow  bool
		wantCode int
	}{
		{"player sees own character only, closed", pU1, false, nil, []string{"c1"}, false, 0},
		{"player, downtime open", pU1, true, nil, []string{"c1"}, true, 0},
		{"owner sees every character, closed", owner, false, nil, []string{"c3", "c2", "c1"}, true, 0},
		{"player with no character", Actor{UserID: "nobody", Role: rPlayer}, true, nil, []string{}, true, 0},
		{"hidden shop is 404", pU1, true, func(f *buyFx) { f.vis.hidden["shop-1"] = true }, nil, false, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			f.repo.downtime = tc.downtime
			if tc.prep != nil {
				tc.prep(f)
			}
			v, err := f.svc.Buyers(context.Background(), "camp", "shop-1", tc.actor)
			if tc.wantCode != 0 {
				if code(err) != tc.wantCode {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, b := range v.Buyers {
				ids = append(ids, b.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("buyers = %v, want %v", ids, tc.wantIDs)
			}
			if v.DowntimeOpen != tc.downtime || v.CanBuyNow != tc.wantNow {
				t.Errorf("downtimeOpen=%v canBuyNow=%v", v.DowntimeOpen, v.CanBuyNow)
			}
		})
	}

	t.Run("money fields", func(t *testing.T) {
		f := newBuyFx()
		v, err := f.svc.Buyers(context.Background(), "camp", "shop-1", owner)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(v)
		got := string(raw)
		for _, want := range []string{
			`{"id":"c1","name":"Thorin","moneyKey":"gp","money":50}`,
			`{"id":"c2","name":"Mira","moneyKey":"gp","money":10}`,
			`{"id":"c3","name":"Grub","moneyKey":"","money":null}`,
			`"downtimeOpen":false`, `"canBuyNow":true`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("%s lacks %s", got, want)
			}
		}
	})
}

func TestRelationRef_Unmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want RelationRef
		bad  bool
	}{
		{`12`, 12, false}, {`"12"`, 12, false}, {`"x"`, 0, true}, {`1.5`, 0, true}, {`null`, 0, false}, {`true`, 0, true}, {`{}`, 0, true},
	}
	for _, tc := range cases {
		var r RelationRef
		err := json.Unmarshal([]byte(tc.in), &r)
		if (err != nil) != tc.bad || (!tc.bad && r != tc.want) {
			t.Errorf("%s -> %v, %v", tc.in, r, err)
		}
	}
}

func TestShopBuy_TooManyItems(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	in := BuyInput{BuyerEntityID: "c1"}
	for i := 1; i <= 51; i++ {
		in.Items = append(in.Items, BuyItemInput{RelationID: RelationRef(i), Quantity: 1})
	}
	_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, in)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
}
