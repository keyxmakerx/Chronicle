// Package calendar — wizard_handler.go serves the new-calendar wizard's
// steps: Start, the preset picker + review, and the import drop zone +
// review. Every route is Owner only (routes.go), matching the rest of this
// plugin's calendar-structure mutation routes.
//
// There is no server-side draft session for the wizard: a preset's
// step-to-step state is just re-fetched each time (PreviewPreset is a pure,
// deterministic read), and an uploaded import's parsed *ImportResult is
// round-tripped through a hidden form field between the import-preview and
// review/create steps, since the browser already holds the original file
// and re-uploading it a second time would need larger form plumbing for no
// benefit. Because that field is client-submitted, WizardCreate re-validates
// it (parseWizardImportJSON) rather than trusting it outright — see that
// function's own doc comment.
package calendar

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// WizardStart renders the wizard's Start step.
// GET /campaigns/:id/calendars/wizard
func (h *Handler) WizardStart(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	return middleware.Render(c, http.StatusOK, wizardShell("Start", wizardStartBody(cc.Campaign.ID)))
}

// WizardPresets renders the preset-picker step.
// GET /campaigns/:id/calendars/wizard/presets
func (h *Handler) WizardPresets(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	names, err := PresetNames()
	if err != nil {
		return apperror.NewInternal(err)
	}
	cards := make([]presetCardData, 0, len(names))
	for _, name := range names {
		ir, err := h.svc.PreviewPreset(ctx, name)
		if err != nil {
			return err
		}
		cards = append(cards, presetCardData{
			Name:         name,
			CalendarName: ir.CalendarName,
			Facts:        presetFacts(ir),
			BarWidths:    monthBarWidths(ir.Months),
		})
	}
	return middleware.Render(c, http.StatusOK, wizardShell("Choose a preset", wizardPresetsBody(cc.Campaign.ID, cards)))
}

// WizardPresetReview renders the Review step for one shipped preset.
// GET /campaigns/:id/calendars/wizard/presets/:name
func (h *Handler) WizardPresetReview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	name := c.Param("name")
	ir, err := h.svc.PreviewPreset(c.Request().Context(), name)
	if err != nil {
		return err
	}
	body := wizardReviewBody(cc.Campaign.ID, "preset", name, "", middleware.GetCSRFToken(c),
		fmt.Sprintf("/campaigns/%s/calendars/wizard/presets", cc.Campaign.ID), "", ir)
	return middleware.Render(c, http.StatusOK, wizardShell("Review", body))
}

// WizardImportStep renders the Import step's drop zone.
// GET /campaigns/:id/calendars/wizard/import
func (h *Handler) WizardImportStep(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	return middleware.Render(c, http.StatusOK, wizardShell("Import a file", wizardImportBody(cc.Campaign.ID, middleware.GetCSRFToken(c), "")))
}

// WizardImportPreview parses the uploaded file (a dry-run — no DB write, see
// CalendarService.PreviewImport's doc comment) and renders the Review step,
// carrying the parsed *ImportResult forward as JSON in a hidden field (see
// this file's package doc for why).
// POST /campaigns/:id/calendars/wizard/import/preview (multipart, field "file")
func (h *Handler) WizardImportPreview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	data, err := readCalendarImportFile(c)
	if err != nil {
		body := wizardImportBody(cc.Campaign.ID, middleware.GetCSRFToken(c), apperror.UserMessage(err, "couldn't read that file"))
		return middleware.Render(c, http.StatusOK, wizardShell("Import a file", body))
	}
	ir, err := h.svc.PreviewImport(c.Request().Context(), data)
	if err != nil {
		body := wizardImportBody(cc.Campaign.ID, middleware.GetCSRFToken(c), apperror.UserMessage(err, "couldn't understand that file"))
		return middleware.Render(c, http.StatusOK, wizardShell("Import a file", body))
	}

	importJSON, err := json.Marshal(ir)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal import result: %w", err))
	}
	reviewBody := wizardReviewBody(cc.Campaign.ID, "import", "", string(importJSON), middleware.GetCSRFToken(c),
		fmt.Sprintf("/campaigns/%s/calendars/wizard/import", cc.Campaign.ID), "", ir)
	return middleware.Render(c, http.StatusOK, wizardShell("Review", reviewBody))
}

// wizardCreateForm is WizardCreate's bound request shape.
type wizardCreateForm struct {
	Source       string
	PresetName   string
	ImportJSON   string
	Name         string
	CurrentYear  *int
	CurrentMonth *int
	CurrentDay   *int
}

