package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// shopGoodPagesAdapter gives the armory's Fill the shelves the entities
// service's shop good pages, so the armory never writes entities itself.
type shopGoodPagesAdapter struct {
	svc entities.EntityService
}

func (a *shopGoodPagesAdapter) EnsureShopGood(ctx context.Context, campaignID, userID string, typeID int, name string, hidden bool) (string, error) {
	e, err := a.svc.EnsureShopGood(ctx, campaignID, userID, typeID, name, hidden)
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

func (a *shopGoodPagesAdapter) ItemPageExists(ctx context.Context, campaignID, id string) (bool, error) {
	e, err := a.svc.GetByID(ctx, id)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return e.CampaignID == campaignID, nil
}

func (a *shopGoodPagesAdapter) Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		e, err := a.svc.GetByID(ctx, id)
		if err != nil {
			var ae *apperror.AppError
			if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
				continue
			}
			return nil, err
		}
		if e.CampaignID == campaignID {
			out[id] = e.Name
		}
	}
	return out, nil
}

// shopStockWriterAdapter writes "sells" listings through the relations service.
type shopStockWriterAdapter struct {
	svc relations.RelationService
}

func (a *shopStockWriterAdapter) CreateSells(ctx context.Context, campaignID, shopID, itemID, userID string, meta json.RawMessage, dmOnly bool) (int, error) {
	rel, err := a.svc.Create(ctx, campaignID, shopID, itemID, "sells", "sold by", userID, meta, dmOnly)
	if err != nil {
		return 0, err
	}
	return rel.ID, nil
}

func (a *shopStockWriterAdapter) DeleteSells(ctx context.Context, id int) error {
	return a.svc.Delete(ctx, id)
}

// systemGoodsAdapter reads the goods the campaign's enabled system names.
type systemGoodsAdapter struct {
	sys interface {
		EnabledSystem(ctx context.Context, campaignID string) systems.System
	}
}

func (a *systemGoodsAdapter) Goods(ctx context.Context, campaignID string) ([]armory.SystemGood, string, string, error) {
	if a.sys == nil {
		return nil, "", "", nil
	}
	sys := a.sys.EnabledSystem(ctx, campaignID)
	if sys == nil || sys.Info() == nil {
		return nil, "", "", nil
	}
	goods, label := systems.GoodsOf(sys)
	out := make([]armory.SystemGood, 0, len(goods))
	for _, g := range goods {
		out = append(out, armory.SystemGood{Slug: g.Slug, Name: g.Name, Kind: g.Kind, Rarity: g.Rarity, Price: g.Price, HasPrice: g.HasPrice, Summary: g.Summary})
	}
	return out, sys.Info().Name, label, nil
}
