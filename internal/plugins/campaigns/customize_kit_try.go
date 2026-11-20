// customize_kit_try.go serves the header brush's "Try a style kit here": the
// kits and their tokens for a try-out drawn only in the owner's browser, and
// "Keep this kit", which puts the tried kit into Customize's draft for the
// Look editor to show. Nothing here changes what anyone else sees.

package campaigns

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// kitTryKit is one kit as the try-out strip names it.
type kitTryKit struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// CustomizeKitTry answers GET /campaigns/:id/customize/kit-try with the
// style kits, the tokens that draw each of them on this campaign
// (layouts.KitTryCSS) and Material's card entrance, which the page wears
// only while no other kit is on. 404 when the page look is not Material,
// where no kit is drawn. Owner-only.
func (h *Handler) CustomizeKitTry(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// The campaign's look reaches a context only on the way to a page
	// render; the tokens must be the ones that page is drawn with.
	ctx := c.Request().Context()
	if middleware.LayoutInjector != nil {
		ctx = middleware.LayoutInjector(c, ctx)
	}
	css := layouts.KitTryCSS(ctx)
	if css == "" {
		return apperror.NewNotFound("this campaign's pages wear no style kit")
	}
	kits := make([]kitTryKit, 0, len(AppearanceStyleKits))
	for _, id := range AppearanceStyleKits {
		kits = append(kits, kitTryKit{ID: id, Label: LookChoiceLabel(id)})
	}
	entrance := ""
	if a := layouts.GetAppearance(ctx); a != nil {
		entrance = a.Entrance
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"kits": kits, "css": css, "entrance": entrance})
}

// CustomizeKitKeep answers POST /campaigns/:id/customize/kit-try/keep
// {"kit": "ornate"}: the kit goes into the look section of the owner's
// Customize draft, and the answer is where the Look editor shows it. The
// draft is the same one Customize's frame keeps, so the kit waits there
// beside the owner's other unsaved changes until Save or Discard.
func (h *Handler) CustomizeKitKeep(c *echo.Context) error {
	cc, userID, err := h.draftContext(c)
	if err != nil {
		return err
	}
	var req struct {
		Kit string `json:"kit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 1024)).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if !slices.Contains(AppearanceStyleKits, req.Kit) {
		return apperror.NewBadRequest("unknown style kit")
	}
	ctx := c.Request().Context()
	version, held := h.CustomizeDraftFor(ctx, cc.Campaign.ID, userID)
	merged, err := keepKitInDraft(held, req.Kit, func() any { return buildCustomizeState(ctx, cc).Draft })
	if err != nil {
		return err
	}
	if _, err := h.customizeDrafts.Save(ctx, cc.Campaign.ID, userID, version, merged, h.CustomizeSectionKeys()); err != nil {
		return h.draftConflictOr(c, cc, userID, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"href": h.customizeBase(cc) + "/look#page"})
}

// keepKitInDraft returns the Customize draft with kit as its pages' kit
// under the Material page look. A look section already in the draft keeps
// everything else in it, both the editor's own state and what Save applies;
// without one, the section starts from the saved look (saved), and the Look
// editor fills in what Save applies when it opens on it.
func keepKitInDraft(draft json.RawMessage, kit string, saved func() any) (json.RawMessage, error) {
	sections, err := parseCustomizeDraft(draft)
	if err != nil {
		return nil, err
	}
	if sections == nil {
		sections = map[string]json.RawMessage{}
	}
	sec := map[string]json.RawMessage{}
	if raw, ok := sections[customizeSectionLook]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &sec); err != nil {
			return nil, apperror.NewBadRequest("the look in the draft is not valid")
		}
	}
	if _, ok := sec["draft"]; !ok {
		b, err := json.Marshal(saved())
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		sec["draft"] = b
	}
	for _, part := range []string{"draft", "input"} {
		raw, ok := sec[part]
		if !ok || string(raw) == "null" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil || m == nil {
			return nil, apperror.NewBadRequest("the look in the draft is not valid")
		}
		page, _ := m["page"].(map[string]any)
		if page == nil {
			page = map[string]any{}
		}
		page["look"] = "annals"
		page["kit"] = kit
		m["page"] = page
		b, err := json.Marshal(m)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		sec[part] = b
	}
	b, err := json.Marshal(sec)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	sections[customizeSectionLook] = b
	return json.Marshal(sections)
}
