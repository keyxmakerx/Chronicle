package syncapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// FolderLister lists a campaign's sidebar folders. The Foundry module mirrors
// them, nested under their categories, as Foundry journal folders.
type FolderLister interface {
	ListAllFolders(ctx context.Context, campaignID string) ([]entities.SidebarNode, error)
}

// SetFolderLister wires the folder list. Until it is set the route answers
// 404, which the module reads as "file pages by category only".
func (h *APIHandler) SetFolderLister(fl FolderLister) {
	h.folders = fl
}

// apiFolder is one sidebar folder as the API returns it. ParentID is another
// folder's id, a page's id, or absent for a folder at the top of its category.
type apiFolder struct {
	ID           string  `json:"id"`
	EntityTypeID int     `json:"entity_type_id"`
	Name         string  `json:"name"`
	ParentID     *string `json:"parent_id,omitempty"`
	SortOrder    int     `json:"sort_order"`
}

// ListFolders handles GET /api/v1/campaigns/:id/folders. Only folders in
// categories this key may see are listed, the same rule as /entity-types.
func (h *APIHandler) ListFolders(c echo.Context) error {
	if h.folders == nil {
		return apperror.NewNotFound("this Chronicle does not list folders")
	}
	ctx := c.Request().Context()
	campaignID := c.Param("id")
	types, err := h.entitySvc.GetEntityTypes(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to list entity types for folders", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list folders"))
	}
	types = entities.TypesForViewer(types, h.visibilityRoleFor(ctx, campaignID, h.resolveUserID(c), h.resolveRole(c)))
	visible := make(map[int]bool, len(types))
	for _, t := range types {
		visible[t.ID] = true
	}
	nodes, err := h.folders.ListAllFolders(ctx, campaignID)
	if err != nil {
		slog.Error("api: failed to list folders", slog.Any("error", err))
		return apperror.NewInternal(fmt.Errorf("failed to list folders"))
	}
	out := make([]apiFolder, 0, len(nodes))
	for _, n := range nodes {
		if !visible[n.EntityTypeID] {
			continue
		}
		out = append(out, apiFolder{ID: n.ID, EntityTypeID: n.EntityTypeID, Name: n.Name, ParentID: n.ParentID, SortOrder: n.SortOrder})
	}
	return c.JSON(http.StatusOK, map[string]any{"data": out, "total": len(out)})
}
