package armory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Fakes for the stock service. Each records what it was asked to do so the
// tests can check what landed and what was rolled back.

type ssShops struct{ ok bool }

func (f ssShops) IsShopInCampaign(context.Context, string, string) (bool, error) { return f.ok, nil }

type ssPages struct {
	made    map[string]string // name -> id
	gone    map[string]bool   // item ids that no longer exist
	failOn  string            // name that fails to be made
	hidden  []bool
	counter int
}

func (f *ssPages) EnsureShopGood(_ context.Context, _, _ string, _ int, name string, hidden bool) (string, error) {
	if name == f.failOn {
		return "", errors.New("boom")
	}
	if f.made == nil {
		f.made = map[string]string{}
	}
	f.hidden = append(f.hidden, hidden)
	if id, ok := f.made[name]; ok {
		return id, nil
	}
	f.counter++
	id := "good-" + name
	f.made[name] = id
	return id, nil
}
func (f *ssPages) ItemPageExists(_ context.Context, _, id string) (bool, error) {
	return !f.gone[id], nil
}
func (f *ssPages) Names(_ context.Context, _ string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		out[id] = "name-" + id
	}
	return out, nil
}

type ssWriter struct {
	next    int
	created []ssCreated
	deleted []int
	failAt  int // 1-based create call that fails; 0 never
}
type ssCreated struct {
	shop, item string
	meta       map[string]any
	dmOnly     bool
}

func (f *ssWriter) CreateSells(_ context.Context, _, shop, item, _ string, meta json.RawMessage, dmOnly bool) (int, error) {
	if f.failAt > 0 && len(f.created)+1 == f.failAt {
		return 0, errors.New("write failed")
	}
	f.next++
	var m map[string]any
	_ = json.Unmarshal(meta, &m)
	f.created = append(f.created, ssCreated{shop, item, m, dmOnly})
	return 100 + f.next, nil
}
func (f *ssWriter) DeleteSells(_ context.Context, id int) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type ssListings struct{ rows []ShopListing }

func (f ssListings) ListSells(context.Context, string, string) ([]ShopListing, error) {
	return f.rows, nil
}

type ssFinder struct{ rels map[int]*RelationInfo }

func (f ssFinder) GetByID(_ context.Context, id int) (*RelationInfo, error) { return f.rels[id], nil }

type ssPrices struct {
	food   float64
	saved  float64
	priced map[string]ItemPrice
}

func (f *ssPrices) FoodPrice(context.Context, string) (float64, error) { return f.food, nil }
func (f *ssPrices) SaveFoodPrice(_ context.Context, _, _ string, p float64) error {
	f.saved = p
	return nil
}
func (f *ssPrices) ItemPrices(context.Context, string) (map[string]ItemPrice, error) {
	return f.priced, nil
}

type ssItems struct {
	ArmoryService
	cards []ItemCard
}

func (f ssItems) ListItems(context.Context, string, int, string, ItemListOptions) ([]ItemCard, int, error) {
	return f.cards, len(f.cards), nil
}

type ssTypes struct {
	ItemTypeFinder
	infos []ItemTypeInfo
}

func (f ssTypes) FindItemTypes(context.Context, string) ([]ItemTypeInfo, error) { return f.infos, nil }

type ssSystem struct{ goods []SystemGood }

func (f ssSystem) Goods(context.Context, string) ([]SystemGood, string, string, error) {
	return f.goods, "Test System", "Tier", nil
}

type ssRig struct {
	svc      ShopStockService
	pages    *ssPages
	writer   *ssWriter
	prices   *ssPrices
	listings ssListings
}

func newSSRig(shopOK bool, listed []ShopListing, rels map[int]*RelationInfo, types []ItemTypeInfo) *ssRig {
	r := &ssRig{pages: &ssPages{gone: map[string]bool{}}, writer: &ssWriter{}, prices: &ssPrices{food: 2, priced: map[string]ItemPrice{"it1": {Price: 12, Currency: "gp"}}}, listings: ssListings{listed}}
	r.svc = NewShopStockService(ssShops{shopOK}, r.pages, r.writer, r.listings, ssFinder{rels}, r.prices,
		ssItems{cards: []ItemCard{{ID: "it1", Name: "Rope"}, {ID: "it2", Name: "Lamp"}}},
		ssTypes{infos: types}, ssSystem{goods: []SystemGood{{Slug: "a", Name: "Axe", HasPrice: false}}})
	return r
}

var (
	ssDirector = Actor{UserID: "u1", Role: permissions.RoleOwner}
	ssScribe   = Actor{UserID: "u2", Role: permissions.RoleScribe}
	ssPlayer   = Actor{UserID: "u3", Role: permissions.RolePlayer}
	ssTypeInfo = []ItemTypeInfo{{ID: 7, Name: "Item"}}
)

