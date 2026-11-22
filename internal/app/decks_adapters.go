package app

// decks_adapters.go bridges card decks to the entities and campaigns
// services. The plugin owns the narrow interfaces; the adapters live here so
// neither side imports the other.

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// decksHeroAdapter implements decks.Heroes over the entities service.
type decksHeroAdapter struct {
	svc entities.EntityService
}

func (a *decksHeroAdapter) Owned(ctx context.Context, campaignID, userID string) ([]decks.Hero, error) {
	if userID == "" {
		return nil, nil
	}
	list, err := a.svc.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]decks.Hero, 0, len(list))
	for _, e := range list {
		out = append(out, decks.Hero{ID: e.ID, Name: e.Name, OwnerUserID: userID})
	}
	return out, nil
}

// InCampaign finds a live page of this campaign. A page of another campaign
// is reported as absent, so a foreign id never resolves through this plugin.
func (a *decksHeroAdapter) InCampaign(ctx context.Context, campaignID, entityID string) (decks.Hero, bool, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		if isNotFound(err) {
			return decks.Hero{}, false, nil
		}
		return decks.Hero{}, false, err
	}
	if e == nil || e.CampaignID != campaignID {
		return decks.Hero{}, false, nil
	}
	h := decks.Hero{ID: e.ID, Name: e.Name}
	if e.OwnerUserID != nil {
		h.OwnerUserID = *e.OwnerUserID
	}
	return h, true, nil
}

// Viewable applies the page's own visibility rule to the viewer.
func (a *decksHeroAdapter) Viewable(ctx context.Context, campaignID, entityID string, v decks.Viewer) (bool, error) {
	if _, ok, err := a.InCampaign(ctx, campaignID, entityID); err != nil || !ok {
		return false, err
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, v.VisibilityRole, v.UserID)
	if err != nil {
		return false, err
	}
	return ep.CanView, nil
}

// Party lists the campaign's claimed pages, the party, as the viewer may see
// them.
func (a *decksHeroAdapter) Party(ctx context.Context, campaignID string, v decks.Viewer) ([]decks.Hero, error) {
	list, err := a.svc.ListClaimed(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]decks.Hero, 0, len(list))
	for _, e := range list {
		h := decks.Hero{ID: e.ID, Name: e.Name}
		if e.OwnerUserID != nil {
			h.OwnerUserID = *e.OwnerUserID
		}
		out = append(out, h)
	}
	return out, nil
}

// decksMemberAdapter implements decks.Members over the campaigns service.
type decksMemberAdapter struct {
	svc campaigns.CampaignService
}

// Members lists the campaign's people. A Director is the owner or anyone
// holding the campaign's DM-only grant, the same rule the request context
// uses.
func (a *decksMemberAdapter) Members(ctx context.Context, campaignID string) ([]decks.Member, error) {
	c, err := a.svc.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	list, err := a.svc.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	grants := map[string]bool{}
	for _, id := range c.ParseSettings().DmGrantIDs {
		grants[id] = true
	}
	out := make([]decks.Member, 0, len(list))
	for _, m := range list {
		out = append(out, decks.Member{UserID: m.UserID, Name: m.DisplayName, Role: int(m.Role),
			Director: m.Role >= campaigns.RoleOwner || grants[m.UserID], JoinedAt: m.JoinedAt})
	}
	return out, nil
}

// decksPictureAdapter implements decks.Pictures over the media service.
type decksPictureAdapter struct {
	svc media.MediaService
}

// Usable is true only for an image of this campaign that is not a note picture
// or page file and is not in the campaign Trash. Every other case, including a
// file that does not exist, is a plain false so the caller cannot tell them
// apart.
func (a *decksPictureAdapter) Usable(ctx context.Context, campaignID, mediaID string) (bool, error) {
	f, err := a.svc.GetByID(ctx, mediaID)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if f == nil || f.IsBound() || f.InTrash() || !f.IsImage() || f.CampaignID == nil || *f.CampaignID != campaignID {
		return false, nil
	}
	// A picture in the campaign Trash is still served until it is purged, so
	// the trash list is the only place its state shows.
	trashed, err := a.svc.ListTrashedPictures(ctx, campaignID)
	if err != nil {
		return false, err
	}
	for _, t := range trashed {
		if t.ID == mediaID {
			return false, nil
		}
	}
	return true, nil
}

