package armory

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// purseSheet gives c1 a five-coin 5e purse.
func (f *buyFx) purseSheet(cp, sp, ep, gp, pp float64) {
	f.dir.ents["c1"].Purse = map[string]string{"cp": "cp", "sp": "sp", "ep": "ep", "gp": "gp", "pp": "pp"}
	f.fields.data["c1"] = map[string]any{"cp": cp, "sp": sp, "ep": ep, "gp": gp, "pp": pp}
}

// coins reads the purse fields as numbers.
func (f *buyFx) coins(id string) map[string]float64 {
	out := map[string]float64{}
	for _, k := range []string{"cp", "sp", "ep", "gp", "pp"} {
		if v, ok := f.fields.data[id][k].(float64); ok {
			out[k] = v
		}
	}
	return out
}

func (f *buyFx) wealthSheet(w float64) {
	f.dir.ents["c1"].MoneyKey = "wealth"
	f.fields.data["c1"] = map[string]any{"wealth": w}
}

func TestShopBuy_Purse(t *testing.T) {
	cases := []struct {
		name       string
		purse      [5]float64 // cp sp ep gp pp
		in         BuyInput
		wantCoins  map[string]float64
		wantLeft   string
		wantChange string
		wantErr    string
	}{
		{"five silver breaks a gold into change", [5]float64{3, 2, 0, 10, 0}, basket("c1", [2]int{11, 1}),
			map[string]float64{"cp": 3, "sp": 7, "ep": 0, "gp": 9, "pp": 0}, "9 gp 7 sp 3 cp", "7 sp 3 cp", ""},
		{"exact copper", [5]float64{5, 0, 0, 1, 0}, basket("c1", [2]int{12, 5}),
			map[string]float64{"cp": 0, "sp": 0, "ep": 0, "gp": 1, "pp": 0}, "1 gp", "", ""},
		{"gold price spends smaller coins first", [5]float64{3, 2, 0, 10, 0}, basket("c1", [2]int{1, 1}),
			map[string]float64{"cp": 3, "sp": 2, "ep": 0, "gp": 0, "pp": 0}, "2 sp 3 cp", "2 sp 3 cp", ""},
		{"platinum is broken into gold, silver and copper", [5]float64{0, 0, 0, 0, 1}, basket("c1", [2]int{12, 1}),
			map[string]float64{"cp": 9, "sp": 9, "ep": 0, "gp": 9, "pp": 0}, "9 gp 9 sp 9 cp", "9 gp 9 sp 9 cp", ""},
		{"electrum pays but is not given back", [5]float64{0, 0, 1, 0, 0}, basket("c1", [2]int{12, 9}),
			map[string]float64{"cp": 1, "sp": 4, "ep": 0, "gp": 0, "pp": 0}, "4 sp 1 cp", "4 sp 1 cp", ""},
		{"short of the price", [5]float64{3, 2, 0, 10, 0}, basket("c1", [2]int{1, 2}),
			map[string]float64{"cp": 3, "sp": 2, "ep": 0, "gp": 10, "pp": 0}, "", "", "Not enough coin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			f.openDowntime()
			f.listings.rels[11] = sells("shop-1", "i1", `{"price":5,"currency":"sp","quantity":5}`)
			f.listings.rels[12] = sells("shop-1", "i1", `{"price":1,"currency":"CP","quantity":9}`)
			p := tc.purse
			f.purseSheet(p[0], p[1], p[2], p[3], p[4])

			res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, tc.in)
			if tc.wantErr != "" {
				if err == nil || code(err) != 400 || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want 400 %q", err, tc.wantErr)
				}
				if got := f.coins("c1"); !reflect.DeepEqual(got, tc.wantCoins) {
					t.Errorf("refusal changed the purse: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := f.coins("c1"); !reflect.DeepEqual(got, tc.wantCoins) {
				t.Errorf("coins = %v, want %v", got, tc.wantCoins)
			}
			if res.PurseLeft != tc.wantLeft || res.Change != tc.wantChange {
				t.Errorf("purseLeft %q change %q, want %q / %q", res.PurseLeft, res.Change, tc.wantLeft, tc.wantChange)
			}
			if len(f.txRows) != len(tc.in.Items) {
				t.Errorf("transactions = %d", len(f.txRows))
			}
		})
	}
}

