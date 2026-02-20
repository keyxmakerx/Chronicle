package settings

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Ensure old template references are not needed since settings are now
// rendered on the combined /admin/storage page.

// Handler handles HTTP requests for site storage settings management.
// All routes require site admin middleware.
type Handler struct {
	service SettingsService
}

// NewHandler creates a new settings handler.
func NewHandler(service SettingsService) *Handler {
	return &Handler{service: service}
}

// StorageSettings redirects to the combined storage page (GET /admin/storage/settings).
// Settings are now shown on the unified /admin/storage page under the Limits tab.
func (h *Handler) StorageSettings(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}

// UpdateStorageSettings saves global storage limits (POST /admin/storage/settings).
func (h *Handler) UpdateStorageSettings(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse form values. Sizes are submitted in MB and converted to bytes.
	maxUploadMB, _ := strconv.ParseFloat(c.FormValue("max_upload_size"), 64)
	maxStorageUserMB, _ := strconv.ParseFloat(c.FormValue("max_storage_per_user"), 64)
	maxStorageCampaignMB, _ := strconv.ParseFloat(c.FormValue("max_storage_per_campaign"), 64)
	maxFiles, _ := strconv.Atoi(c.FormValue("max_files_per_campaign"))
	rateLimit, _ := strconv.Atoi(c.FormValue("rate_limit_uploads_per_min"))

	limits := &GlobalStorageLimits{
		MaxUploadSize:          int64(maxUploadMB * 1024 * 1024),
		MaxStoragePerUser:      int64(maxStorageUserMB * 1024 * 1024),
		MaxStoragePerCampaign:  int64(maxStorageCampaignMB * 1024 * 1024),
		MaxFilesPerCampaign:    maxFiles,
		RateLimitUploadsPerMin: rateLimit,
	}

	if err := h.service.UpdateStorageLimits(ctx, limits); err != nil {
		slog.Error("failed to update storage limits", slog.Any("error", err))
		return err
	}

	slog.Info("storage limits updated",
		slog.String("by", auth.GetUserID(c)),
		slog.Int64("max_upload", limits.MaxUploadSize),
	)

	// Redirect back to the combined storage page.
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/storage")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}

// SetUserStorageLimit creates or updates a per-user storage override
// (PUT /admin/users/:id/storage).
func (h *Handler) SetUserStorageLimit(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}

	// Parse override values. Empty string means NULL (inherit global).
	limit := &UserStorageLimit{UserID: userID}

	if v := c.FormValue("max_upload_size"); v != "" {
		mb, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return apperror.NewBadRequest("invalid max upload size")
		}
		bytes := int64(mb * 1024 * 1024)
		limit.MaxUploadSize = &bytes
	}
	if v := c.FormValue("max_total_storage"); v != "" {
		mb, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return apperror.NewBadRequest("invalid max total storage")
		}
		bytes := int64(mb * 1024 * 1024)
		limit.MaxTotalStorage = &bytes
	}

	if err := h.service.SetUserLimit(c.Request().Context(), limit); err != nil {
		return err
	}

	slog.Info("user storage limit set",
		slog.String("target_user", userID),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/storage")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}

// DeleteUserStorageLimit removes a per-user storage override
// (DELETE /admin/users/:id/storage).
func (h *Handler) DeleteUserStorageLimit(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}

	if err := h.service.DeleteUserLimit(c.Request().Context(), userID); err != nil {
		return err
	}

	slog.Info("user storage limit removed",
		slog.String("target_user", userID),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/storage")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}

// SetCampaignStorageLimit creates or updates a per-campaign storage override
// (PUT /admin/campaigns/:id/storage).
func (h *Handler) SetCampaignStorageLimit(c echo.Context) error {
	campaignID := c.Param("id")
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}

	limit := &CampaignStorageLimit{CampaignID: campaignID}

	if v := c.FormValue("max_total_storage"); v != "" {
		mb, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return apperror.NewBadRequest("invalid max total storage")
		}
		bytes := int64(mb * 1024 * 1024)
		limit.MaxTotalStorage = &bytes
	}
	if v := c.FormValue("max_files"); v != "" {
		files, err := strconv.Atoi(v)
		if err != nil {
			return apperror.NewBadRequest("invalid max files")
		}
		limit.MaxFiles = &files
	}

	if err := h.service.SetCampaignLimit(c.Request().Context(), limit); err != nil {
		return err
	}

	slog.Info("campaign storage limit set",
		slog.String("target_campaign", campaignID),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/storage")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}

// DeleteCampaignStorageLimit removes a per-campaign storage override
// (DELETE /admin/campaigns/:id/storage).
func (h *Handler) DeleteCampaignStorageLimit(c echo.Context) error {
	campaignID := c.Param("id")
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}

	if err := h.service.DeleteCampaignLimit(c.Request().Context(), campaignID); err != nil {
		return err
	}

	slog.Info("campaign storage limit removed",
		slog.String("target_campaign", campaignID),
		slog.String("by", auth.GetUserID(c)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/storage")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/storage")
}
