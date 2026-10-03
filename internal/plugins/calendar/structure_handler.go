// Package calendar - structure_handler.go serves an existing calendar's
// "Edit structure" page and its preview and save. Owner only (routes.go),
// like the wizard and PUT /calendars/:calid. Thin: bind, call the service,
// render.
package calendar

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// StructureEditPage renders the editor prefilled from the calendar.
// GET /campaigns/:id/calendars/:calid/structure
func (h *Handler) StructureEditPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	cal, err := h.svc.GetCalendarForViewer(c.Request().Context(), c.Param("calid"), cc.Campaign.ID, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	data := StructureEditData{
		CampaignID:   cc.Campaign.ID,
		CampaignName: cc.Campaign.Name,
		Calendar:     cal,
		CSRFToken:    middleware.GetCSRFToken(c),
		RealTime:     cal.UsesRealTime(),
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, StructureEditFragment(data))
	}
	return middleware.Render(c, http.StatusOK, StructureEditPage(data))
}

// bindStructureEdit re-validates the editor's submission through the same
// size cap and clamp a wizard build gets (parseWizardImportJSON): it is
// browser-submitted, on the preview and again on the save.
func bindStructureEdit(c echo.Context) (StructureEdit, string, error) {
	ir, err := parseWizardImportJSON(c.FormValue("import_json"))
	if err != nil {
		return StructureEdit{}, "", apperror.NewBadRequest(err.Error())
	}
	raw, err := json.Marshal(ir)
	if err != nil {
		return StructureEdit{}, "", apperror.NewInternal(fmt.Errorf("marshal structure: %w", err))
	}
	return StructureEditFromImport(ir), string(raw), nil
}

// isOwnerFacingError reports whether err is a refusal the owner should read
// in the preview slot (bad input, a stale preview, a real-time calendar),
// rather than a not-found or server failure for the error handler.
func isOwnerFacingError(err error) bool {
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		return false
	}
	switch ae.Code {
	case http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// StructureEditPreview shows what saving the submitted structure would do.
// POST /campaigns/:id/calendars/:calid/structure/preview
func (h *Handler) StructureEditPreview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	calID := c.Param("calid")
	edit, raw, err := bindStructureEdit(c)
	if err == nil {
		var p *StructurePreview
		p, err = h.svc.PreviewStructureEdit(c.Request().Context(), calID, cc.Campaign.ID, edit)
		if err == nil {
			return middleware.Render(c, http.StatusOK,
				structurePreviewBody(cc.Campaign.ID, calID, middleware.GetCSRFToken(c), raw, "", p))
		}
	}
	if isOwnerFacingError(err) {
		return middleware.Render(c, http.StatusOK, structurePreviewError(apperror.UserMessage(err, "couldn't preview that structure")))
	}
	return err
}

// StructureEditApply saves the previewed structure and sends the owner to
// the calendar. If the calendar changed since the preview, nothing is
// written and the up-to-date preview replaces the old one.
// POST /campaigns/:id/calendars/:calid/structure
func (h *Handler) StructureEditApply(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	calID := c.Param("calid")
	edit, raw, err := bindStructureEdit(c)
	if err == nil {
		var p *StructurePreview
		p, err = h.svc.ApplyStructureEdit(c.Request().Context(), calID, cc.Campaign.ID, c.FormValue("fingerprint"), edit)
		if err == nil {
			return middleware.HTMXRedirect(c, structureCalendarURL(cc.Campaign.ID, calID))
		}
		if apperror.SafeCode(err) == http.StatusConflict && p != nil {
			return middleware.Render(c, http.StatusOK,
				structurePreviewBody(cc.Campaign.ID, calID, middleware.GetCSRFToken(c), raw, apperror.UserMessage(err, ""), p))
		}
	}
	if isOwnerFacingError(err) {
		return middleware.Render(c, http.StatusOK, structurePreviewError(apperror.UserMessage(err, "couldn't save that structure")))
	}
	return err
}
