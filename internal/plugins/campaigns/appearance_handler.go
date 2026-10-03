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
	"menu":     3 << 19, // 1.5 MB: the menu's banner corner
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
	if settings.Appearance != nil {
		stored[settings.Appearance.SidebarBanner] = true
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