func ssInt(n int) *int { return &n }

func ssKind(t *testing.T, err error, want int) {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != want {
		t.Fatalf("want app error %d, got %v", want, err)
	}
}

func TestShopStock_DirectorOnly(t *testing.T) {
	for _, a := range []Actor{ssScribe, ssPlayer} {
		r := newSSRig(true, nil, nil, ssTypeInfo)
		ctx := context.Background()
		if _, err := r.svc.Sources(ctx, "c", "s", a); err == nil {
			t.Errorf("role %d: Sources allowed", a.Role)
		}
		if _, err := r.svc.Keep(ctx, "c", "s", a, KeepInput{Goods: []KeepGood{{Name: "x"}}}); err == nil {
			t.Errorf("role %d: Keep allowed", a.Role)
		}
		if _, err := r.svc.Undo(ctx, "c", "s", a, "b", []int{1}); err == nil {
			t.Errorf("role %d: Undo allowed", a.Role)
		}
		if _, err := r.svc.SaveFoodPrice(ctx, "c", a, 2); err == nil {
			t.Errorf("role %d: SaveFoodPrice allowed", a.Role)
		}
		if len(r.writer.created) != 0 || r.prices.saved != 0 {
			t.Errorf("role %d: something was written", a.Role)
		}
	}
}

func TestShopStock_NotAShopOfThisCampaign(t *testing.T) {
	r := newSSRig(false, nil, nil, ssTypeInfo)
	if _, err := r.svc.Keep(context.Background(), "c", "s", ssDirector, KeepInput{Goods: []KeepGood{{Name: "x"}}}); err == nil {
		t.Fatal("Keep on a foreign shop succeeded")
	}
	if len(r.writer.created) != 0 || len(r.pages.made) != 0 {
		t.Fatal("foreign shop was written to")
	}
}