func TestShopBuy_SilverPriceOnGoldOnlySheet(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	// Listing 3 is 1 sp each: five of them cost 5 sp = half a gold, not 5 gp.
	res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{3, 5}))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.money("c1").(float64); got != 49.5 {
		t.Errorf("gp = %v, want 49.5", f.money("c1"))
	}
	if *res.MoneyLeft != 49.5 || res.Currency != "sp" || *res.Spent != 5 {
		t.Errorf("result = %+v", res)
	}
	t.Run("a fraction of a copper rounds up", func(t *testing.T) {
		f := newBuyFx()
		f.openDowntime()
		f.listings.rels[13] = sells("shop-1", "i1", `{"price":0.5,"currency":"cp","quantity":5}`)
		if _, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{13, 1})); err != nil {
			t.Fatal(err)
		}
		if got, _ := f.money("c1").(float64); got != 49.99 {
			t.Errorf("gp = %v, want 49.99 (1 cp)", f.money("c1"))
		}
	})
	t.Run("a currency that is not a coin is charged at face value", func(t *testing.T) {
		f := newBuyFx()
		f.openDowntime()
		f.listings.rels[14] = sells("shop-1", "i1", `{"price":2,"currency":"credits","quantity":5}`)
		if _, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{14, 1})); err != nil {
			t.Fatal(err)
		}
		if got, _ := f.money("c1").(float64); got != 48 {
			t.Errorf("gp = %v, want 48", f.money("c1"))
		}
	})
}

func TestShopBuy_PurseRollbackRestoresExactly(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	f.listings.rels[11] = sells("shop-1", "i1", `{"price":5,"currency":"sp","quantity":5}`)
	f.purseSheet(3, 2, 0, 10, 0)
	f.rels.fail = true // the new line for i2 cannot be created, after the coins were taken
	_, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{11, 1}, [2]int{2, 1}))
	if err == nil {
		t.Fatal("want failure")
	}
	want := map[string]float64{"cp": 3, "sp": 2, "ep": 0, "gp": 10, "pp": 0}
	if got := f.coins("c1"); !reflect.DeepEqual(got, want) {
		t.Errorf("purse = %v, want %v restored", got, want)
	}
	if f.listings.stock(11) != 5 || len(f.txRows) != 0 {
		t.Errorf("stock %d rows %d", f.listings.stock(11), len(f.txRows))
	}
}

func TestShopBuy_Wealth(t *testing.T) {
	cases := []struct {
		name   string
		wealth float64
		in     BuyInput
	}{
		{"wealth equal to the price", 10, basket("c1", [2]int{1, 1})},
		{"wealth above the price", 12, basket("c1", [2]int{1, 3})},
		{"only the dearest unit counts, not the total", 10, basket("c1", [2]int{1, 1}, [2]int{2, 4})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newBuyFx()
			f.openDowntime()
			f.wealthSheet(tc.wealth)
			stock := f.listings.stock(1)

			res, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != BuyStatusBought || res.Spent != nil || res.Currency != "" || res.MoneyLeft == nil || *res.MoneyLeft != tc.wealth {
				t.Errorf("result = %+v", res)
			}
			if got, _ := f.fields.data["c1"]["wealth"].(float64); got != tc.wealth || len(f.fields.data["c1"]) != 1 {
				t.Errorf("sheet = %v, Wealth must not change", f.fields.data["c1"])
			}
			if f.carried("c1", "i1") < 3+1 || len(f.txRows) != len(tc.in.Items) {
				t.Errorf("held %d rows %d", f.carried("c1", "i1"), len(f.txRows))
			}
			if f.listings.stock(1) != stock-tc.in.Items[0].Quantity {
				t.Errorf("stock = %d", f.listings.stock(1))
			}
		})
	}
}

