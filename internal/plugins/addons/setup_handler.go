package addons

// HTTP handlers for the per-extension settings / onboarding pages. These render
// a generic page driven entirely by a slug's registered SetupProvider (see
// setup_provider.go) — the framework gives every addon a consistent settings
// surface (overlay or full page) without per-addon templates.

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ExtensionSettingsView is the view-model the settings templates render. It is
// provider-agnostic: the handler fills it from whatever SetupProvider (if any)
// is registered for the addon slug.
type ExtensionSettingsView struct {
	CampaignID  string
	Slug        string
	AddonName   string
	AddonIcon   string
	HasProvider bool // false → "nothing to set up" panel
	Checks      []SetupCheck
	Questions   []SetupQuestion
	State       SetupState
	CSRFToken   string
	SuccessMsgs []string
	ErrMsg      string
}

// ExtensionSettings renders an extension's settings page
// (GET /campaigns/:id/extensions/:slug/settings). HTMX requests get the modal
// overlay (so the Extensions hub can inject it); normal requests get the full
// page (deep-link / no-JS fallback). Owner-gated by the route.
func (h *Handler) ExtensionSettings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewForbidden("campaign context required")
	}
	ctx := c.Request().Context()
	slug := c.Param("slug")

	addon, err := h.service.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if addon == nil {
		return apperror.NewNotFound("extension not found")
	}

	v := ExtensionSettingsView{
		CampaignID: cc.Campaign.ID,
		Slug:       slug,
		AddonName:  addon.Name,
		AddonIcon:  addon.Icon,
		CSRFToken:  middleware.GetCSRFToken(c),
	}
	h.fillProviderView(ctx, &v, cc.Campaign.ID)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ExtensionSettingsOverlay(v))
	}
	return middleware.Render(c, http.StatusOK, ExtensionSettingsPage(v))
}

// ApplyExtensionSettings applies the owner's answers
// (POST /campaigns/:id/extensions/:slug/settings/apply). On success it persists
// the answers + completion, re-renders the body with the result, and fires the
// hub-refresh + a toast. Owner-gated by the route.
func (h *Handler) ApplyExtensionSettings(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewForbidden("campaign context required")
	}
	ctx := c.Request().Context()
	slug := c.Param("slug")

	addon, err := h.service.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if addon == nil {
		return apperror.NewNotFound("extension not found")
	}
	provider, ok := h.service.SetupProviderFor(slug)
	if !ok {
		return apperror.NewBadRequest("this extension has no configurable setup")
	}

	answers, err := formAnswers(c)
	if err != nil {
		return err
	}

	v := ExtensionSettingsView{
		CampaignID:  cc.Campaign.ID,
		Slug:        slug,
		AddonName:   addon.Name,
		AddonIcon:   addon.Icon,
		HasProvider: true,
		CSRFToken:   middleware.GetCSRFToken(c),
	}

	result, applyErr := provider.Apply(ctx, cc.Campaign.ID, answers)
	if applyErr != nil {
		// Re-render with the error and a fresh snapshot so the owner can retry.
		h.fillProviderView(ctx, &v, cc.Campaign.ID)
		v.ErrMsg = apperror.SafeMessage(applyErr)
		return h.renderSettingsBody(c, v)
	}

	// Persist answers + completion, preserving any other config_json keys.
	state, _ := h.service.GetSetupState(ctx, cc.Campaign.ID, slug)
	state.Completed = result.Completed
	state.Dismissed = false
	state.Answers = answers
	if err := h.service.SaveSetupState(ctx, cc.Campaign.ID, slug, state); err != nil {
		return err
	}

	// Fresh snapshot AFTER applying, so resolved checks disappear.
	h.fillProviderView(ctx, &v, cc.Campaign.ID)
	v.SuccessMsgs = result.Messages
	if len(v.SuccessMsgs) == 0 {
		v.SuccessMsgs = []string{"Settings applied."}
	}

	if middleware.IsHTMX(c) {
		// Clear the card badge (hub re-fetches its fragment) and toast success.
		setSetupHXTrigger(c, "extensions-hub-refresh", "Setup applied for "+addon.Name, "success")
	}
	return h.renderSettingsBody(c, v)
}

// DismissExtensionSetup marks setup as dismissed so the Features card stops
// nudging (POST /campaigns/:id/extensions/:slug/settings/dismiss). The owner can
// reopen the page any time. Owner-gated by the route.
func (h *Handler) DismissExtensionSetup(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewForbidden("campaign context required")
	}
	ctx := c.Request().Context()
	slug := c.Param("slug")

	state, _ := h.service.GetSetupState(ctx, cc.Campaign.ID, slug)
	state.Dismissed = true
	if err := h.service.SaveSetupState(ctx, cc.Campaign.ID, slug, state); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Trigger", "extensions-hub-refresh")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/extensions")
}

// fillProviderView populates the checks/questions/state on v from the registered
// provider for v.Slug, if any. Best-effort: a provider read error becomes an
// inline ErrMsg rather than failing the whole page.
func (h *Handler) fillProviderView(ctx context.Context, v *ExtensionSettingsView, campaignID string) {
	provider, ok := h.service.SetupProviderFor(v.Slug)
	if !ok {
		v.HasProvider = false
		return
	}
	v.HasProvider = true
	if checks, err := provider.RunChecks(ctx, campaignID); err != nil {
		v.ErrMsg = apperror.SafeMessage(err)
	} else {
		v.Checks = checks
	}
	if qs, err := provider.Questions(ctx, campaignID); err == nil {
		v.Questions = qs
	}
	if state, err := h.service.GetSetupState(ctx, campaignID, v.Slug); err == nil {
		v.State = state
	}
}

// renderSettingsBody renders the inner fragment for HTMX (so an Apply swaps
// inside the open modal) or the full page otherwise.
func (h *Handler) renderSettingsBody(c echo.Context, v ExtensionSettingsView) error {
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, ExtensionSettingsFormFragment(v))
	}
	return middleware.Render(c, http.StatusOK, ExtensionSettingsPage(v))
}

// formAnswers parses the POST body into an answer map, dropping framework fields.
func formAnswers(c echo.Context) (map[string]string, error) {
	if err := c.Request().ParseForm(); err != nil {
		return nil, apperror.NewBadRequest("invalid form submission")
	}
	answers := make(map[string]string)
	for key, vals := range c.Request().Form {
		if key == "csrf_token" || key == "action" {
			continue
		}
		if len(vals) > 0 {
			answers[key] = vals[0]
		}
	}
	return answers, nil
}

// setSetupHXTrigger sets an HX-Trigger header that both refreshes the Extensions
// hub fragment and raises a client toast, in one JSON payload (HTMX fires every
// key as an event; a null value means "no detail").
func setSetupHXTrigger(c echo.Context, refreshEvent, notifyMsg, notifyType string) {
	payload := map[string]any{
		refreshEvent: nil,
		"chronicle:notify": map[string]string{
			"message": notifyMsg,
			"type":    notifyType,
		},
	}
	if b, err := json.Marshal(payload); err == nil {
		c.Response().Header().Set("HX-Trigger", string(b))
	}
}