// decksCardPictureAdapter implements media.CardPictureGuard over the decks
// service, so the media server decides a card's picture by the card's own
// visibility and neither plugin imports the other.
type decksCardPictureAdapter struct {
	svc decks.Service
}

func (a *decksCardPictureAdapter) IsCardPicture(ctx context.Context, campaignID, mediaID string) (bool, error) {
	return a.svc.IsCardPicture(ctx, campaignID, mediaID)
}

func decksViewerOf(v media.CardPictureViewer) decks.Viewer {
	return decks.Viewer{UserID: v.UserID, MemberRole: v.MemberRole, VisibilityRole: v.VisibilityRole, Director: v.Director}
}

func (a *decksCardPictureAdapter) CardPictureSeenBy(ctx context.Context, campaignID, mediaID string, v media.CardPictureViewer) (bool, error) {
	return a.svc.CardPictureSeenBy(ctx, campaignID, mediaID, decksViewerOf(v))
}

func (a *decksCardPictureAdapter) CardPictureIDs(ctx context.Context, campaignID string) ([]string, error) {
	return a.svc.CardPictureIDs(ctx, campaignID)
}

func (a *decksCardPictureAdapter) CardPictureUses(ctx context.Context, campaignID, mediaID string, v media.CardPictureViewer) ([]media.CardPictureUse, error) {
	uses, err := a.svc.CardPictureUses(ctx, campaignID, mediaID, decksViewerOf(v))
	if err != nil {
		return nil, err
	}
	out := make([]media.CardPictureUse, 0, len(uses))
	for _, u := range uses {
		out = append(out, media.CardPictureUse{DeckName: u.DeckName, CardName: u.CardName})
	}
	return out, nil
}

// wireCardPictures gives the media server, the sync media API and the media
// service's upload merge (so a player uploading a card picture's bytes is not
// told it exists) the guard that keeps a face-down card's picture from anyone
// it is not face up to. A named helper so a test can prove the production
// wiring sets both; without it a card's picture is one id away from any member.
func wireCardPictures(decksService decks.Service, mediaService media.MediaService, mediaHandler *media.Handler, mediaAPIHandler *syncapi.MediaAPIHandler) {
	g := &decksCardPictureAdapter{svc: decksService}
	mediaHandler.SetCardPictureGuard(g)
	mediaAPIHandler.SetCardPictureGuard(g)
	mediaService.SetCardPictureGuard(g)
}

// decksLivePublisher carries "your decks view changed" over the hub. The
// message has no payload; who gets it is the audience the service worked out:
// every member, named players (Directors join them by the hub's rule), or,
// with neither, Directors alone.
type decksLivePublisher struct {
	bus ws.EventBus
}

func (p *decksLivePublisher) Changed(campaignID string, to decks.Audience) {
	if p.bus == nil {
		return
	}
	msg := ws.NewMessage(ws.MsgDecksChanged, campaignID, "", nil)
	switch {
	case to.Members:
		msg.Public = true
	case len(to.Users) > 0:
		msg.AllowedUsers = to.Users
	default:
		msg.RequiresDM = true
	}
	p.bus.Publish(msg)
}

// decksBellAdapter writes the decks' bell entries through the notification
// service, which honours each person's notification choices.
type decksBellAdapter struct {
	notify interface {
		NotifyUsersWithDetail(ctx context.Context, userIDs []string, campaignID, ntype, message, detail, link string) error
	}
}

func (a *decksBellAdapter) Ring(ctx context.Context, campaignID string, e decks.BellEntry) error {
	return a.notify.NotifyUsersWithDetail(ctx, e.Users, campaignID, e.Kind, e.Message, "", e.Link)
}

// registerDecksBlock registers the Cards part of a hero's page, behind the
// Card decks add-on. What it draws is decided by the decks service for the
// viewer; a page they may not read the hand of draws nothing.
func registerDecksBlock(blockRegistry *entities.BlockRegistry, svc decks.Service) {
	blockRegistry.Register(entities.BlockMeta{
		Type: decks.BlockType, Label: "Cards", Icon: "fa-layer-group", Picture: entities.PictureGrid,
		Description: "The cards this hero holds, as the viewer may read them",
		Addon:       decks.AddonSlug,
		Contexts:    []string{"template"},
		TileSizes:   []string{"2x2", "4x2", "1x2"},
		Singleton:   true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		part, err := decks.HeroBlock(context.Background(), svc, rc.CC, rc.Entity.ID, rc.UserID)
		if err != nil {
			return templ.NopComponent
		}
		return part
	})
}
