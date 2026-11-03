package armory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeShelfRepo struct {
	spots   map[string]int
	savedBy string
}

func (f *fakeShelfRepo) GetSpots(_ context.Context, _, shop string) (int, error) {
	return f.spots[shop], nil
}

func (f *fakeShelfRepo) UpsertSpots(_ context.Context, _, shop, userID string, n int) error {
	f.spots[shop] = n
	f.savedBy = userID
	return nil
}

type fakeListings struct {
	rows []ShopListing
	err  error
}

func (f *fakeListings) ListSells(context.Context, string, string) ([]ShopListing, error) {
	return f.rows, f.err
}

type fakeShelfDays struct {
	day *ShelfDay
	err error
}

func (f *fakeShelfDays) ShelfDay(context.Context, string, int, string) (*ShelfDay, error) {
	return f.day, f.err
}

func listing(id int, item, meta string) ShopListing {
	return ShopListing{RelationID: id, TargetEntityID: item, Metadata: json.RawMessage(meta)}
}

func todayFx() (*shopTodayService, *fakeShelfRepo, *fakeListings, *fakeShelfDays, *fakeRoomVis) {
	repo := &fakeShelfRepo{spots: map[string]int{}}
	ls := &fakeListings{rows: []ShopListing{
		listing(1, "i1", `{"price":1}`),
		listing(2, "i2", `{"price":1,"today":true}`),
		listing(3, "i3", `{"price":1,"today":true}`),
		listing(4, "i4", `{"price":1,"today":true}`),
		listing(5, "i5", `{"price":1,"today":true}`),
		listing(6, "i6", `{"price":1,"today":false}`),
	}}
	dm := listing(7, "i7", `{"price":1,"today":true}`)
	dm.DmOnly = true
	ls.rows = append(ls.rows, dm)
	days := &fakeShelfDays{day: &ShelfDay{Index: 100, HoursToDawn: 6}}
	vis := &fakeRoomVis{viewable: map[string]bool{"shop-1": true, "i2": true, "i3": true, "i4": true, "i5": true}}
	svc := NewShopTodayService(repo, fakeShops{shops: map[string]string{"shop-1": "camp"}}, vis, ls, days).(*shopTodayService)
	return svc, repo, ls, days, vis
}

func TestPickToday(t *testing.T) {
	pool := []int{5, 3, 9, 1, 7, 2}
	a := pickToday("shop-1", 10, pool, 2)
	if len(a) != 2 {
		t.Fatalf("picked %v, want 2 goods", a)
	}
	if b := pickToday("shop-1", 10, []int{2, 7, 1, 9, 3, 5}, 2); !reflect.DeepEqual(a, b) {
		t.Errorf("listing order changed the pick: %v vs %v", a, b)
	}
	if got := pickToday("shop-1", 10, pool, 20); len(got) != len(pool) {
		t.Errorf("more places than goods gave %v", got)
	}
	if got := pickToday("shop-1", 10, nil, 2); len(got) != 0 {
		t.Errorf("empty pool gave %v", got)
	}
	// Over a month the shelf changes from day to day and differs between shops.
	changed, differs := 0, 0
	for d := int64(0); d < 30; d++ {
		if !reflect.DeepEqual(pickToday("shop-1", d, pool, 2), pickToday("shop-1", d+1, pool, 2)) {
			changed++
		}
		if !reflect.DeepEqual(pickToday("shop-1", d, pool, 2), pickToday("shop-2", d, pool, 2)) {
			differs++
		}
	}
	if changed < 15 || differs < 15 {
		t.Errorf("shelf barely moves: %d of 30 days changed, %d of 30 differ between shops", changed, differs)
	}
}

