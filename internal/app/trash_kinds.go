package app

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// gameNightTrash puts deleted game nights in the campaign Trash. Sessions
// already soft-delete; this lists them, restores them with the same "back
// on" notice as the night's own restore, and purges them with the pages.
type gameNightTrash struct {
	svc sessions.SessionService
}

func (g gameNightTrash) TrashKind() string { return "game-night" }

func (g gameNightTrash) ListTrashed(ctx context.Context, campaignID string) ([]entities.TrashedThing, error) {
	list, err := g.svc.ListTrashedSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]entities.TrashedThing, 0, len(list))
	for _, s := range list {
		out = append(out, entities.TrashedThing{
			Kind: "game-night", Label: "Game night", Icon: "fa-dice-d20",
			ID: s.ID, Name: s.Name, DeletedAt: s.DeletedAt,
		})
	}
	return out, nil
}

func (g gameNightTrash) RestoreTrashed(ctx context.Context, campaignID, id string) (string, error) {
	sess, err := g.svc.RestoreAndAnnounce(ctx, campaignID, id)
	if err != nil {
		return "", err
	}
	return "/campaigns/" + campaignID + "/sessions/" + sess.ID, nil
}

func (g gameNightTrash) PurgeTrashedBefore(ctx context.Context, cutoff time.Time) (int, error) {
	return g.svc.PurgeTrashedSessions(ctx, cutoff)
}

// pictureTrash puts deleted campaign pictures in the campaign Trash. A
// trashed picture keeps showing on its pages until the purge removes its
// files.
type pictureTrash struct {
	svc media.MediaService
}

func (p pictureTrash) TrashKind() string { return "picture" }

func (p pictureTrash) ListTrashed(ctx context.Context, campaignID string) ([]entities.TrashedThing, error) {
	list, err := p.svc.ListTrashedPictures(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]entities.TrashedThing, 0, len(list))
	for _, f := range list {
		out = append(out, entities.TrashedThing{
			Kind: "picture", Label: "Picture", Icon: "fa-image",
			ID: f.ID, Name: f.Name, DeletedAt: f.DeletedAt,
		})
	}
	return out, nil
}

func (p pictureTrash) RestoreTrashed(ctx context.Context, campaignID, id string) (string, error) {
	if err := p.svc.RestoreCampaignPicture(ctx, campaignID, id); err != nil {
		return "", err
	}
	return "/campaigns/" + campaignID + "/media", nil
}

func (p pictureTrash) PurgeTrashedBefore(ctx context.Context, cutoff time.Time) (int, error) {
	return p.svc.PurgeTrashedPictures(ctx, cutoff)
}