func TestShopStock_KeepValidation(t *testing.T) {
	many := make([]KeepGood, maxKeepGoods+1)
	for i := range many {
		many[i] = KeepGood{Name: "g"}
	}
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	tests := []struct {
		name  string
		goods []KeepGood
	}{
		{"empty", nil},
		{"too many", many},
		{"no name and no page", []KeepGood{{Name: "  "}}},
		{"name too long", []KeepGood{{Name: string(long)}}},
		{"negative price", []KeepGood{{Name: "a", Price: -1}}},
		{"huge price", []KeepGood{{Name: "a", Price: 1e15}}},
		{"negative count", []KeepGood{{Name: "a", Quantity: ssInt(-1)}}},
		{"count too big", []KeepGood{{Name: "a", Quantity: ssInt(maxKeepQuantity + 1)}}},
		{"coin too long", []KeepGood{{Name: "a", Currency: "abcdefghijklmnopqrstuvwxyz"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newSSRig(true, nil, nil, ssTypeInfo)
			_, err := r.svc.Keep(context.Background(), "c", "s", ssDirector, KeepInput{Goods: tt.goods})
			ssKind(t, err, 400)
			if len(r.writer.created) != 0 || len(r.pages.made) != 0 {
				t.Error("a rejected draft wrote something")
			}
		})
	}
}

func TestShopStock_KeepNoItemsCategory(t *testing.T) {
	r := newSSRig(true, nil, nil, nil)
	_, err := r.svc.Keep(context.Background(), "c", "s", ssDirector, KeepInput{Goods: []KeepGood{{Name: "Rope"}}})
	ssKind(t, err, 400)
}

func TestShopStock_KeepWritesBatch(t *testing.T) {
	r := newSSRig(true, []ShopListing{{RelationID: 9, TargetEntityID: "it2"}}, nil, ssTypeInfo)
	res, err := r.svc.Keep(context.Background(), "c", "s", ssDirector, KeepInput{
		HiddenFromPlayers: true,
		Goods: []KeepGood{
			{Name: " Rope ", Price: 1.234, Quantity: ssInt(3), Today: true},
			{ItemID: "it1", Price: 12, Currency: "gp"},
			{ItemID: "it2", Price: 5}, // already sold: skipped
			{Name: "Rope", Price: 2},  // same shop good again: skipped
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Relations) != 2 || res.Skipped != 2 || res.Batch == "" {
		t.Fatalf("result = %+v", res)
	}
	first := r.writer.created[0]
	if first.item != "good-Rope" || first.meta["price"] != 1.23 || first.meta["quantity"] != float64(3) || first.meta["today"] != true || first.meta["fill_batch"] != res.Batch || !first.dmOnly {
		t.Errorf("first listing = %+v", first)
	}
	if _, has := r.writer.created[1].meta["quantity"]; has {
		t.Error("unlimited good got a quantity")
	}
	for _, h := range r.pages.hidden {
		if !h {
			t.Error("hidden listing made a visible page")
		}
	}
}

func TestShopStock_KeepRollsBack(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(r *ssRig)
		goods  []KeepGood
		wantDl int
	}{
		{"later write fails", func(r *ssRig) { r.writer.failAt = 3 },
			[]KeepGood{{Name: "a"}, {Name: "b"}, {Name: "c"}}, 2},
		{"page gone mid-draft", func(r *ssRig) { r.pages.gone["it1"] = true },
			[]KeepGood{{Name: "a"}, {ItemID: "it1"}}, 1},
		{"page cannot be made", func(r *ssRig) { r.pages.failOn = "b" },
			[]KeepGood{{Name: "a"}, {Name: "b"}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newSSRig(true, nil, nil, ssTypeInfo)
			tt.setup(r)
			if _, err := r.svc.Keep(context.Background(), "c", "s", ssDirector, KeepInput{Goods: tt.goods}); err == nil {
				t.Fatal("want error")
			}
			if len(r.writer.deleted) != tt.wantDl {
				t.Fatalf("deleted %v, want %d removed", r.writer.deleted, tt.wantDl)
			}
		})
	}
}

func TestShopStock_Undo(t *testing.T) {
	meta := func(batch string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"fill_batch": batch})
		return b
	}
	rels := map[int]*RelationInfo{
		1: {ID: 1, CampaignID: "c", SourceEntityID: "s", RelationType: "sells", Metadata: meta("B")},
		2: {ID: 2, CampaignID: "c", SourceEntityID: "s", RelationType: "sells", Metadata: meta("other")},
		3: {ID: 3, CampaignID: "c", SourceEntityID: "s", RelationType: "sells", Metadata: json.RawMessage(`{"price":1}`)},
		4: {ID: 4, CampaignID: "c", SourceEntityID: "other-shop", RelationType: "sells", Metadata: meta("B")},
		5: {ID: 5, CampaignID: "other-campaign", SourceEntityID: "s", RelationType: "sells", Metadata: meta("B")},
		6: {ID: 6, CampaignID: "c", SourceEntityID: "s", RelationType: "mentions", Metadata: meta("B")},
	}
	r := newSSRig(true, nil, rels, ssTypeInfo)
	n, err := r.svc.Undo(context.Background(), "c", "s", ssDirector, "B", []int{1, 2, 3, 4, 5, 6, 99})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(r.writer.deleted) != 1 || r.writer.deleted[0] != 1 {
		t.Fatalf("removed %d, deleted %v; only relation 1 belongs to this shop and batch", n, r.writer.deleted)
	}
	if _, err := r.svc.Undo(context.Background(), "c", "s", ssDirector, "", []int{1}); err == nil {
		t.Error("empty batch accepted")
	}
}

func TestShopStock_Sources(t *testing.T) {
	r := newSSRig(true, []ShopListing{{RelationID: 9, TargetEntityID: "it2"}}, nil, ssTypeInfo)
	got, err := r.svc.Sources(context.Background(), "c", "s", ssDirector)
	if err != nil {
		t.Fatal(err)
	}
	if got.FoodPrice != 2 || len(got.Items) != 2 || got.Items[0].Price != 12 || got.Items[1].Price != 0 {
		t.Errorf("items/prices = %+v", got)
	}
	if len(got.Listed) != 1 || got.Listed[0].ItemID != "it2" {
		t.Errorf("listed = %+v", got.Listed)
	}
	if len(got.System) != 1 || got.System[0].HasPrice || got.SystemName != "Test System" || got.RarityLabel != "Tier" {
		t.Errorf("system = %+v", got.System)
	}
}

func TestShopStock_SaveFoodPrice(t *testing.T) {
	tests := []struct {
		name    string
		in      float64
		want    float64
		wantErr bool
	}{
		{"plain", 2, 2, false},
		{"rounded", 1.239, 1.24, false},
		{"zero", 0, 0, true},
		{"negative", -1, 0, true},
		{"rounds to nothing", 0.001, 0, true},
		{"too big", 2_000_000, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newSSRig(true, nil, nil, ssTypeInfo)
			got, err := r.svc.SaveFoodPrice(context.Background(), "c", ssDirector, tt.in)
			if (err != nil) != tt.wantErr || got != tt.want || r.prices.saved != tt.want {
				t.Fatalf("got %v, %v; saved %v", got, err, r.prices.saved)
			}
		})
	}
}
