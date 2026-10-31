package entities

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PinViewer is who the Pinned Pages widget is drawn for. Role is the
// visibility role the viewer reads pages with; Director is true for the owner
// and granted co-Directors, unless they are looking as a player.
type PinViewer struct {
	CampaignID string
	UserID     string
	Role       int
	Director   bool
}

// PinnedPages is what the widget shows one viewer: the Director's pins on
// top, then their own, each already narrowed to pages they can open.
type PinnedPages struct {
	Shared []PinnedPage
	Own    []PinnedPage
	// MoreOwn counts the viewer's own pins past the ones drawn.
	MoreOwn int
	// SharedAvailable is false while the Director's pins table is missing,
	// so the widget doesn't offer what can't be saved.
	SharedAvailable bool
}

// Empty reports whether there is nothing to draw.
func (p PinnedPages) Empty() bool { return len(p.Shared) == 0 && len(p.Own) == 0 }

// pinVisibility is the one question pins ask the page service: which of these
// pages may this viewer open.
type pinVisibility interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// PagePinService decides what the Pinned Pages widget shows and who may change
// it. A page a viewer can't open is left out for them without a trace.
type PagePinService interface {
	// ForViewer returns the widget's pages for one viewer.
	ForViewer(ctx context.Context, v PinViewer) (PinnedPages, error)
	// SharedAvailable is false while the Director's pins can't be stored.
	SharedAvailable() bool
	// IsShared reports whether a page is pinned for everyone.
	IsShared(ctx context.Context, campaignID, entityID string) (bool, error)
	// SetShared pins a page for everyone, or takes it off. Director only.
	SetShared(ctx context.Context, v PinViewer, entityID string, on bool) error
	// RemoveOwn takes a page off the viewer's own pins.
	RemoveOwn(ctx context.Context, v PinViewer, entityID string) error
}

type pagePinService struct {
	repo            PagePinRepository
	pages           pinVisibility
	sharedAvailable bool
}

// NewPagePinService creates the pins service. sharedAvailable is false when
// the page_pins table did not migrate: members' own pins still show, and the
// Director's are left out.
func NewPagePinService(repo PagePinRepository, pages pinVisibility, sharedAvailable bool) PagePinService {
	return &pagePinService{repo: repo, pages: pages, sharedAvailable: sharedAvailable}
}

func (s *pagePinService) ForViewer(ctx context.Context, v PinViewer) (PinnedPages, error) {
	out := PinnedPages{SharedAvailable: s.sharedAvailable}
	var shared, own []PinnedPage
	if s.sharedAvailable {
		var err error
		if shared, err = s.repo.Shared(ctx, v.CampaignID); err != nil {
			// The Director's pins are the extra; a member's own still show.
			slog.Warn("reading pinned pages", slog.String("campaign_id", v.CampaignID), slog.Any("error", err))
			shared = nil
		}
	}
	if v.UserID != "" {
		var err error
		if own, err = s.repo.Own(ctx, v.CampaignID, v.UserID); err != nil {
			return out, err
		}
	}
	if len(shared)+len(own) == 0 {
		return out, nil
	}

	ids := make([]string, 0, len(shared)+len(own))
	for _, p := range shared {
		ids = append(ids, p.ID)
	}
	for _, p := range own {
		ids = append(ids, p.ID)
	}
	can, err := s.pages.FilterViewableEntityIDs(ctx, v.CampaignID, ids, v.Role, v.UserID)
	if err != nil {
		return out, err
	}

	onTop := make(map[string]bool, len(shared))
	for _, p := range shared {
		if can[p.ID] {
			out.Shared = append(out.Shared, p)
			onTop[p.ID] = true
		}
	}
	for _, p := range own {
		// A page the Director pinned already sits above; it isn't listed twice.
		if !can[p.ID] || onTop[p.ID] {
			continue
		}
		if len(out.Own) == maxOwnPinsShown {
			out.MoreOwn++
			continue
		}
		out.Own = append(out.Own, p)
	}
	return out, nil
}

func (s *pagePinService) SharedAvailable() bool { return s.sharedAvailable }

func (s *pagePinService) IsShared(ctx context.Context, campaignID, entityID string) (bool, error) {
	if !s.sharedAvailable {
		return false, nil
	}
	return s.repo.IsShared(ctx, campaignID, entityID)
}

func (s *pagePinService) SetShared(ctx context.Context, v PinViewer, entityID string, on bool) error {
	if !v.Director {
		return apperror.NewForbidden("only the campaign owner or a granted co-Director pins pages for everyone")
	}
	if !s.sharedAvailable {
		return apperror.NewBadRequest("pinning pages for everyone is unavailable right now")
	}
	if !on {
		return s.repo.Unshare(ctx, v.CampaignID, entityID)
	}
	can, err := s.pages.FilterViewableEntityIDs(ctx, v.CampaignID, []string{entityID}, v.Role, v.UserID)
	if err != nil {
		return err
	}
	if !can[entityID] {
		return apperror.NewNotFound("page not found")
	}
	already, err := s.repo.IsShared(ctx, v.CampaignID, entityID)
	if err != nil || already {
		return err
	}
	n, err := s.repo.CountShared(ctx, v.CampaignID)
	if err != nil {
		return err
	}
	if n >= MaxSharedPins {
		return apperror.NewBadRequest(fmt.Sprintf("You can pin up to %d pages for everyone. Unpin one from the home page first.", MaxSharedPins))
	}
	return s.repo.Share(ctx, v.CampaignID, entityID, v.UserID)
}

func (s *pagePinService) RemoveOwn(ctx context.Context, v PinViewer, entityID string) error {
	if v.UserID == "" {
		return apperror.NewForbidden("sign in to change your pins")
	}
	return s.repo.RemoveOwn(ctx, v.CampaignID, entityID, v.UserID)
}
