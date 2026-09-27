// Package calendar — wizard_handler.go serves the new-calendar wizard's
// steps: Start, the preset picker + review, the import drop zone + review,
// the real-world calendar's own review, and the "Build your own" structure
// editor + review. Every route is Owner only (routes.go), matching the rest
// of this plugin's calendar-structure mutation routes.
//
// There is no server-side draft session for the wizard: a preset's
// step-to-step state is just re-fetched each time (PreviewPreset is a pure,
// deterministic read), the real-world calendar's structure is rebuilt fresh
// server-side every time too (GregorianImportResult), and an uploaded
// import's or a hand-built calendar's parsed *ImportResult is round-tripped
// through a hidden form field between its preview and review/create steps,
// since the browser already holds the original file (or Alpine state) and
// re-sending it a second time would need larger form plumbing for no
// benefit. Because that field is client-submitted, WizardCreate re-validates
// it (parseWizardImportJSON) rather than trusting it outright — see that
// function's own doc comment.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// timezoneLookup is the minimal auth surface the real-world calendar step
// needs — a consumer-defined interface (not auth.AuthService itself), so
// this plugin depends on exactly the one method it calls rather than the
// full auth service (plugins reach each other only through service
// interfaces — never a wider one than the call site needs). Satisfied by
// *auth.authService without either package importing the other's concrete
// type.
type timezoneLookup interface {
	GetUser(ctx context.Context, userID string) (*auth.User, error)
}

// SetTimezoneLookup wires the optional owner-account-timezone lookup used to
// suggest a starting default on the real-world calendar's review step (see
// WizardRealWorldReview). Wiring it is a setter, not a NewHandler parameter,
// so every existing caller of NewHandler (production and tests alike) is
// unaffected; a Handler with no lookup wired simply falls back to the
// browser's own detection or UTC.
func (h *Handler) SetTimezoneLookup(l timezoneLookup) {
	h.userZones = l
}

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

// WizardRealWorldReview renders the Review step for the "Real-world
// calendar" tile: its structure is fixed (GregorianImportResult, never
// client-controlled — see this file's package doc), so unlike the preset
// and import sources there is no separate picker/upload step first.
// GET /campaigns/:id/calendars/wizard/reallife
func (h *Handler) WizardRealWorldReview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ir, err := GregorianImportResult()
	if err != nil {
		return apperror.NewInternal(err)
	}
	body := wizardRealWorldReviewBody(cc.Campaign.ID, middleware.GetCSRFToken(c),
		fmt.Sprintf("/campaigns/%s/calendars/wizard", cc.Campaign.ID), "", h.ownerZoneHint(c), timeutil.CommonZones(), ir)
	return middleware.Render(c, http.StatusOK, wizardShell("Review", body))
}

// ownerZoneHint returns the requesting owner's stored account timezone, or
// "" when none is set, the lookup was never wired (SetTimezoneLookup), or
// the lookup fails — this is a starting-default nicety, never something a
// failure here should surface as an error (the review step's own Intl
// detection and the plain "UTC" fallback both still work with "").
func (h *Handler) ownerZoneHint(c echo.Context) string {
	if h.userZones == nil {
		return ""
	}
	userID := auth.GetUserID(c)
	if userID == "" {
		return ""
	}
	u, err := h.userZones.GetUser(c.Request().Context(), userID)
	if err != nil || u == nil || u.Timezone == nil {
		return ""
	}
	return *u.Timezone
}

// WizardBuildStep renders the "Build your own" structure editor.
// GET /campaigns/:id/calendars/wizard/build
func (h *Handler) WizardBuildStep(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	return middleware.Render(c, http.StatusOK, wizardShell("Build your own", wizardBuildBody(cc.Campaign.ID, middleware.GetCSRFToken(c), "")))
}

// WizardBuildPreview re-validates the structure the browser just built
// (Alpine-serialized into import_json exactly like the Review step's own
// round-tripped field — see parseWizardImportJSON's doc comment for why
// this can't be trusted outright) and renders the shared Review step. A
// failure here re-renders the editor with fresh defaults rather than the
// owner's in-progress edits: parseWizardImportJSON only rejects an
// oversized payload, unparsable JSON, or a structure over the same limits
// checkCalendarImportLimits enforces everywhere else — none of which the
// editor's own client-side soft caps let a normal session reach, so this
// path is realistically only hit by a tampered request, not a lost edit.
// POST /campaigns/:id/calendars/wizard/build/preview
func (h *Handler) WizardBuildPreview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ir, err := parseWizardImportJSON(c.FormValue("import_json"))
	if err != nil {
		body := wizardBuildBody(cc.Campaign.ID, middleware.GetCSRFToken(c), apperror.UserMessage(err, "couldn't build that calendar"))
		return middleware.Render(c, http.StatusOK, wizardShell("Build your own", body))
	}
	importJSON, err := json.Marshal(ir)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal import result: %w", err))
	}
	reviewBody := wizardReviewBody(cc.Campaign.ID, "build", "", string(importJSON), middleware.GetCSRFToken(c),
		fmt.Sprintf("/campaigns/%s/calendars/wizard/build", cc.Campaign.ID), "", ir)
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
	// RealTimeZone/TracksRealTime are only meaningful for source="reallife" —
	// see WizardCreate's own case.
	RealTimeZone   string
	TracksRealTime bool
}

