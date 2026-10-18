package syncapi

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// SystemEntryLister lists a campaign's own pick-list entries.
// systems.SystemEntryService satisfies it; the service applies the
// visibility and secret-stripping rules for the actor it is given.
type SystemEntryLister interface {
	List(ctx context.Context, campaignID, fieldKey string, actor systems.EntryActor) ([]systems.SystemEntry, error)
}

// SetSystemEntryLister wires the system-entries read endpoint. Without it the
// endpoint answers 404.
func (h *APIHandler) SetSystemEntryLister(l SystemEntryLister) { h.systemEntries = l }

// apiSystemEntry is what leaves for Foundry: no visibility, author or ids,
// since only player-visible entries are ever returned.
type apiSystemEntry struct {
	FieldKey    string         `json:"fieldKey"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// ListSystemEntries returns the campaign's own pick-list entries (a homebrew
// ancestry, kit...) so the Foundry module can fill the item it makes for one.
// Every caller gets the player view, the Owner's key included: the module
// writes these into Foundry items that players can open, and GM-only content
// never goes into a saved Foundry document. So Directors-only entries are
// left out and secret spans are stripped by the service.
// GET /api/v1/campaigns/:id/system-entries?fieldKey=ancestry
func (h *APIHandler) ListSystemEntries(c echo.Context) error {
	if h.systemEntries == nil {
		return apperror.NewNotFound("system entries are not available")
	}
	list, err := h.systemEntries.List(c.Request().Context(), c.Param("id"), c.QueryParam("fieldKey"), systems.EntryActor{})
	if err != nil {
		return err
	}
	out := make([]apiSystemEntry, 0, len(list))
	for _, e := range list {
		// The service already refuses these to a non-Director; checked again
		// here so a change there cannot leak a draft into Foundry.
		if e.Visibility != systems.EntryVisibilityEveryone {
			continue
		}
		props := e.Properties
		if props == nil {
			props = map[string]any{}
		}
		out = append(out, apiSystemEntry{
			FieldKey: e.FieldKey, Slug: e.Slug, Name: e.Name, Summary: e.Summary,
			Description: e.Description, Properties: props, UpdatedAt: e.UpdatedAt,
		})
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"data": out, "total": len(out)})
}
