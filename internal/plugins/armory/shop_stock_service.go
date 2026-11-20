// shop_stock_service.go fills a shop's shelves from a Director's draft. The
// draft is made and edited in the browser; nothing is saved until Keep, which
// lands the whole batch or none of it, and Undo removes exactly that batch.
//
// A good with no page becomes a real page marked as a shop good, so buying,
// packs, stashes, today's shelf and the shop room work on it unchanged. The
// mark keeps such pages out of lists and search (entities plugin).
package armory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// maxKeepGoods bounds one Keep so a batch stays small enough to undo at once.
	maxKeepGoods = 80
	// maxKeepQuantity bounds a stocked count.
	maxKeepQuantity = 9999
	// maxFillItems bounds the campaign items offered to a fill.
	maxFillItems = 500
	// fillBatchKey is the listing metadata key that ties a listing to its Keep.
	fillBatchKey = "fill_batch"
)

// ShopGoodPages finds or makes the page behind a good and reads page names.
// Implemented in internal/app over the entities service, so this plugin never
// reaches the entities tables for writes.
type ShopGoodPages interface {
	// EnsureShopGood returns the id of the shop good page called name,
	// making it in the item type when there is none.
	EnsureShopGood(ctx context.Context, campaignID, userID string, typeID int, name string, hiddenFromPlayers bool) (string, error)
	// PageInfo describes a live page of the campaign; nil when there is none.
	PageInfo(ctx context.Context, campaignID, id string) (*ListablePage, error)
	// Names maps page ids to names; ids that are gone are left out.
	Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error)
}

// ListablePage is what Keep needs to know about a page it is asked to sell.
type ListablePage struct {
	TypeID     int
	IsTemplate bool
	ShopGood   bool
}

// StockTx runs fn as one all-or-nothing unit: every page and listing fn
// writes through the context is kept together or not at all. Implemented in
// internal/app over the database handle.
type StockTx interface {
	Atomic(ctx context.Context, fn func(ctx context.Context) error) error
}

// ShopStockWriter creates and removes "sells" listings. Implemented in
// internal/app over the relations service.
type ShopStockWriter interface {
	CreateSells(ctx context.Context, campaignID, shopEntityID, itemEntityID, userID string, metadata json.RawMessage, dmOnly bool) (int, error)
	DeleteSells(ctx context.Context, relationID int) error
}

// SystemGood is one stockable entry of the campaign's game system.
type SystemGood struct {
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind,omitempty"`
	Rarity   int     `json:"rarity"`
	Price    float64 `json:"price"`
	HasPrice bool    `json:"hasPrice"`
	Summary  string  `json:"summary,omitempty"`
}

// SystemGoodsSource reads the goods the campaign's system names in its
// manifest. Implemented in internal/app; (nil, "", "", nil) means none.
type SystemGoodsSource interface {
	Goods(ctx context.Context, campaignID string) (goods []SystemGood, systemName, rarityLabel string, err error)
}

// FillItem is a campaign item page a fill may stock, with the price a shop
// already lists it at (Price 0 means none yet).
type FillItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency,omitempty"`
}

// FillListed is a page the shop already sells, so a fill leaves it alone.
type FillListed struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
}

// FillSources is everything a draft may draw on, for one shop.
type FillSources struct {
	FoodPrice   float64      `json:"foodPrice"`
	Items       []FillItem   `json:"items"`
	Listed      []FillListed `json:"listed"`
	System      []SystemGood `json:"system"`
	SystemName  string       `json:"systemName,omitempty"`
	RarityLabel string       `json:"rarityLabel,omitempty"`
}

// KeepGood is one drafted good, as the Director left it.
type KeepGood struct {
	Name string `json:"name"`
	// ItemID is the page of a good that has one; empty makes a shop good.
	ItemID   string  `json:"itemId,omitempty"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency,omitempty"`
	// Quantity nil is unlimited.
	Quantity *int `json:"quantity,omitempty"`
	Today    bool `json:"today,omitempty"`
}

