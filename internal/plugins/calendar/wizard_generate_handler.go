// Package calendar — wizard_generate_handler.go serves the "Generate one"
// step: a client-side recipe sheet that drives the generator engine
// (static/js/widgets/chronicle_gen.js, loaded on demand only for this step
// — see wizardGenerateBody's own doc comment for why) so the owner can reach
// the wizard's shared Review step from a fresh, complete calendar without
// typing anything.
//
// The server never runs the generator itself — generation is pure client-
// side JavaScript, deterministic by seed, with nothing for Go to compute or
// import Echo-free business logic over. This file's only job is what every
// other wizard step's Preview handler already does: re-validate whatever the
// browser produced before the Review step ever renders it. A generated
// preset gets exactly the same treatment an uploaded file gets — the same
// Chronicle-format parser, the same clampCalendarStructure/
// checkCalendarImportLimits bounds (parseGeneratedCalendarJSON, reusing
// CalendarService.PreviewImport rather than a bespoke check) — because it
// says "chronicle-calendar-v1" no more trustworthy than an upload claiming
// the same thing.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// maxGeneratedCalendarSize caps the Generate step's client-serialised
// preset. A real generated calendar (bounded by the engine's own recipe
// schema: a couple dozen months/weekdays at most, a handful of moons/
// seasons, one era) is a few KB of JSON — this is a generous multiple of
// that, existing only to give a tampered payload a clear error before
// parseChronicle ever runs. Unlike the wizard's other import routes, this
// one is deliberately NOT in app.go's calendarImportPathPattern skip list:
// a generated calendar never needs anything close to their 10MB allowance,
// so it stays under Echo's ordinary global 2MB body limit, and this is the
// friendlier error for a payload well under that but still absurd for
// anything the generator itself would ever produce.
const maxGeneratedCalendarSize = 256 * 1024

// parseGeneratedCalendarJSON re-derives an *ImportResult from the Generate
// step's client-side engine output (ChronicleGen.cal.toPreset()'s
// chronicle-calendar-v1 shape, posted as an ordinary form field) through
// CalendarService.PreviewImport — the SAME auto-detecting parser and
// clamp/limits (clampCalendarStructure, checkCalendarImportLimits) an
// uploaded Chronicle export already goes through, not a second check
// invented for this path. Never trusted just because Chronicle's own
// generator produced it: the browser is exactly as untrusted here as
// everywhere else in this wizard.
func parseGeneratedCalendarJSON(ctx context.Context, svc CalendarService, raw string) (*ImportResult, error) {
	if len(raw) > maxGeneratedCalendarSize {
		return nil, apperror.NewBadRequest(fmt.Sprintf("generated calendar data too large, maximum %d KB", maxGeneratedCalendarSize/1024))
	}
	return svc.PreviewImport(ctx, []byte(raw))
}

// WizardGenerateStep renders the Generate step: recipe chips, shape
// steppers, the "work it out from the sky" switch, and the live result —
// all driven client-side once chronicle_gen.js loads (wizardGenerateBody's
// own x-init handles that on demand; nothing here waits for it).
// GET /campaigns/:id/calendars/wizard/generate
func (h *Handler) WizardGenerateStep(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	body := wizardGenerateBody(cc.Campaign.ID, middleware.GetCSRFToken(c), "")
	return middleware.Render(c, http.StatusOK, wizardShell("Generate one", body))
}

// WizardGeneratePreview re-validates the calendar the browser's generator
// engine just produced (parseGeneratedCalendarJSON) and renders the SAME
// Review step every other source uses, with source="generate" so
// WizardCreate re-validates it exactly like "import"/"build" already do
// (see that switch's own comment). A failure re-renders the Generate step
// fresh (a new seed, not the owner's in-progress recipe sheet) rather than
// trying to restore client state the server never held — matching
// WizardBuildPreview's own precedent, and just as realistically unreached:
// parseGeneratedCalendarJSON only rejects an oversized payload or a
// structure over the same limits every other import path enforces, neither
// of which the engine's own generators ever produce on their own.
// POST /campaigns/:id/calendars/wizard/generate/preview
func (h *Handler) WizardGeneratePreview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ir, err := parseGeneratedCalendarJSON(c.Request().Context(), h.svc, c.FormValue("generated_json"))
	if err != nil {
		body := wizardGenerateBody(cc.Campaign.ID, middleware.GetCSRFToken(c), apperror.UserMessage(err, "couldn't use that generated calendar"))
		return middleware.Render(c, http.StatusOK, wizardShell("Generate one", body))
	}
	importJSON, err := json.Marshal(ir)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshal import result: %w", err))
	}
	reviewBody := wizardReviewBody(cc.Campaign.ID, "generate", "", string(importJSON), middleware.GetCSRFToken(c),
		fmt.Sprintf("/campaigns/%s/calendars/wizard/generate", cc.Campaign.ID), "", ir)
	return middleware.Render(c, http.StatusOK, wizardShell("Review", reviewBody))
}