// bindWizardCreateForm reads WizardCreate's multipart/urlencoded form
// fields. current_year/month/day are parsed leniently (an unparseable or
// absent value becomes nil, exactly like formIntPtr does for the JSON
// import-create route) so a genuinely missing value is reported by
// CreateCalendarFromImport's own #741 validation, not masked by a 400 here.
func bindWizardCreateForm(c echo.Context) wizardCreateForm {
	return wizardCreateForm{
		Source:       c.FormValue("source"),
		PresetName:   c.FormValue("preset_name"),
		ImportJSON:   c.FormValue("import_json"),
		Name:         c.FormValue("name"),
		CurrentYear:  formIntPtr(c, "current_year"),
		CurrentMonth: formIntPtr(c, "current_month"),
		CurrentDay:   formIntPtr(c, "current_day"),
	}
}

// parseWizardImportJSON re-derives an *ImportResult from the Review step's
// round-tripped import_json field. That field is ordinary client-submitted
// form data — a hidden input the browser sends back, not a value this
// handler ever re-reads from the original upload — so it is held to the
// same two checks an upload itself goes through rather than trusted as
// already-validated: the size cap every other import route enforces via
// readCalendarImportFile (isCalendarImportPath in internal/app/app.go skips
// the global body-limit middleware for this route specifically because it
// assumes this check exists), and clampCalendarStructure's structural
// bounds, which are otherwise only reached by parsing raw bytes through
// DetectAndParse. A tampered or hand-crafted import_json — more months than
// any calendar needs, a day count past what a month can hold — fails here
// exactly as it would have failed the upload it claims to be.
func parseWizardImportJSON(raw string) (*ImportResult, error) {
	if len(raw) > maxCalendarImportSize {
		return nil, fmt.Errorf("import data too large, maximum %d MB", maxCalendarImportSize/(1024*1024))
	}
	var parsed ImportResult
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, errors.New("the import couldn't be read back — try uploading the file again")
	}
	if err := clampCalendarStructure(&parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

// WizardCreate resolves the review step's source (a preset re-fetched by
// name, or an uploaded file's ImportResult carried forward as JSON), applies
// the confirmed name/date, and creates the calendar. On success it redirects
// to the calendars list (HTMXRedirect: a real navigation, not a swap, so the
// new card renders through the ordinary list read rather than needing an
// out-of-band insert) with ?created=<name>&new=<id> for the list page's
// toast + entrance animation. On failure it re-renders the Review step with
// the error and the caller's own submitted values, never the original
// preset/import defaults, so a correction doesn't lose what was already typed.
// POST /campaigns/:id/calendars/wizard/create
func (h *Handler) WizardCreate(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	form := bindWizardCreateForm(c)

	var ir *ImportResult
	var backURL string
	switch form.Source {
	case "preset":
		backURL = fmt.Sprintf("/campaigns/%s/calendars/wizard/presets", cc.Campaign.ID)
		fetched, err := h.svc.PreviewPreset(ctx, form.PresetName)
		if err != nil {
			return err
		}
		ir = fetched
	case "import":
		backURL = fmt.Sprintf("/campaigns/%s/calendars/wizard/import", cc.Campaign.ID)
		parsed, err := parseWizardImportJSON(form.ImportJSON)
		if err != nil {
			return apperror.NewBadRequest(err.Error())
		}
		ir = parsed
	default:
		return apperror.NewBadRequest("source must be \"preset\" or \"import\"")
	}

	// Reflect exactly what the owner submitted, not the source's own
	// defaults, so CreateCalendarFromImport validates (and, on failure, the
	// Review step redisplays) the values actually on screen.
	if form.Name != "" {
		ir.CalendarName = form.Name
	}
	opts := CreateCalendarFromImportOptions{
		CurrentYear:  form.CurrentYear,
		CurrentMonth: form.CurrentMonth,
		CurrentDay:   form.CurrentDay,
	}

	cal, err := h.svc.CreateCalendarFromImport(ctx, cc.Campaign.ID, ir, opts)
	if err != nil {
		display := *ir
		display.Today = ImportedToday{Year: valOr(form.CurrentYear, ir.Today.Year), Month: form.CurrentMonth, Day: form.CurrentDay}
		body := wizardReviewBody(cc.Campaign.ID, form.Source, form.PresetName, form.ImportJSON, middleware.GetCSRFToken(c),
			backURL, apperror.UserMessage(err, "couldn't create that calendar"), &display)
		return middleware.Render(c, http.StatusOK, wizardShell("Review", body))
	}

	redirect := fmt.Sprintf("/campaigns/%s/calendars?created=%s&new=%s",
		cc.Campaign.ID, url.QueryEscape(cal.Name), url.QueryEscape(cal.ID))
	return middleware.HTMXRedirect(c, redirect)
}

// valOr returns *p when p is non-nil, else fallback.
func valOr(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}
