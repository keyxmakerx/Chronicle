package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// lookPickRequest is the body of PUT /campaigns/:id/my-look/:part. A null or
// empty value clears the member's pick, so they follow the campaign again.
type lookPickRequest struct {
	Value *string `json:"value"`
}

// SheetChecker reports whether a campaign's game system draws a character
// sheet. Avoids importing the systems package.
type SheetChecker interface {
	CampaignHasSheet(ctx context.Context, campaignID string) bool
}

// SetSheetChecker wires the sheet check "Your look" uses to leave out the
// character sheet styles when there is no sheet for them to dress.
func (h *Handler) SetSheetChecker(c SheetChecker) {
	h.sheetChecker = c
}

// ShownLookParts drops the character sheet part when hasSheet says the
// campaign has no sheet: its styles would save and change nothing the member
// can find. hasSheet is asked only when the sheet part is there, and a nil
// check keeps every part.
func ShownLookParts(parts []LookPart, hasSheet func() bool) []LookPart {
	if hasSheet == nil {
		return parts
	}
	i := slices.IndexFunc(parts, func(p LookPart) bool { return p.Key == LookPartSheet })
	if i < 0 || hasSheet() {
		return parts
	}
	return slices.Delete(slices.Clone(parts), i, i+1)
}

// YourLook is the page where a member sets the parts of the look the owner let
// them change for themselves: GET /campaigns/:id/my-look. The owner edits the
// campaign's look in Customize, and a member with nothing allowed is told so.
func (h *Handler) YourLook(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.MemberRole >= RoleOwner {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/customize/look", cc.Campaign.ID))
	}
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	ap := cc.Campaign.ParseSettings().Appearance
	picks, err := h.service.MemberLookPicks(ctx, cc, userID)
	if err != nil {
		return err
	}
	var hasSheet func() bool
	if h.sheetChecker != nil {
		hasSheet = func() bool { return h.sheetChecker.CampaignHasSheet(ctx, cc.Campaign.ID) }
	}
	var parts []yourLookPart
	for _, p := range ShownLookParts(PickableParts(ap, cc), hasSheet) {
		parts = append(parts, yourLookPart{
			Part:     p,
			URL:      fmt.Sprintf("/campaigns/%s/my-look/%s", cc.Campaign.ID, p.Key),
			Current:  picks[p.Key],
			Campaign: p.campaignChoice(ap),
		})
	}
	return middleware.Render(c, http.StatusOK, YourLookPage(cc, parts))
}

// PutLookPick stores or clears the caller's own pick for one part:
// PUT /campaigns/:id/my-look/:part. The service checks the current "who can
// change" setting and the value; the row is the caller's own.
func (h *Handler) PutLookPick(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req lookPickRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	value := ""
	if req.Value != nil {
		value = *req.Value
	}
	if err := h.service.SetMemberLookPick(c.Request().Context(), cc, auth.GetUserID(c), c.Param("part"), value); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// yourLookOnClick saves the pressed choice (or the reset, whose value is
// empty) through the PUT endpoint and reloads to show it. Inline, with no
// <script> sibling, because the page can be swapped in by a boosted link;
// the address and value come from the button's own escaped attributes.
func yourLookOnClick() templ.ComponentScript {
	body := `(function(b){` +
		`var g=b.closest('[data-look-part]'),all=g.querySelectorAll('button');` +
		`all.forEach(function(x){x.disabled=true;});` +
		`Chronicle.apiFetch(b.getAttribute('data-url'),{method:'PUT',body:{value:b.getAttribute('data-value')||null}}).then(function(r){` +
		`if(!r.ok)throw new Error('refused');window.location.reload();` +
		`}).catch(function(){all.forEach(function(x){x.disabled=false;});Chronicle.notify('That could not be saved. Try again.','error');});` +
		`})(this)`
	return templ.ComponentScript{Name: "campaigns_yourLook", Call: body}
}
