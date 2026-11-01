package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// maxStreetShops bounds one street; a campaign with more is told so.
const maxStreetShops = 300

// armoryShopListerAdapter implements armory.ShopLister over the entities
// service. Listing goes through entities.List, which applies page
// visibility, and a shop's town (its parent page) is named only when the
// viewer can see that page too.
type armoryShopListerAdapter struct {
	svc entities.EntityService
}

func (a *armoryShopListerAdapter) ListShops(ctx context.Context, campaignID string, role int, userID string) ([]armory.StreetShop, bool, error) {
	et, err := a.svc.GetEntityTypeBySlug(ctx, campaignID, "shop")
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == http.StatusNotFound {
			return []armory.StreetShop{}, false, nil
		}
		return nil, false, err
	}
	if et == nil {
		return []armory.StreetShop{}, false, nil
	}
	var list []entities.Entity
	total := 0
	for page := 1; len(list) < maxStreetShops; page++ {
		got, n, err := a.svc.List(ctx, campaignID, et.ID, role, userID, entities.ListOptions{Page: page, PerPage: 100, Sort: "name"})
		if err != nil {
			return nil, false, err
		}
		total = n
		list = append(list, got...)
		if len(got) < 100 || len(list) >= total {
			break
		}
	}
	truncated := total > len(list)
	if len(list) > maxStreetShops {
		list, truncated = list[:maxStreetShops], true
	}

	towns := map[string]string{}
	for _, e := range list {
		if e.ParentID != nil && *e.ParentID != "" {
			towns[*e.ParentID] = ""
		}
	}
	ids := make([]string, 0, len(towns))
	for id := range towns {
		ids = append(ids, id)
	}
	seen := map[string]bool{}
	if role >= permissions.RoleOwner {
		for _, id := range ids {
			seen[id] = true
		}
	} else if len(ids) > 0 {
		if seen, err = a.svc.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID); err != nil {
			return nil, false, err
		}
	}
	for _, id := range ids {
		if !seen[id] {
			continue
		}
		p, err := a.svc.GetByID(ctx, id)
		if err != nil || p == nil || p.CampaignID != campaignID {
			continue
		}
		towns[id] = p.Name
	}

	out := make([]armory.StreetShop, 0, len(list))
	for _, e := range list {
		s := armory.StreetShop{ID: e.ID, Name: e.Name, URL: "/campaigns/" + campaignID + "/entities/" + e.ID}
		if e.ParentID != nil && towns[*e.ParentID] != "" {
			s.TownID, s.TownName = *e.ParentID, towns[*e.ParentID]
		}
		out = append(out, s)
	}
	return out, truncated, nil
}
