package app

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
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