func TestShopBuy_WealthRollbackWritesNoMoney(t *testing.T) {
	f := newBuyFx()
	f.openDowntime()
	f.wealthSheet(10)
	f.rels.fail = true
	if _, err := f.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}, [2]int{2, 1})); err == nil {
		t.Fatal("want failure")
	}
	if got := f.fields.data["c1"]; len(got) != 1 || got["wealth"] != 10.0 || f.listings.stock(1) != 5 {
		t.Errorf("sheet %v stock %d", got, f.listings.stock(1))
	}
}

func TestShopRequest_ApproveWithPurseAndWealth(t *testing.T) {
	t.Run("purse buyer pays and gets change on approval", func(t *testing.T) {
		f := newBuyFx()
		f.listings.rels[11] = sells("shop-1", "i1", `{"price":5,"currency":"sp","quantity":5}`)
		f.purseSheet(3, 2, 0, 10, 0)
		id := f.ask(t, pU1, basket("c1", [2]int{11, 1}))
		if got := f.coins("c1"); got["gp"] != 10 {
			t.Fatalf("queueing spent coins: %v", got)
		}
		if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
			t.Fatal(err)
		}
		want := map[string]float64{"cp": 3, "sp": 7, "ep": 0, "gp": 9, "pp": 0}
		if got := f.coins("c1"); !reflect.DeepEqual(got, want) {
			t.Errorf("coins = %v, want %v", got, want)
		}
		if f.reqs.status(id) != PurchaseApplied {
			t.Errorf("status = %q", f.reqs.status(id))
		}
	})
	t.Run("a purse that can no longer pay fails the request", func(t *testing.T) {
		f := newBuyFx()
		f.listings.rels[11] = sells("shop-1", "i1", `{"price":5,"currency":"sp","quantity":5}`)
		f.purseSheet(3, 2, 0, 10, 0)
		id := f.ask(t, pU1, basket("c1", [2]int{11, 1}))
		f.purseSheet(1, 0, 0, 0, 0)
		if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
			t.Fatal(err)
		}
		if f.reqs.status(id) != PurchaseFailed || f.coins("c1")["cp"] != 1 {
			t.Errorf("status %q coins %v", f.reqs.status(id), f.coins("c1"))
		}
	})
	t.Run("wealth buyer is approved without a money write", func(t *testing.T) {
		f := newBuyFx()
		f.wealthSheet(10)
		id := f.ask(t, pU1, basket("c1", [2]int{1, 2}))
		if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
			t.Fatal(err)
		}
		if f.reqs.status(id) != PurchaseApplied || f.fields.data["c1"]["wealth"] != 10.0 || f.carried("c1", "i1") != 5 {
			t.Errorf("status %q sheet %v held %d", f.reqs.status(id), f.fields.data["c1"], f.carried("c1", "i1"))
		}
	})
	t.Run("wealth that fell below the price fails the request", func(t *testing.T) {
		f := newBuyFx()
		f.wealthSheet(10)
		id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
		f.wealthSheet(4)
		if _, err := f.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
			t.Fatal(err)
		}
		if f.reqs.status(id) != PurchaseFailed || !strings.Contains(f.reqs.reason(id), "Needs Wealth 10") {
			t.Errorf("status %q reason %q", f.reqs.status(id), f.reqs.reason(id))
		}
	})
}

func TestShopBuy_BuyersReportHowEachSheetPays(t *testing.T) {
	f := newBuyFx()
	f.purseSheet(3, 2, 0, 10, 0)
	f.dir.ents["c2"].MoneyKey = "wealth"
	f.fields.data["c2"] = map[string]any{"wealth": 3.0}
	v, err := f.svc.Buyers(context.Background(), "camp", "shop-1", owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	for _, want := range []string{
		`"id":"c1","name":"Thorin","moneyKey":"gp","money":10.23,"purse":{"cp":3,"ep":0,"gp":10,"pp":0,"sp":2},"moneyCp":1023,"kind":"purse"`,
		`"id":"c2","name":"Mira","moneyKey":"wealth","money":3,"kind":"wealth"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s lacks %s", raw, want)
		}
	}
}