// KeepInput is the body of a Keep.
type KeepInput struct {
	Goods []KeepGood `json:"goods"`
	// HiddenFromPlayers lists the goods as Director-only, with pages hidden
	// from players too.
	HiddenFromPlayers bool `json:"hiddenFromPlayers,omitempty"`
}

// KeepResult names what a Keep wrote, for Undo.
type KeepResult struct {
	Batch     string `json:"batch"`
	Relations []int  `json:"relations"`
	// Skipped counts goods the shop already sold.
	Skipped int `json:"skipped"`
}

// ShopStockService drafts and keeps stock. Every method is Director-only
// (Owner visibility); the route checks too.
type ShopStockService interface {
	Sources(ctx context.Context, campaignID, shopEntityID string, a Actor) (*FillSources, error)
	Keep(ctx context.Context, campaignID, shopEntityID string, a Actor, in KeepInput) (*KeepResult, error)
	// Undo removes the listings of one Keep and returns how many it removed
	// and which it left because a roll, purchase or note is attached.
	Undo(ctx context.Context, campaignID, shopEntityID string, a Actor, batch string, relationIDs []int) (removed int, kept []int, err error)
	SaveFoodPrice(ctx context.Context, campaignID string, a Actor, price float64) (float64, error)
}

type shopStockService struct {
	shops    ShopEntityChecker
	pages    ShopGoodPages
	writer   ShopStockWriter
	listings ShopListingLister
	finder   RelationFinder
	prices   ShopPriceRepository
	items    ArmoryService
	types    ItemTypeFinder
	system   SystemGoodsSource
	tx       StockTx
	activity ListingActivity
}

// NewShopStockService wires the service. system, tx and activity may be nil
// (tests): without tx the batch is not atomic.
func NewShopStockService(shops ShopEntityChecker, pages ShopGoodPages, writer ShopStockWriter, listings ShopListingLister, finder RelationFinder, prices ShopPriceRepository, items ArmoryService, types ItemTypeFinder, system SystemGoodsSource, tx StockTx, activity ListingActivity) ShopStockService {
	return &shopStockService{shops: shops, pages: pages, writer: writer, listings: listings, finder: finder, prices: prices, items: items, types: types, system: system, tx: tx, activity: activity}
}

func requireDirector(a Actor) error {
	if !a.IsOwner() {
		return apperror.NewForbidden("Only the Director can fill a shop's shelves.")
	}
	return nil
}

