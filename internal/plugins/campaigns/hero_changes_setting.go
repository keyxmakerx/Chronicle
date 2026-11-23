package campaigns

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// hero_changes_setting.go is the People page's "Changing heroes" section:
// when players may change their own hero's picks. It stores the same rule the
// Characters page's Directors' section does; reopening one hero stays there,
// beside the heroes, and this section links to it.

// HeroChangesSetting is what the section draws.
type HeroChangesSetting struct {
	CampaignID string
	CSRFToken  string
	Summary    string
	// Ruled is whether a rule is stored, which is when the Characters page
	// lists each hero to reopen.
	Ruled bool
	Form  HeroChangeFormValues
	// Notice confirms a save; Err says why one was refused. Both empty on a
	// plain page load.
	Notice string
	Err    string
}

// SetHeroChanges hands in whether players' hero picks can lock in a
// campaign. The lock lives in the entities plugin: it is left out while its
// table is unhealthy, and it applies only to heroes players claim, which the
// claiming add-on allows. The composition root judges both, since campaigns
// names neither; the section and its Find entry show only where it is true,
// so they never offer a rule nothing would enforce.
func (h *Handler) SetHeroChanges(on func(ctx context.Context, campaignID string) bool) {
	h.heroChangesOn = on
}

// heroChangesApply reports whether the section belongs on this campaign.
func (h *Handler) heroChangesApply(ctx context.Context, campaignID string) bool {
	return h.heroChangesOn != nil && h.heroChangesOn(ctx, campaignID)
}

// heroChangesSetting builds the section for the owner, or nil.
func (h *Handler) heroChangesSetting(c *echo.Context, cc *CampaignContext) *HeroChangesSetting {
	if cc.MemberRole < RoleOwner || !h.heroChangesApply(c.Request().Context(), cc.Campaign.ID) {
		return nil
	}
	rule := cc.Campaign.ParseSettings().HeroChanges
	return &HeroChangesSetting{
		CampaignID: cc.Campaign.ID, CSRFToken: middleware.GetCSRFToken(c),
		Summary: rule.Summary(), Ruled: rule != nil, Form: rule.FormValues(),
	}
}

// SaveHeroChangesSetting stores when players may change their hero's picks
// and redraws the section. Owner only, like the rest of People.
//
// Route: POST /campaigns/:id/members/hero-changes
func (h *Handler) SaveHeroChangesSetting(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner || !h.heroChangesApply(c.Request().Context(), cc.Campaign.ID) {
		return apperror.NewNotFound("page not found")
	}
	var form HeroChangeForm
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 8*1024)
	if err := c.Bind(&form); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	v := h.heroChangesSetting(c, cc)
	rule, err := form.Rule()
	if err == nil {
		err = h.service.UpdateHeroChanges(c.Request().Context(), cc.Campaign.ID, rule)
	}
	if err != nil {
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
			return err
		}
		// A refused rule keeps what was typed, so it can be put right.
		v.Err = sentence(apperror.SafeMessage(err))
		v.Form = form.Values(v.Form)
		return h.renderHeroChangesSetting(c, cc, v)
	}
	h.logAudit(c, cc.Campaign.ID, "campaign.hero_changes.updated", map[string]any{"ruled": rule != nil})
	v.Summary, v.Ruled, v.Form, v.Notice = rule.Summary(), rule != nil, rule.FormValues(), "Saved."
	return h.renderHeroChangesSetting(c, cc, v)
}

// renderHeroChangesSetting answers a save: the section alone for HTMX.
// Otherwise a save goes back to the People page at the section, and a
// refusal draws the page itself, since a redirect would lose the sentence.
func (h *Handler) renderHeroChangesSetting(c *echo.Context, cc *CampaignContext, v *HeroChangesSetting) error {
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, heroChangesSection(v))
	}
	if v.Err != "" {
		list := h.memberListView(c, cc, "")
		list.HeroChanges = v
		return h.renderPeoplePage(c, cc, list)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/members#hero-changes")
}

// sentence starts a message with a capital and ends it with a full stop.
func sentence(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		s = string(s[0]-'a'+'A') + s[1:]
	}
	if s[len(s)-1] != '.' {
		s += "."
	}
	return s
}
