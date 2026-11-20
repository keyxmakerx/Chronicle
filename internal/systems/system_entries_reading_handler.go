package systems

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The writes that let a campaign read its system in its own words. Each only
// binds, states who is asking and calls the service, which refuses anyone
// but a Director.

// RewriteAPI copies a standard entry into a new Directors-only campaign
// entry tied to it.
//
// POST /campaigns/:id/system-entries/rewrite
func (h *SystemEntryHandler) RewriteAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in RewriteInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	e, err := h.svc.Rewrite(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, map[string]any{"entry": e})
}

// TieAPI lets an existing campaign entry take a standard entry's place.
//
// POST /campaigns/:id/system-entries/:entryId/tie  {"slug": "dwarf"}
func (h *SystemEntryHandler) TieAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := entryID(c)
	if err != nil {
		return err
	}
	var in struct {
		Slug string `json:"slug"`
	}
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	e, err := h.svc.Tie(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id, in.Slug)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"entry": e})
}

// UntieAPI makes a tied entry the campaign's own again.
//
// DELETE /campaigns/:id/system-entries/:entryId/tie
func (h *SystemEntryHandler) UntieAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := entryID(c)
	if err != nil {
		return err
	}
	e, err := h.svc.Untie(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"entry": e})
}

// SeenAPI marks the package's change to the standard entry as seen.
//
// POST /campaigns/:id/system-entries/:entryId/seen
func (h *SystemEntryHandler) SeenAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := entryID(c)
	if err != nil {
		return err
	}
	e, err := h.svc.MarkSeen(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"entry": e})
}

// LeaveOutAPI leaves a standard entry out, or changes its sentence.
//
// PUT /campaigns/:id/system-entries/left-out
func (h *SystemEntryHandler) LeaveOutAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var in LeftOutInput
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	l, err := h.svc.LeaveOut(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"leftOut": l})
}

// BringBackAPI offers a left-out standard entry again.
//
// DELETE /campaigns/:id/system-entries/left-out/:fieldKey/:slug
func (h *SystemEntryHandler) BringBackAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.svc.BringBack(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), c.Param("fieldKey"), c.Param("slug")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// FreeOldNameAPI lets another entry take an old name.
//
// DELETE /campaigns/:id/system-entries/old-names/:nameId
func (h *SystemEntryHandler) FreeOldNameAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := strconv.ParseInt(c.Param("nameId"), 10, 64)
	if err != nil || id <= 0 {
		return apperror.NewBadRequest("invalid old name id")
	}
	if err := h.svc.FreeOldName(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// AddNoteAPI records a line the players were told about an entry.
//
// POST /campaigns/:id/system-entries/:entryId/notes  {"line": "..."}
func (h *SystemEntryHandler) AddNoteAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	id, err := entryID(c)
	if err != nil {
		return err
	}
	var in struct {
		Line string `json:"line"`
	}
	if err := readEntryBody(c, &in); err != nil {
		return err
	}
	n, err := h.svc.AddNote(c.Request().Context(), cc.Campaign.ID, entryActor(c, cc), id, in.Line)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, map[string]any{"note": n})
}
