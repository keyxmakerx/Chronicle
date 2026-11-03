package armory

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"sort"
	"strconv"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Today's shelf: the GM flags some of a shop's "sells" listings as able to
// turn up on today's shelf (metadata "today": true) and sets how many places
// change each day. Which flagged goods are out on a given day is worked out
// from the shop and the shelf day, never stored, so every viewer and every
// server sees the same shelf and nothing needs a nightly job.

// Bounds and default for the number of places on today's shelf.
const (
	defaultTodaySpots = 2
	minTodaySpots     = 1
	maxTodaySpots     = 20
)

// notOnShelfMessage is what a buyer is told when a good that only turns up on
// some days is not out today.
const notOnShelfMessage = "That's not on the shelf today."

// ShelfDay is the day today's shelf is set for. The shelf changes at dawn, so
// the hours before dawn still belong to the previous day.
type ShelfDay struct {
	// Index counts shelf days; consecutive days differ by one.
	Index int64
	// HoursToDawn is the whole calendar hours until the shelf changes, or -1
	// when there is no calendar to count them on.
	HoursToDawn int
}

// ShelfDayReader reads the campaign calendar's current shelf day. Implemented
// in internal/app over the calendar service. (nil, nil) means the campaign
// has no calendar, and the real date is used instead. The day is the same
// for every viewer; HoursToDawn is -1 for a viewer the calendar is hidden
// from, since it tells the time of day.
type ShelfDayReader interface {
	ShelfDay(ctx context.Context, campaignID string, role int, userID string) (*ShelfDay, error)
}

// ShopListing is one "sells" listing of a shop, as today's shelf reads it.
type ShopListing struct {
	RelationID     int
	TargetEntityID string
	DmOnly         bool
	Metadata       json.RawMessage
}

// ShopListingLister lists a shop's "sells" listings. Implemented in
// internal/app over the relations service.
type ShopListingLister interface {
	ListSells(ctx context.Context, campaignID, shopEntityID string) ([]ShopListing, error)
}

// TodayView is today's shelf of one shop, as the shop room and stock list
// draw it. Ids are "sells" relation ids.
type TodayView struct {
	// Spots is how many places change each day.
	Spots int `json:"spots"`
	// HoursToDawn is nil when there is no calendar to count on.
	HoursToDawn *int `json:"hoursToDawn"`
	// Pool is every good that can turn up; Today is the ones out today. A
	// good in Pool but not in Today is not for sale today.
	Pool  []int `json:"pool"`
	Today []int `json:"today"`
}

// ShopTodayService works out today's shelf and saves its number of places.
type ShopTodayService interface {
	Today(ctx context.Context, campaignID, shopEntityID string, a Actor) (*TodayView, error)
	// SaveSpots stores the number of places, clamped to the allowed range.
	// Who may save is decided by the route (Owner).
	SaveSpots(ctx context.Context, campaignID, shopEntityID, userID string, spots int) (*TodayView, error)
	// OnShelf reports whether a listing may be bought today: true for every
	// good that is not flagged for today's shelf.
	OnShelf(ctx context.Context, campaignID, shopEntityID string, relationID int, meta shopMeta) (bool, error)
}

type shopTodayService struct {
	repo       ShopShelfRepository
	shops      ShopEntityChecker
	visibility EntityVisibilityFilter
	listings   ShopListingLister
	days       ShelfDayReader
	now        func() time.Time
}

// NewShopTodayService creates the service. days may be nil (no calendar
// plugin), and the real date is used.
func NewShopTodayService(repo ShopShelfRepository, shops ShopEntityChecker, visibility EntityVisibilityFilter, listings ShopListingLister, days ShelfDayReader) ShopTodayService {
	return &shopTodayService{repo: repo, shops: shops, visibility: visibility, listings: listings, days: days, now: time.Now}
}

