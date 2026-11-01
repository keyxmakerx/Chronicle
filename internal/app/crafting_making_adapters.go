package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The ports crafting needs to make recipes where the work is: Armory items
// for a recipe's result or a pack's missing material, the "How it's made"
// block on item pages, and which campaigns a user directs for recipe packs.

// craftingItemMaker makes a Director-only Armory item in the campaign's
// first item category, as a give's map handout does.
type craftingItemMaker struct {
	dir *armoryStashDirectoryAdapter
	svc entities.EntityService
}

func (m *craftingItemMaker) CreateItem(ctx context.Context, campaignID, userID, name string) (string, error) {
	ti, err := m.dir.types(ctx, campaignID)
	if err != nil {
		return "", err
	}
	if len(ti.itemIDs) == 0 {
		return "", apperror.NewBadRequest("This campaign has no item category to hold a new item. Add one first.")
	}
	e, err := m.svc.Create(ctx, campaignID, userID, entities.CreateEntityInput{Name: name, EntityTypeID: ti.itemIDs[0], IsPrivate: true})
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

// craftingLayoutPlacer puts the item block after the entry of every item
// page type's layout that lacks it.
type craftingLayoutPlacer struct{ svc entities.EntityService }

func (p *craftingLayoutPlacer) itemTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error) {
	return p.svc.GetEntityTypesByPresetCategory(ctx, campaignID, "item")
}

func (p *craftingLayoutPlacer) ItemBlockPlaced(ctx context.Context, campaignID string) (bool, error) {
	types, err := p.itemTypes(ctx, campaignID)
	if err != nil {
		return false, err
	}
	for _, et := range types {
		if !layoutHasBlock(et.Layout, crafting.ItemBlockType) {
			return false, nil
		}
	}
	return true, nil
}

func (p *craftingLayoutPlacer) PlaceItemBlock(ctx context.Context, campaignID string) (int, error) {
	types, err := p.itemTypes(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, et := range types {
		if layoutHasBlock(et.Layout, crafting.ItemBlockType) {
			continue
		}
		if err := p.svc.UpdateEntityTypeLayout(ctx, et.ID, withItemBlock(et.Layout)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// layoutHasBlock scans the saved layout, since containers keep their
// children in free-form config.
func layoutHasBlock(l entities.EntityTypeLayout, blockType string) bool {
	raw, err := json.Marshal(l)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), `"type":"`+blockType+`"`)
}

// withItemBlock puts the block right after the page's entry, where the
// signed mockup draws "How it's made"; a layout with no entry gets it as a
// full-width row at the end.
func withItemBlock(l entities.EntityTypeLayout) entities.EntityTypeLayout {
	block := entities.TemplateBlock{ID: "blk-crafting-item", Type: crafting.ItemBlockType}
	for r := range l.Rows {
		for c := range l.Rows[r].Columns {
			blocks := l.Rows[r].Columns[c].Blocks
			for i, b := range blocks {
				if b.Type == "entry" {
					out := append([]entities.TemplateBlock{}, blocks[:i+1]...)
					out = append(out, block)
					l.Rows[r].Columns[c].Blocks = append(out, blocks[i+1:]...)
					return l
				}
			}
		}
	}
	l.Rows = append(l.Rows, entities.TemplateRow{ID: "row-crafting-item", Columns: []entities.TemplateColumn{
		{ID: "col-crafting-item", Width: 12, Blocks: []entities.TemplateBlock{block}},
	}})
	return l
}

// craftingCampaigns says which campaigns a user directs: they own it, or
// its Owner granted them the Director's sight.
type craftingCampaigns struct{ svc campaigns.CampaignService }

// maxDirectedPages caps the campaign list read for one pack listing.
const maxDirectedPages = 5

func (c *craftingCampaigns) DirectedBy(ctx context.Context, userID string) ([]crafting.Named, error) {
	var out []crafting.Named
	for page := 1; page <= maxDirectedPages; page++ {
		list, total, err := c.svc.List(ctx, userID, campaigns.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return nil, err
		}
		for _, cp := range list {
			ok, err := c.Directs(ctx, cp.ID, userID)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, crafting.Named{ID: cp.ID, Name: cp.Name})
			}
		}
		if page*100 >= total {
			break
		}
	}
	return out, nil
}

func (c *craftingCampaigns) Directs(ctx context.Context, campaignID, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	m, err := c.svc.GetMember(ctx, campaignID, userID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == 404 {
			return false, nil
		}
		return false, err
	}
	if m == nil {
		return false, nil
	}
	if m.Role >= campaigns.RoleOwner {
		return true, nil
	}
	return c.svc.IsUserDmGranted(ctx, campaignID, userID)
}

// registerCraftingItemBlock adds "How it's made", the block an item page
// carries for crafting. It renders on item pages only; the widget then
// shows a player only the recipes their book already holds, and hides
// itself when there are none.
func registerCraftingItemBlock(blockRegistry *entities.BlockRegistry) {
	blockRegistry.Register(entities.BlockMeta{
		Type: crafting.ItemBlockType, Label: "How it's made", Icon: "fa-mortar-pestle",
		Description: "The crafting recipe that makes this item, and where it is used",
		Addon:       crafting.AddonSlug,
		Contexts:    []string{"template"},
		Singleton:   true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil || rc.EntityType == nil {
			return templ.NopComponent
		}
		if rc.EntityType.PresetCategory == nil || *rc.EntityType.PresetCategory != "item" {
			return templ.NopComponent
		}
		if rc.CC.MemberRole < campaigns.RolePlayer {
			return templ.NopComponent
		}
		return crafting.ItemBlockMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken, rc.CC.CanAuthorDmOnly())
	})
}