func TestShopToday_View(t *testing.T) {
	ctx := context.Background()
	t.Run("owner sees the pool, the default places and hours to dawn", func(t *testing.T) {
		svc, _, _, _, _ := todayFx()
		v, err := svc.Today(ctx, "camp", "shop-1", owner)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v.Pool, []int{2, 3, 4, 5}) {
			t.Errorf("pool = %v; unflagged and DM-only goods must stay out", v.Pool)
		}
		if v.Spots != defaultTodaySpots || len(v.Today) != defaultTodaySpots {
			t.Errorf("spots %d, today %v", v.Spots, v.Today)
		}
		if v.HoursToDawn == nil || *v.HoursToDawn != 6 {
			t.Errorf("hoursToDawn = %v", v.HoursToDawn)
		}
		if !reflect.DeepEqual(v.Today, pickToday("shop-1", 100, v.Pool, 2)) {
			t.Errorf("today %v is not the day's pick", v.Today)
		}
	})
	t.Run("a player is shown only goods they can see, from the same pick", func(t *testing.T) {
		svc, _, _, _, vis := todayFx()
		all, _ := svc.Today(ctx, "camp", "shop-1", owner)
		hidden := all.Today[0]
		vis.viewable["i"+string(rune('0'+hidden))] = false
		v, err := svc.Today(ctx, "camp", "shop-1", pU1)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range append(append([]int{}, v.Pool...), v.Today...) {
			if id == hidden {
				t.Errorf("hidden good %d was named to a player: %+v", hidden, v)
			}
		}
		if len(v.Today) != len(all.Today)-1 || v.Today[0] != all.Today[1] {
			t.Errorf("player's shelf %v, owner's %v: the pick must not change", v.Today, all.Today)
		}
	})
	t.Run("a hidden or foreign shop is not found", func(t *testing.T) {
		svc, _, _, _, vis := todayFx()
		vis.viewable["shop-1"] = false
		if _, err := svc.Today(ctx, "camp", "shop-1", pU1); !isStatus(err, http.StatusNotFound) {
			t.Errorf("hidden shop: %v", err)
		}
		if _, err := svc.Today(ctx, "other", "shop-1", owner); !isStatus(err, http.StatusNotFound) {
			t.Errorf("foreign shop: %v", err)
		}
	})
	t.Run("no calendar uses the real date and gives no countdown", func(t *testing.T) {
		svc, _, _, days, _ := todayFx()
		days.day = nil
		svc.now = func() time.Time { return time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC) }
		v, err := svc.Today(ctx, "camp", "shop-1", owner)
		if err != nil {
			t.Fatal(err)
		}
		if v.HoursToDawn != nil {
			t.Errorf("hoursToDawn = %v, want none", *v.HoursToDawn)
		}
		want := pickToday("shop-1", time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC).Unix()/86400, v.Pool, 2)
		if !reflect.DeepEqual(v.Today, want) {
			t.Errorf("today %v, want %v", v.Today, want)
		}
	})
	t.Run("a calendar fault is an error, not an empty shelf", func(t *testing.T) {
		svc, _, _, days, _ := todayFx()
		days.err = errors.New("db down")
		if _, err := svc.Today(ctx, "camp", "shop-1", owner); err == nil {
			t.Error("want an error")
		}
	})
}

func TestShopToday_SaveSpots(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ in, want int }{{3, 3}, {0, minTodaySpots}, {-4, minTodaySpots}, {99, maxTodaySpots}} {
		svc, repo, _, _, _ := todayFx()
		v, err := svc.SaveSpots(ctx, "camp", "shop-1", "gm", tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if repo.spots["shop-1"] != tc.want || v.Spots != tc.want || repo.savedBy != "gm" {
			t.Errorf("save %d stored %d, view %d", tc.in, repo.spots["shop-1"], v.Spots)
		}
	}
	svc, _, _, _, _ := todayFx()
	if _, err := svc.SaveSpots(ctx, "camp", "nope", "gm", 2); !isStatus(err, http.StatusNotFound) {
		t.Errorf("unknown shop: %v", err)
	}
}

// fakeToday puts the listed relations out today and nothing else.
type fakeToday struct {
	ShopTodayService
	out map[int]bool
}

func (f fakeToday) OnShelf(_ context.Context, _, _ string, id int, m shopMeta) (bool, error) {
	return !m.Today || f.out[id], nil
}

func TestShopBuy_TodayShelf(t *testing.T) {
	ctx := context.Background()
	setup := func() *buyFx {
		f := newBuyFx()
		f.openDowntime()
		f.listings.rels[2].Metadata = json.RawMessage(`{"price":2.5,"currency":"gp","today":true}`)
		f.svc.(*shopBuyService).today = fakeToday{out: map[int]bool{}}
		return f
	}
	t.Run("a good not out today is refused in plain words", func(t *testing.T) {
		f := setup()
		_, err := f.svc.Buy(ctx, "camp", "shop-1", pU1, basket("c1", [2]int{2, 1}))
		if !isStatus(err, http.StatusBadRequest) || !strings.Contains(err.Error(), notOnShelfMessage) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a good out today and an everyday good sell", func(t *testing.T) {
		f := setup()
		f.svc.(*shopBuyService).today = fakeToday{out: map[int]bool{2: true}}
		if _, err := f.svc.Buy(ctx, "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}, [2]int{2, 1})); err != nil {
			t.Fatal(err)
		}
	})
}
