package app

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer/htmlconv"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// extensionPackPages creates an extension's entity-pack pages through the
// entities service, so they get the same checks, history and events as a
// page made by hand.
type extensionPackPages struct {
	svc entities.EntityService
}

// EntityTypeIDBySlug implements extensions.PackEntityCreator.
func (a extensionPackPages) EntityTypeIDBySlug(ctx context.Context, campaignID, slug string) (int, error) {
	et, err := a.svc.GetEntityTypeBySlug(ctx, campaignID, slug)
	if err != nil {
		return 0, err
	}
	return et.ID, nil
}

// CreatePackEntity implements extensions.PackEntityCreator. The body is
// Markdown, rendered through the same sanitising funnel the AI import uses.
// Writes are marked as an extension's, so WASM plugins do not hear a pack's
// pages one by one.
func (a extensionPackPages) CreatePackEntity(ctx context.Context, campaignID, userID string, typeID int, e extensions.PackEntry) (string, error) {
	ctx = changesource.With(ctx, changesource.Source{Kind: changesource.KindExtension, UserID: userID})
	ent, err := a.svc.Create(ctx, campaignID, userID, entities.CreateEntityInput{
		Name:         e.Name,
		EntityTypeID: typeID,
		TypeLabel:    e.TypeLabel,
		IsPrivate:    e.IsPrivate(),
		FieldsData:   e.Fields,
	})
	if err != nil {
		return "", err
	}
	if e.Entry == "" {
		return ent.ID, nil
	}
	html, err := importer.MarkdownToHTML(e.Entry)
	if err != nil {
		return ent.ID, fmt.Errorf("page body: %w", err)
	}
	doc, err := htmlconv.Convert(html)
	if err != nil {
		return ent.ID, fmt.Errorf("page body: %w", err)
	}
	if err := a.svc.UpdateEntry(ctx, ent.ID, doc, html); err != nil {
		return ent.ID, fmt.Errorf("page body: %w", err)
	}
	return ent.ID, nil
}
