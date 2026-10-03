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
	"strconv"

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
		Climates:     WeatherClimates,
	}
	ws, err := h.svc.GetWeatherSettings(c.Request().Context(), cal.ID, cc.Campaign.ID, viewerFrom(c, cc))
	if err != nil {
		return err
	}
	data.Weather = *ws
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

// bindWeatherCarry reads the editor's three weather fields against the stored
// settings. None sent keeps what is stored (an older form, or a client that
// only sends structure); any sent fills the others from storage, so an
// absent weather_kinds keeps the stored kinds and "[]" clears them. Invalid
// values are a bad-request the owner reads in the preview slot.
func (h *Handler) bindWeatherCarry(c echo.Context, cc *campaigns.CampaignContext, calID string) (weatherCarry, error) {
	climate, cont, kindsRaw := c.FormValue("weather_climate"), c.FormValue("weather_continuity"), c.FormValue("weather_kinds")
	if climate == "" && cont == "" && kindsRaw == "" {
		return weatherCarry{}, nil
	}
	cur, err := h.svc.GetWeatherSettings(c.Request().Context(), calID, cc.Campaign.ID, viewerFrom(c, cc))
	if err != nil {
		return weatherCarry{}, err
	}
	next := *cur
	if kindsRaw != "" {
		parsed, perr := parseWeatherKinds(kindsRaw)
		if perr != nil {
			return weatherCarry{}, perr
		}
		next.Kinds = parsed
	}
	if climate != "" {
		next.Climate = climate
	}
	if cont != "" {
		f, perr := strconv.ParseFloat(cont, 64)
		if perr != nil {
			return weatherCarry{}, apperror.NewBadRequest("how long weather lasts must be a number between 0 and 1")
		}
		next.Continuity = f
	}
	if err := validateWeatherSettings(next); err != nil {
		return weatherCarry{}, err
	}
	next.Continuity = roundContinuity(next.Continuity)
	cleaned, err := validateWeatherKinds(next.Kinds)
	if err != nil {
		return weatherCarry{}, err
	}
	wx := weatherCarry{
		Sent:       true,
		Climate:    next.Climate,
		Continuity: strconv.FormatFloat(next.Continuity, 'f', -1, 64),
		Kinds:      cleaned,
		KindsJSON:  encodeWeatherKinds(cleaned),
	}
	if next.Climate != cur.Climate {
		wx.Notes = append(wx.Notes, fmt.Sprintf("Weather: the climate changes to %s.", climateName(next.Climate)))
	}
	switch {
	case next.Continuity > cur.Continuity:
		wx.Notes = append(wx.Notes, "Weather: weather lasts longer.")
	case next.Continuity < cur.Continuity:
		wx.Notes = append(wx.Notes, "Weather: weather changes more often.")
	}
	wx.Notes = append(wx.Notes, weatherKindNotes(cur.Kinds, cleaned)...)
	return wx, nil
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
	var wx weatherCarry
	if err == nil {
		wx, err = h.bindWeatherCarry(c, cc, calID)
	}
	if err == nil {
		var p *StructurePreview
		p, err = h.svc.PreviewStructureEdit(c.Request().Context(), calID, cc.Campaign.ID, edit)
		if err == nil {
			return middleware.Render(c, http.StatusOK,
				structurePreviewBody(cc.Campaign.ID, calID, middleware.GetCSRFToken(c), raw, "", p, wx))
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
	var wx weatherCarry
	if err == nil {
		// Validated before the structure write so a bad climate can't leave
		// the structure saved and the weather refused.
		wx, err = h.bindWeatherCarry(c, cc, calID)
	}
	if err == nil {
		var p *StructurePreview
		p, err = h.svc.ApplyStructureEdit(c.Request().Context(), calID, cc.Campaign.ID, c.FormValue("fingerprint"), edit)
		if err == nil {
			if wx.Sent {
				cont, _ := strconv.ParseFloat(wx.Continuity, 64)
				err = h.svc.SetWeatherSettings(c.Request().Context(), calID, cc.Campaign.ID,
					WeatherSettings{Climate: wx.Climate, Continuity: cont, Kinds: wx.Kinds})
			}
			if err == nil {
				return middleware.HTMXRedirect(c, structureCalendarURL(cc.Campaign.ID, calID))
			}
		} else if apperror.SafeCode(err) == http.StatusConflict && p != nil {
			return middleware.Render(c, http.StatusOK,
				structurePreviewBody(cc.Campaign.ID, calID, middleware.GetCSRFToken(c), raw, apperror.UserMessage(err, ""), p, wx))
		}
	}
	if isOwnerFacingError(err) {
		return middleware.Render(c, http.StatusOK, structurePreviewError(apperror.UserMessage(err, "couldn't save that structure")))
	}
	return err
}