func (s *shopStockService) Sources(ctx context.Context, campaignID, shopEntityID string, a Actor) (*FillSources, error) {
	if err := requireDirector(a); err != nil {
		return nil, err
	}
	if err := requireShopInCampaign(ctx, s.shops, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	out := &FillSources{Items: []FillItem{}, Listed: []FillListed{}, System: []SystemGood{}}
	var err error
	if out.FoodPrice, err = s.prices.FoodPrice(ctx, campaignID); err != nil {
		return nil, err
	}

	opts := DefaultItemListOptions()
	opts.PerPage = maxFillItems
	cards, _, err := s.items.ListItems(ctx, campaignID, a.Role, a.UserID, opts)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	priced, err := s.prices.ItemPrices(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	for _, c := range cards {
		it := FillItem{ID: c.ID, Name: c.Name}
		if p, ok := priced[c.ID]; ok {
			it.Price, it.Currency = p.Price, p.Currency
		}
		out.Items = append(out.Items, it)
	}

	listed, err := s.listings.ListSells(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	ids := make([]string, 0, len(listed))
	for _, l := range listed {
		ids = append(ids, l.TargetEntityID)
	}
	names, err := s.pages.Names(ctx, campaignID, ids)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	for _, id := range ids {
		if n, ok := names[id]; ok {
			out.Listed = append(out.Listed, FillListed{ItemID: id, Name: n})
		}
	}

	if s.system != nil {
		goods, name, label, err := s.system.Goods(ctx, campaignID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if goods != nil {
			out.System = goods
		}
		out.SystemName, out.RarityLabel = name, label
	}
	return out, nil
}

// cleanGood validates one drafted good and returns it in stored form.
func cleanGood(g KeepGood) (KeepGood, error) {
	g.Name = strings.TrimSpace(g.Name)
	if g.ItemID == "" && g.Name == "" {
		return g, apperror.NewBadRequest("Every good needs a name.")
	}
	if len(g.Name) > 200 {
		return g, apperror.NewBadRequest("A good's name can be at most 200 characters.")
	}
	if math.IsNaN(g.Price) || math.IsInf(g.Price, 0) || g.Price < 0 {
		return g, apperror.NewBadRequest("A price can't be negative.")
	}
	// Zero is allowed: it is a good left for the Director to price, and buying
	// already refuses it until one is set.
	if g.Price > 0 {
		if _, err := unitPriceCents(g.Price); err != nil {
			return g, apperror.NewBadRequest("A price that large isn't allowed.")
		}
		g.Price = math.Round(g.Price*100) / 100
	}
	g.Currency = normCurrency(g.Currency)
	if len(g.Currency) > 16 {
		return g, apperror.NewBadRequest("A coin name can be at most 16 characters.")
	}
	if g.Quantity != nil && (*g.Quantity < 0 || *g.Quantity > maxKeepQuantity) {
		return g, apperror.NewBadRequest("A count must be between 0 and 9999.")
	}
	return g, nil
}

func newBatchID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "batch"
	}
	return hex.EncodeToString(b)
}

func (s *shopStockService) Keep(ctx context.Context, campaignID, shopEntityID string, a Actor, in KeepInput) (*KeepResult, error) {
	if err := requireDirector(a); err != nil {
		return nil, err
	}
	if err := requireShopInCampaign(ctx, s.shops, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	if len(in.Goods) == 0 {
		return nil, apperror.NewBadRequest("There are no goods to keep.")
	}
	if len(in.Goods) > maxKeepGoods {
		return nil, apperror.NewBadRequest(fmt.Sprintf("Keep at most %d goods at once.", maxKeepGoods))
	}
	goods := make([]KeepGood, 0, len(in.Goods))
	for _, g := range in.Goods {
		c, err := cleanGood(g)
		if err != nil {
			return nil, err
		}
		goods = append(goods, c)
	}

	have, err := s.listings.ListSells(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	typeID := 0
	needType := false
	for _, g := range goods {
		if g.ItemID == "" {
			needType = true
		}
	}
	if needType {
		infos, err := s.types.FindItemTypes(ctx, campaignID)
		if err != nil || len(infos) == 0 {
			return nil, apperror.NewBadRequest("This campaign has no Items category yet, so goods can't be given pages. Add one under Campaign settings first.")
		}
		typeID = infos[0].ID
	}

	// Pages named by id are checked before anything is written: an Items
	// page or a shop good, never a template, a character or another type.
	var itemTypes map[int]bool
	for _, g := range goods {
		if g.ItemID == "" {
			continue
		}
		if itemTypes == nil {
			ids, err := s.types.FindItemTypeIDs(ctx, campaignID)
			if err != nil {
				return nil, apperror.NewInternal(err)
			}
			itemTypes = make(map[int]bool, len(ids))
			for _, id := range ids {
				itemTypes[id] = true
			}
		}
		pg, err := s.pages.PageInfo(ctx, campaignID, g.ItemID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if pg == nil {
			return nil, apperror.NewBadRequest("One of the pages in this draft is gone. Roll again to refresh it.")
		}
		if pg.IsTemplate || !(pg.ShopGood || itemTypes[pg.TypeID]) {
			return nil, apperror.NewBadRequest("Only item pages can be stocked. Roll again to refresh the draft.")
		}
	}

	res := &KeepResult{Batch: newBatchID(), Relations: []int{}}
	write := func(ctx context.Context) error {
		// A retry of a failed attempt starts clean.
		res.Relations, res.Skipped = res.Relations[:0], 0
		sold := make(map[string]bool, len(have))
		for _, l := range have {
			sold[l.TargetEntityID] = true
		}
		for _, g := range goods {
			itemID := g.ItemID
			if itemID == "" {
				id, err := s.pages.EnsureShopGood(ctx, campaignID, a.UserID, typeID, g.Name, in.HiddenFromPlayers)
				if err != nil {
					return asAppError(err)
				}
				itemID = id
			}
			if sold[itemID] {
				res.Skipped++
				continue
			}
			meta := map[string]any{"price": g.Price, "currency": g.Currency, "in_stock": true, fillBatchKey: res.Batch}
			if g.Quantity != nil {
				meta["quantity"] = *g.Quantity
			}
			if g.Today {
				meta["today"] = true
			}
			raw, _ := json.Marshal(meta)
			id, err := s.writer.CreateSells(ctx, campaignID, shopEntityID, itemID, a.UserID, raw, in.HiddenFromPlayers)
			if err != nil {
				return asAppError(err)
			}
			sold[itemID] = true
			res.Relations = append(res.Relations, id)
		}
		return nil
	}
	var err2 error
	if s.tx != nil {
		err2 = s.tx.Atomic(ctx, write)
	} else {
		err2 = write(ctx)
	}
	if err2 != nil {
		return nil, asAppError(err2)
	}
	sort.Ints(res.Relations)
	return res, nil
}

func (s *shopStockService) Undo(ctx context.Context, campaignID, shopEntityID string, a Actor, batch string, relationIDs []int) (int, []int, error) {
	if err := requireDirector(a); err != nil {
		return 0, nil, err
	}
	if err := requireShopInCampaign(ctx, s.shops, campaignID, shopEntityID); err != nil {
		return 0, nil, err
	}
	if batch == "" || len(relationIDs) > maxKeepGoods {
		return 0, nil, apperror.NewBadRequest("There is nothing to undo.")
	}
	// Only a listing of this shop that this Keep wrote: a good the
	// Director has since changed by hand keeps the batch mark, but one
	// added by hand never has it.
	var mine []int
	for _, id := range relationIDs {
		rel, err := s.finder.GetByID(ctx, id)
		if err != nil || rel == nil {
			continue
		}
		if rel.CampaignID != campaignID || rel.SourceEntityID != shopEntityID || rel.RelationType != shopSellsRelation {
			continue
		}
		var m map[string]any
		if json.Unmarshal(rel.Metadata, &m) != nil || m[fillBatchKey] != batch {
			continue
		}
		mine = append(mine, id)
	}
	// Removing a listing deletes the counter rolls and notes tied to it, so
	// one a player has already rolled for or bought from stays.
	active := map[int]bool{}
	if s.activity != nil && len(mine) > 0 {
		var err error
		if active, err = s.activity.Active(ctx, campaignID, shopEntityID, mine); err != nil {
			return 0, nil, apperror.NewInternal(err)
		}
	}
	removed := 0
	kept := []int{}
	for _, id := range mine {
		if active[id] {
			kept = append(kept, id)
			continue
		}
		if err := s.writer.DeleteSells(ctx, id); err != nil {
			return removed, kept, asAppError(err)
		}
		removed++
	}
	return removed, kept, nil
}

func (s *shopStockService) SaveFoodPrice(ctx context.Context, campaignID string, a Actor, price float64) (float64, error) {
	if err := requireDirector(a); err != nil {
		return 0, err
	}
	if math.IsNaN(price) || math.IsInf(price, 0) || price <= 0 || price > 1_000_000 {
		return 0, apperror.NewBadRequest("A day's food has to cost more than nothing.")
	}
	price = math.Round(price*100) / 100
	if price <= 0 {
		return 0, apperror.NewBadRequest("A day's food has to cost at least 0.01.")
	}
	if err := s.prices.SaveFoodPrice(ctx, campaignID, a.UserID, price); err != nil {
		return 0, err
	}
	return price, nil
}