func (s *shopTodayService) Today(ctx context.Context, campaignID, shopEntityID string, a Actor) (*TodayView, error) {
	if err := requireViewableShop(ctx, s.shops, s.visibility, campaignID, shopEntityID, a.Role, a.UserID); err != nil {
		return nil, err
	}
	v, pool, err := s.shelf(ctx, campaignID, shopEntityID, a)
	if err != nil {
		return nil, err
	}
	if !a.IsOwner() {
		if err := s.onlyViewable(ctx, campaignID, a, v, pool); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (s *shopTodayService) SaveSpots(ctx context.Context, campaignID, shopEntityID, userID string, spots int) (*TodayView, error) {
	if err := requireShopInCampaign(ctx, s.shops, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	if err := s.repo.UpsertSpots(ctx, campaignID, shopEntityID, userID, clampSpots(spots)); err != nil {
		return nil, err
	}
	v, _, err := s.shelf(ctx, campaignID, shopEntityID, shelfSystem)
	return v, err
}

func (s *shopTodayService) OnShelf(ctx context.Context, campaignID, shopEntityID string, relationID int, meta shopMeta) (bool, error) {
	if !meta.Today {
		return true, nil
	}
	v, _, err := s.shelf(ctx, campaignID, shopEntityID, shelfSystem)
	if err != nil {
		return false, err
	}
	for _, id := range v.Today {
		if id == relationID {
			return true, nil
		}
	}
	return false, nil
}

// shelf works out the shelf with nothing filtered for the viewer; pool maps
// each pool relation id to its good.
func (s *shopTodayService) shelf(ctx context.Context, campaignID, shopEntityID string, a Actor) (*TodayView, map[int]string, error) {
	spots, err := s.repo.GetSpots(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, nil, err
	}
	if spots == 0 {
		spots = defaultTodaySpots
	}
	spots = clampSpots(spots)
	listings, err := s.listings.ListSells(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, nil, apperror.NewInternal(err)
	}
	pool := map[int]string{}
	ids := []int{}
	for _, l := range listings {
		// A DM-only listing is not on anyone's shelf: the shelf is the same
		// for every viewer, so it may only hold goods every viewer is shown.
		if l.DmOnly || !parseShopMeta(l.Metadata).Today {
			continue
		}
		pool[l.RelationID] = l.TargetEntityID
		ids = append(ids, l.RelationID)
	}
	day, err := s.shelfDay(ctx, campaignID, a)
	if err != nil {
		return nil, nil, err
	}
	v := &TodayView{Spots: spots, Pool: ids, Today: pickToday(shopEntityID, day.Index, ids, spots)}
	if day.HoursToDawn >= 0 {
		h := day.HoursToDawn
		v.HoursToDawn = &h
	}
	return v, pool, nil
}

// shelfDay falls back to the real UTC date when the campaign has no calendar.
func (s *shopTodayService) shelfDay(ctx context.Context, campaignID string, a Actor) (ShelfDay, error) {
	if s.days != nil {
		d, err := s.days.ShelfDay(ctx, campaignID, a.Role, a.UserID)
		if err != nil {
			return ShelfDay{}, err
		}
		if d != nil {
			return *d, nil
		}
	}
	return ShelfDay{Index: s.now().UTC().Unix() / 86400, HoursToDawn: -1}, nil
}

// onlyViewable drops goods the viewer cannot see, so the response names no
// listing of a hidden good. The pick itself is made over the whole pool, so
// the shelf stays the same for everyone.
func (s *shopTodayService) onlyViewable(ctx context.Context, campaignID string, a Actor, v *TodayView, pool map[int]string) error {
	if len(v.Pool) == 0 {
		return nil
	}
	if s.visibility == nil {
		v.Pool, v.Today = []int{}, []int{}
		return nil
	}
	items := make([]string, 0, len(pool))
	for _, it := range pool {
		items = append(items, it)
	}
	seen, err := s.visibility.FilterViewableEntityIDs(ctx, campaignID, items, a.Role, a.UserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	keep := func(ids []int) []int {
		out := make([]int, 0, len(ids))
		for _, id := range ids {
			if seen[pool[id]] {
				out = append(out, id)
			}
		}
		return out
	}
	v.Pool, v.Today = keep(v.Pool), keep(v.Today)
	return nil
}

// shelfSystem reads the shelf for a save or a purchase check, where no
// countdown is shown.
var shelfSystem = Actor{Role: permissions.RoleOwner}

func clampSpots(n int) int {
	return min(max(n, minTodaySpots), maxTodaySpots)
}

// pickToday chooses spots goods from the pool for one shop on one shelf day.
// It is a pure function of its inputs: the pool is sorted first, so the order
// listings come back in never changes the pick, and the seed is the shop and
// the day, so two shops differ and each day differs from the last.
func pickToday(shopEntityID string, dayIndex int64, pool []int, spots int) []int {
	ids := append([]int(nil), pool...)
	sort.Ints(ids)
	h := fnv.New64a()
	_, _ = h.Write([]byte(shopEntityID + ":" + strconv.FormatInt(dayIndex, 10)))
	state := h.Sum64()
	next := func() uint64 {
		// splitmix64: small, fast and well mixed, which is all a shelf needs.
		state += 0x9e3779b97f4a7c15
		z := state
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		return z ^ (z >> 31)
	}
	for i := len(ids) - 1; i > 0; i-- {
		j := int(next() % uint64(i+1))
		ids[i], ids[j] = ids[j], ids[i]
	}
	if spots < len(ids) {
		ids = ids[:spots]
	}
	sort.Ints(ids)
	return ids
}
