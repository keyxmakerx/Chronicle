package app

import (
	"context"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// aiPageUndo is how Undo on an applied AI request reaches pages: the entity
// service for reads and deletes, and the page history's service for putting
// text back and bringing pages out of the Trash, so every put-back is
// versioned and broadcast like any other save.
type aiPageUndo struct {
	pages  entities.EntityService
	safety entities.PageSafetyService
}

var _ ai_workspace.PageUndo = aiPageUndo{}

func (u aiPageUndo) Read(ctx context.Context, campaignID, id string) (*entities.Entity, error) {
	e, err := u.pages.GetByID(ctx, id)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == 404 {
			return nil, nil
		}
		return nil, err
	}
	if e == nil || e.CampaignID != campaignID {
		return nil, nil
	}
	return e, nil
}

func (u aiPageUndo) RestoreText(ctx context.Context, id, name string, entry, entryHTML *string) error {
	return u.safety.RestoreText(ctx, id, name, entry, entryHTML)
}

func (u aiPageUndo) Inside(ctx context.Context, _, id string) (int, error) {
	kids, err := u.pages.GetChildren(ctx, id, permissions.RoleOwner, "")
	return len(kids), err
}

func (u aiPageUndo) Trash(ctx context.Context, id string) error { return u.pages.Delete(ctx, id) }

func (u aiPageUndo) RestoreFromTrash(ctx context.Context, campaignID, id string) error {
	_, err := u.safety.RestoreFromTrash(ctx, campaignID, id)
	return err
}