// bindWizardCreateForm reads WizardCreate's multipart/urlencoded form
// fields. current_year/month/day are parsed leniently (an unparseable or
// absent value becomes nil, exactly like formIntPtr does for the JSON
// import-create route) so a genuinely missing value is reported by
// CreateCalendarFromImport's own #741 validation, not masked by a 400 here.
func bindWizardCreateForm(c echo.Context) wizardCreateForm {
	return wizardCreateForm{
		Source:         c.FormValue("source"),
		PresetName:     c.FormValue("preset_name"),
		ImportJSON:     c.FormValue("import_json"),
		Name:           c.FormValue("name"),
		CurrentYear:    formIntPtr(c, "current_year"),
		CurrentMonth:   formIntPtr(c, "current_month"),
		CurrentDay:     formIntPtr(c, "current_day"),
		RealTimeZone:   c.FormValue("real_time_zone"),
		TracksRealTime: c.FormValue("tracks_real_time") == "true",
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

// WizardCreate resolves the review step's source — a preset re-fetched by
// name, an uploaded file's or hand-built calendar's ImportResult carried
// forward as JSON, or the fixed real-world Gregorian structure — applies the
// confirmed name/date, and creates the calendar. On success it redirects to
// the calendars list (HTMXRedirect: a real navigation, not a swap, so the
// new card renders through the ordinary list read rather than needing an
// out-of-band insert) with ?created=<name>&new=<id> for the list page's
// toast + entrance animation. On failure it re-renders the Review step with
// the error and the caller's own submitted values, never the original
// source's defaults, so a correction doesn't lose what was already typed.
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
	case "build":
		// "Build your own" hands the SAME re-validation the import path gets
		// (parseWizardImportJSON: size cap, clampCalendarStructure) — its
		// import_json is exactly as client-controlled as an uploaded file's.
		backURL = fmt.Sprintf("/campaigns/%s/calendars/wizard/build", cc.Campaign.ID)
		parsed, err := parseWizardImportJSON(form.ImportJSON)
		if err != nil {
			return apperror.NewBadRequest(err.Error())
		}
		ir = parsed
	case "reallife":
		// The structure itself is never client-controlled (rebuilt fresh
		// here, not round-tripped) — only the zone/tracks-flag/date fields
		// below come from the browser, and CreateCalendarFromImport's own
		// checks (time.LoadLocation, validateImportCurrentDate) validate
		// them the same way an enabled real-time calendar's update route does.
		backURL = fmt.Sprintf("/campaigns/%s/calendars/wizard", cc.Campaign.ID)
		built, err := GregorianImportResult()
		if err != nil {
			return apperror.NewInternal(err)
		}
		ir = built
		ir.Settings.TracksRealTime = form.TracksRealTime
		if form.RealTimeZone != "" {
			zone := form.RealTimeZone
			ir.Settings.RealTimeZone = &zone
		}
		if form.TracksRealTime {
			// The date selects are hidden client-side while this switch is
			// on (today comes from the clock), so there is nothing in
			// form.CurrentYear/Month/Day to trust — compute "today" from the
			// chosen zone's wall clock here instead. A zone that fails to
			// load falls through to CreateCalendarFromImport's own
			// real_time_zone check below (with a harmless year-1/Jan-1
			// placeholder), so the owner sees THAT precise error rather than
			// a vaguer "current_month is required".
			y, m, d := 1, 1, 1
			if loc, zerr := time.LoadLocation(form.RealTimeZone); zerr == nil {
				yy, mm, dd := time.Now().In(loc).Date()
				y, m, d = yy, int(mm), dd
			}
			form.CurrentYear, form.CurrentMonth, form.CurrentDay = &y, &m, &d
		}
	default:
		return apperror.NewBadRequest("source must be \"preset\", \"import\", \"build\" or \"reallife\"")
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
		errMsg := apperror.UserMessage(err, "couldn't create that calendar")
		var body templ.Component
		if form.Source == "reallife" {
			body = wizardRealWorldReviewBody(cc.Campaign.ID, middleware.GetCSRFToken(c), backURL, errMsg,
				form.RealTimeZone, timeutil.CommonZones(), &display)
		} else {
			body = wizardReviewBody(cc.Campaign.ID, form.Source, form.PresetName, form.ImportJSON, middleware.GetCSRFToken(c),
				backURL, errMsg, &display)
		}
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
