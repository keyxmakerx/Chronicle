package entities

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PageLayoutService reads and writes a category's named layouts and the
// layout each page wears. Reads degrade: when the tables are unreadable a
// category reads as having only its main layout, so pages still draw.
type PageLayoutService interface {
	// ForType returns a category's named layouts.
	ForType(ctx context.Context, et *EntityType) PageLayouts
	// ForCampaign returns every category's named layouts that has any.
	ForCampaign(ctx context.Context, campaignID string) map[int]PageLayouts
	// PageKeys returns every page's own choice in a campaign, by page id.
	PageKeys(ctx context.Context, campaignID string) map[string]string
	// Worn returns the category's named layouts and the key of the one the
	// page wears.
	Worn(ctx context.Context, e *Entity, et *EntityType) (PageLayouts, string)
	// SetPageLayout makes a page wear the layout key names; the main key
	// clears the page's own choice so it follows the category again.
	SetPageLayout(ctx context.Context, e *Entity, et *EntityType, key string) error
	// SaveTypeLayouts stores a category's named layouts. The caller has
	// checked them, blocks included.
	SaveTypeLayouts(ctx context.Context, campaignID string, typeID int, p PageLayouts) error
	// RestorePageKey stores a page's own choice as a backup recorded it,
	// "main" included, without SetPageLayout's reading of who owns the page:
	// a restored page gets its owner back only later.
	RestorePageKey(ctx context.Context, e *Entity, et *EntityType, key string) error
}

// SetPageLayoutStore lets the Customize Save write a category's named layouts.
func (s *entityService) SetPageLayoutStore(p PageLayoutService) { s.pageLayouts = p }

// SaveCategoryPageLayouts checks a category's named layouts as the Customize
// Save does, blocks included, and stores them. A restore uses it.
func (s *entityService) SaveCategoryPageLayouts(ctx context.Context, campaignID string, typeID int, p PageLayouts) error {
	if err := s.checkPageLayouts(ctx, typeID, p); err != nil {
		return err
	}
	return s.pageLayouts.SaveTypeLayouts(ctx, campaignID, typeID, p)
}

type pageLayoutService struct {
	repo PageLayoutRepository
}

// NewPageLayoutService creates the named layouts service.
func NewPageLayoutService(repo PageLayoutRepository) PageLayoutService {
	return &pageLayoutService{repo: repo}
}

func (s *pageLayoutService) ForType(ctx context.Context, et *EntityType) PageLayouts {
	if et == nil {
		return PageLayouts{}
	}
	p, err := s.repo.TypeLayouts(ctx, et.ID)
	if err != nil {
		slog.Warn("page layouts: reading a category's layouts failed", slog.Int("type_id", et.ID), slog.Any("error", err))
		return PageLayouts{}
	}
	return p
}

func (s *pageLayoutService) ForCampaign(ctx context.Context, campaignID string) map[int]PageLayouts {
	m, err := s.repo.CampaignTypeLayouts(ctx, campaignID)
	if err != nil {
		slog.Warn("page layouts: listing a campaign's layouts failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return map[int]PageLayouts{}
	}
	return m
}

func (s *pageLayoutService) PageKeys(ctx context.Context, campaignID string) map[string]string {
	m, err := s.repo.CampaignPageKeys(ctx, campaignID)
	if err != nil {
		slog.Warn("page layouts: listing pages' layouts failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return map[string]string{}
	}
	return m
}

func (s *pageLayoutService) Worn(ctx context.Context, e *Entity, et *EntityType) (PageLayouts, string) {
	p := s.ForType(ctx, et)
	if p.Empty() || e == nil {
		return p, MainLayoutKey
	}
	chosen, err := s.repo.PageKey(ctx, e.ID)
	if err != nil {
		slog.Warn("page layouts: reading a page's layout failed", slog.String("entity_id", e.ID), slog.Any("error", err))
	}
	return p, p.WornKey(e, chosen)
}

func (s *pageLayoutService) SetPageLayout(ctx context.Context, e *Entity, et *EntityType, key string) error {
	if e == nil || et == nil || e.EntityTypeID != et.ID {
		return apperror.NewNotFound("page not found")
	}
	p, err := s.repo.TypeLayouts(ctx, et.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	stored := key
	if key == MainLayoutKey {
		// A player's character with a layout of its own to start on keeps
		// the main one only by choosing it.
		if !isPlayersCharacter(e) || p.Claimed == "" {
			stored = ""
		}
	} else if _, ok := p.Find(key); !ok {
		return apperror.NewBadRequest("that layout is not one of this category's")
	}
	if err := s.repo.SetPageKey(ctx, e.CampaignID, e.ID, stored); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *pageLayoutService) RestorePageKey(ctx context.Context, e *Entity, et *EntityType, key string) error {
	if e == nil || et == nil || e.EntityTypeID != et.ID {
		return apperror.NewNotFound("page not found")
	}
	if key != MainLayoutKey {
		p, err := s.repo.TypeLayouts(ctx, et.ID)
		if err != nil {
			return apperror.NewInternal(err)
		}
		if _, ok := p.Find(key); !ok {
			return apperror.NewBadRequest("that layout is not one of this category's")
		}
	}
	if err := s.repo.SetPageKey(ctx, e.CampaignID, e.ID, key); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (s *pageLayoutService) SaveTypeLayouts(ctx context.Context, campaignID string, typeID int, p PageLayouts) error {
	if err := p.Check(); err != nil {
		return err
	}
	if err := s.repo.SaveTypeLayouts(ctx, campaignID, typeID, p); err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}
