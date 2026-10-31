package armory

import (
	"context"
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StreetShop is one shop page as the street lists it: its town is the page
// it sits under, if the viewer can see that page.
type StreetShop struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	TownID   string `json:"townId,omitempty"`
	TownName string `json:"townName,omitempty"`
}

// ShopLister lists the shop pages a viewer can see. Implemented in
// internal/app over the entities service, which applies page visibility, so
// a hidden shop is never listed. truncated is true when the campaign has more
// shops than one street shows.
type ShopLister interface {
	ListShops(ctx context.Context, campaignID string, role int, userID string) (shops []StreetShop, truncated bool, err error)
}

// DowntimeReader reports whether the campaign's shops are open (downtime).
type DowntimeReader interface {
	IsDowntimeOpen(ctx context.Context, campaignID string) (bool, error)
}

// StreetFront is a listed shop with the storefront it is drawn with.
type StreetFront struct {
	StreetShop
	Front json.RawMessage `json:"front"`
	// Saved is false while the shop still shows the default storefront.
	Saved bool `json:"saved"`
}

// StreetView is everything the Shops street draws.
type StreetView struct {
	// Open is the campaign's downtime switch: closed shops draw their
	// closed look, and buying needs the GM's OK.
	Open      bool          `json:"open"`
	Shops     []StreetFront `json:"shops"`
	Truncated bool          `json:"truncated"`
}

// ShopStreetService reads the street and saves storefronts.
type ShopStreetService interface {
	Street(ctx context.Context, campaignID string, role int, userID string) (*StreetView, error)
	// SaveFront validates raw and stores it as the shop's storefront. Who may
	// save is decided by the route (Owner).
	SaveFront(ctx context.Context, campaignID, shopEntityID, userID string, raw []byte) (json.RawMessage, error)
}

type shopStreetService struct {
	repo     ShopFrontRepository
	shops    ShopEntityChecker
	lister   ShopLister
	downtime DowntimeReader
}

// NewShopStreetService creates the service.
func NewShopStreetService(repo ShopFrontRepository, shops ShopEntityChecker, lister ShopLister, downtime DowntimeReader) ShopStreetService {
	return &shopStreetService{repo: repo, shops: shops, lister: lister, downtime: downtime}
}

func (s *shopStreetService) Street(ctx context.Context, campaignID string, role int, userID string) (*StreetView, error) {
	list, truncated, err := s.lister.ListShops(ctx, campaignID, role, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	open := true
	if s.downtime != nil {
		if open, err = s.downtime.IsDowntimeOpen(ctx, campaignID); err != nil {
			return nil, err
		}
	}
	ids := make([]string, len(list))
	for i, sh := range list {
		ids[i] = sh.ID
	}
	fronts, err := s.repo.ListFor(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	def, err := json.Marshal(DefaultShopFront())
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	v := &StreetView{Open: open, Shops: make([]StreetFront, 0, len(list)), Truncated: truncated}
	for _, sh := range list {
		f, ok := fronts[sh.ID]
		if !ok {
			f = def
		}
		v.Shops = append(v.Shops, StreetFront{StreetShop: sh, Front: f, Saved: ok})
	}
	return v, nil
}

func (s *shopStreetService) SaveFront(ctx context.Context, campaignID, shopEntityID, userID string, raw []byte) (json.RawMessage, error) {
	if err := requireShopInCampaign(ctx, s.shops, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	front, err := NormalizeShopFront(raw)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, campaignID, shopEntityID, userID, front); err != nil {
		return nil, err
	}
	return front, nil
}
