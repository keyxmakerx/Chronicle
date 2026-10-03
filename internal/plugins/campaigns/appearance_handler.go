package campaigns

import (
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// appearancePictureLimits caps each Customize picture, matching what the
// page tells the owner. The media service still applies its own limits.
var appearancePictureLimits = map[string]int64{
	"logo":     1 << 20,
	"backdrop": 4 << 20,
	"header":   3 << 19, // 1.5 MB
}

// SaveAppearanceAPI handles PUT /campaigns/:id/appearance: one Save from
// the Customize page, carrying the whole draft.
func (h *Handler) SaveAppearanceAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change how the campaign looks")
	}

	var in AppearanceInput
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	// A picture is either the one already saved or one this campaign
	// uploaded; anything else could point the page at another campaign's file.
	stored := map[string]bool{}
	settings := cc.Campaign.ParseSettings()
	stored[settings.BrandLogo] = true
	if settings.TopbarStyle != nil {
		stored[settings.TopbarStyle.ImagePath] = true
	}
	if cc.Campaign.BackdropPath != nil {
		stored[*cc.Campaign.BackdropPath] = true
	}
	for _, p := range in.AppearancePictures() {
		if stored[p] {
			continue
		}
		if h.mediaUploader == nil {
			return apperror.NewBadRequest("pictures can't be checked right now")
		}
		ok, err := h.mediaUploader.OwnsFile(c.Request().Context(), cc.Campaign.ID, p)
		if err != nil {
			return err
		}
		if !ok {
			return apperror.NewBadRequest("that picture doesn't belong to this campaign")
		}
	}

	if err := h.service.SaveAppearance(c.Request().Context(), cc.Campaign.ID, in); err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, "campaign.appearance.saved", map[string]any{"look": in.Look})
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UploadAppearancePictureAPI handles POST /campaigns/:id/appearance/picture.
// It stores the file and returns its name; nothing changes on the campaign
// until the owner saves the draft that uses it.
func (h *Handler) UploadAppearancePictureAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change how the campaign looks")
	}
	if h.mediaUploader == nil {
		return apperror.NewInternal(nil)
	}

	limit, ok := appearancePictureLimits[c.FormValue("kind")]
	if !ok {
		return apperror.NewBadRequest("unknown picture kind")
	}
	file, err := c.FormFile("file")
	if err != nil {
		return apperror.NewBadRequest("no file provided")
	}
	if file.Size > limit {
		return apperror.NewBadRequest("that picture is too large")
	}
	src, err := file.Open()
	if err != nil {
		return apperror.NewInternal(err)
	}
	defer func() { _ = src.Close() }()
	fileBytes, err := io.ReadAll(io.LimitReader(src, limit+1))
	if err != nil {
		return apperror.NewBadRequest("failed to read file")
	}
	if int64(len(fileBytes)) > limit {
		return apperror.NewBadRequest("that picture is too large")
	}

	mimeType := http.DetectContentType(fileBytes)
	if !strings.HasPrefix(mimeType, "image/") {
		return apperror.NewBadRequest("that file is not a picture")
	}
	filename, err := h.mediaUploader.UploadBackdrop(
		c.Request().Context(), cc.Campaign.ID,
		auth.GetUserID(c), fileBytes, file.Filename, mimeType,
	)
	if err != nil {
		return err
	}
	h.logAudit(c, cc.Campaign.ID, "campaign.appearance.picture_uploaded", map[string]any{"kind": c.FormValue("kind")})
	return c.JSON(http.StatusOK, map[string]string{
		"name": filename,
		"url":  layouts.MediaURL(c.Request().Context(), filename),
	})
}

// DeleteAppearancePictureAPI handles DELETE /campaigns/:id/appearance/picture?name=...
// It tidies a picture the Customize page uploaded but never saved. The page
// calls it fire-and-forget, so every "not yours to delete" case is a quiet
// no-op: a file the saved look still uses, another campaign's file, a file
// that did not come from this upload path, or one that is already gone.
// Only a malformed name is refused, since that is never a legitimate call.
func (h *Handler) DeleteAppearancePictureAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can change how the campaign looks")
	}
	name, err := pictureName("appearance", c.QueryParam("name"))
	if err != nil {
		return err
	}
	if name == "" {
		return apperror.NewBadRequest("invalid appearance picture")
	}
	done := func(deleted bool) error {
		return c.JSON(http.StatusOK, map[string]any{"status": "ok", "deleted": deleted})
	}

	// The request's campaign was loaded from the database for this request,
	// so it is the saved state; a draft is never consulted.
	if appearancePictureSaved(cc.Campaign, name) || h.mediaUploader == nil {
		return done(false)
	}
	deleted, err := h.mediaUploader.DeletePicture(c.Request().Context(), cc.Campaign.ID, name)
	if err != nil {
		return err
	}
	if deleted {
		h.logAudit(c, cc.Campaign.ID, "campaign.appearance.picture_discarded", nil)
	}
	return done(deleted)
}

// appearancePictureSaved reports whether the campaign's saved settings or
// backdrop column use the picture. This is the one list of places a picture
// can live; a new picture field must be added here and to AppearancePictures.
func appearancePictureSaved(campaign *Campaign, name string) bool {
	settings := campaign.ParseSettings()
	if settings.BrandLogo == name {
		return true
	}
	if settings.TopbarStyle != nil && settings.TopbarStyle.ImagePath == name {
		return true
	}
	return campaign.BackdropPath != nil && *campaign.BackdropPath == name
}
