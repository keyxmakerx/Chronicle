package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// aiFoldersAdapter implements records.FoldersAPI over the entities folder
// service, so the folder kind never touches the plugin's repositories.
type aiFoldersAdapter struct {
	folders entities.FolderService
	types   interface {
		GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	}
}

var _ records.FoldersAPI = aiFoldersAdapter{}

func (f aiFoldersAdapter) Categories(ctx context.Context, campaignID string) ([]records.FolderCategory, error) {
	types, err := f.types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]records.FolderCategory, 0, len(types))
	for _, t := range types {
		out = append(out, records.FolderCategory{ID: t.ID, Name: t.Name, Plural: t.NamePlural, Slug: t.Slug})
	}
	return out, nil
}

func (f aiFoldersAdapter) List(ctx context.Context, campaignID string) ([]records.FolderRef, error) {
	nodes, err := f.folders.ListAllFolders(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = true
	}
	out := make([]records.FolderRef, 0, len(nodes))
	for _, n := range nodes {
		ref := records.FolderRef{ID: n.ID, Name: n.Name, CategoryID: n.EntityTypeID}
		// A folder nested under a page has no folder parent to name.
		if n.ParentID != nil && ids[*n.ParentID] {
			ref.ParentID = *n.ParentID
		}
		out = append(out, ref)
	}
	return out, nil
}

func (f aiFoldersAdapter) Create(ctx context.Context, campaignID string, categoryID int, name, parentID string) error {
	_, err := f.folders.CreateFolder(ctx, campaignID, entities.CreateSidebarNodeInput{
		Name: name, EntityTypeID: categoryID, ParentID: parentID,
	})
	return err
}

func (f aiFoldersAdapter) Rename(ctx context.Context, campaignID, id, name string) error {
	return f.folders.RenameFolder(ctx, campaignID, id, name)
}

func (f aiFoldersAdapter) Move(ctx context.Context, campaignID, id string, parentID *string) error {
	return f.folders.MoveFolder(ctx, campaignID, id, parentID)
}

func (f aiFoldersAdapter) Delete(ctx context.Context, campaignID, id string) error {
	return f.folders.DeleteFolder(ctx, campaignID, id)
}

// aiFolderNames names folders by id for AI Export and the page lookups.
type aiFolderNames struct{ folders entities.FolderService }

func (n aiFolderNames) FolderNames(ctx context.Context, campaignID string) (map[string]string, error) {
	nodes, err := n.folders.ListAllFolders(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(nodes))
	for _, f := range nodes {
		out[f.ID] = f.Name
	}
	return out, nil
}
