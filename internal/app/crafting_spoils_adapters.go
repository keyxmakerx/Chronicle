package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// craftingPagesAdapter gives crafting any page of a campaign, by name and
// category, for drop lists, and whether a viewer may open one. A page in
// another campaign is no page at all.
type craftingPagesAdapter struct {
	entities entities.EntityService
}

func (a *craftingPagesAdapter) entity(ctx context.Context, campaignID, id string) (*entities.Entity, error) {
	if id == "" {
		return nil, nil
	}
	e, err := a.entities.GetByID(ctx, id)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if e == nil || e.CampaignID != campaignID {
		return nil, nil
	}
	return e, nil
}

func (a *craftingPagesAdapter) Page(ctx context.Context, campaignID, id string) (*crafting.PageInfo, error) {
	e, err := a.entity(ctx, campaignID, id)
	if err != nil || e == nil {
		return nil, err
	}
	kind := e.TypeName
	if kind == "" {
		kind = "Page"
	}
	return &crafting.PageInfo{ID: e.ID, Name: e.Name, Kind: kind}, nil
}

func (a *craftingPagesAdapter) CanSee(ctx context.Context, campaignID, id string, role int, userID string) (bool, error) {
	e, err := a.entity(ctx, campaignID, id)
	if err != nil || e == nil {
		return false, err
	}
	access, err := a.entities.CheckEntityAccess(ctx, e.ID, role, userID)
	if err != nil {
		return false, err
	}
	return access != nil && access.CanView, nil
}

// craftingFightListener drops what defeated creatures leave into the
// crafting spoils tray when a fight on the DM Screen ends, in campaigns
// with crafting on, and tells the Fight tab which creatures have a drop
// list.
type craftingFightListener struct {
	svc     crafting.Service
	enabled func(ctx context.Context, campaignID string) bool
}

// craftingFightViewer is the fight's viewer as crafting sees them. The
// Fight tab is the DM team's alone (Owner sight), which is crafting's
// Director.
func craftingFightViewer(v encounters.Viewer) crafting.Viewer {
	return crafting.Viewer{UserID: v.UserID, MemberRole: v.VisibilityRole, VisibilityRole: v.VisibilityRole,
		Director: v.VisibilityRole >= permissions.RoleOwner}
}

func (l *craftingFightListener) on(ctx context.Context, campaignID string) bool {
	return l.enabled == nil || l.enabled(ctx, campaignID)
}

func (l *craftingFightListener) Leaves(ctx context.Context, campaignID string, v encounters.Viewer, pageIDs []string) (map[string]string, error) {
	if !l.on(ctx, campaignID) {
		return nil, nil
	}
	return l.svc.DropNotes(ctx, campaignID, craftingFightViewer(v), pageIDs)
}

func (l *craftingFightListener) FightEnded(ctx context.Context, campaignID string, v encounters.Viewer, f encounters.EndedFight) (*encounters.EndReport, error) {
	if !l.on(ctx, campaignID) {
		return nil, nil
	}
	fight := crafting.Fight{Name: f.Name, Creatures: make([]crafting.FightCreature, 0, len(f.Creatures))}
	for _, c := range f.Creatures {
		fight.Creatures = append(fight.Creatures, crafting.FightCreature{PageID: c.EntityID, Name: c.Name, Down: c.Down, Copies: c.Copies})
	}
	got, err := l.svc.FightEnded(ctx, campaignID, craftingFightViewer(v), fight)
	if err != nil || got == nil {
		return nil, err
	}
	out := &encounters.EndReport{Lines: got.Lines}
	if got.Spoils > 0 {
		out.Link = "/campaigns/" + url.PathEscape(campaignID) + "/crafting#spoils"
		out.LinkText = "Open the spoils tray"
		out.Note = "Players see the spoils on their character pages now."
	}
	return out, nil
}
