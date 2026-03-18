package settings

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	// Empty values default to 0 (unlimited). Non-empty invalid values are rejected
	// to prevent accidentally removing limits via malformed input.
	maxUploadMB, err := parseFloatField(c.FormValue("max_upload_size"))
	if err != nil {
		return apperror.NewBadRequest("invalid max upload size")
	}
	maxStorageUserMB, err := parseFloatField(c.FormValue("max_storage_per_user"))
	if err != nil {
		return apperror.NewBadRequest("invalid max storage per user")
	}
	maxStorageCampaignMB, err := parseFloatField(c.FormValue("max_storage_per_campaign"))
	if err != nil {
		return apperror.NewBadRequest("invalid max storage per campaign")
	}
	maxFiles, err := parseIntField(c.FormValue("max_files_per_campaign"))
	if err != nil {
		return apperror.NewBadRequest("invalid max files per campaign")
	}
	rateLimit, err := parseIntField(c.FormValue("rate_limit_uploads_per_min"))
	if err != nil {
		return apperror.NewBadRequest("invalid rate limit")
	}

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
	return middleware.HTMXRedirect(c, "/admin/storage")
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

	return middleware.HTMXRedirect(c, "/admin/storage")
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

	return middleware.HTMXRedirect(c, "/admin/storage")
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

	return middleware.HTMXRedirect(c, "/admin/storage")
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

	return middleware.HTMXRedirect(c, "/admin/storage")
}

// --- Temporary Bypass Handlers ---

// SetUserBypass sets a temporary bypass on a user's upload limit
// (PUT /admin/users/:id/storage/bypass).
func (h *Handler) SetUserBypass(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}

	// Parse bypass upload limit (MB).
	var maxUpload *int64
	if v := c.FormValue("bypass_max_upload"); v != "" {
		mb, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return apperror.NewBadRequest("invalid bypass max upload size")
		}
		bytes := int64(mb * 1024 * 1024)
		maxUpload = &bytes
	}

	// Parse duration.
	expiresAt, err := parseDuration(c.FormValue("duration"))
	if err != nil {
		return apperror.NewBadRequest("invalid duration")
	}

	reason := c.FormValue("reason")
	grantedBy := auth.GetUserID(c)

	if err := h.service.SetUserBypass(c.Request().Context(), userID, maxUpload, expiresAt, reason, grantedBy); err != nil {
		return err
	}

	slog.Info("user storage bypass set",
		slog.String("target_user", userID),
		slog.String("by", grantedBy),
		slog.Time("expires_at", expiresAt),
	)

	return middleware.HTMXRedirect(c, "/admin/storage")
}

// ClearUserBypass removes a temporary bypass from a user's storage limits
// (DELETE /admin/users/:id/storage/bypass).
func (h *Handler) ClearUserBypass(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}

	if err := h.service.ClearUserBypass(c.Request().Context(), userID); err != nil {
		return err
	}

	slog.Info("user storage bypass cleared",
		slog.String("target_user", userID),
		slog.String("by", auth.GetUserID(c)),
	)

	return middleware.HTMXRedirect(c, "/admin/storage")
}

// SetCampaignBypass sets a temporary bypass on a campaign's storage limits
// (PUT /admin/campaigns/:id/storage/bypass).
func (h *Handler) SetCampaignBypass(c echo.Context) error {
	campaignID := c.Param("id")
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}

	// Parse bypass storage limit (MB).
	var maxStorage *int64
	if v := c.FormValue("bypass_max_storage"); v != "" {
		mb, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return apperror.NewBadRequest("invalid bypass max storage")
		}
		bytes := int64(mb * 1024 * 1024)
		maxStorage = &bytes
	}

	// Parse bypass files limit.
	var maxFiles *int
	if v := c.FormValue("bypass_max_files"); v != "" {
		files, err := strconv.Atoi(v)
		if err != nil {
			return apperror.NewBadRequest("invalid bypass max files")
		}
		maxFiles = &files
	}

	// Parse duration.
	expiresAt, err := parseDuration(c.FormValue("duration"))
	if err != nil {
		return apperror.NewBadRequest("invalid duration")
	}

	reason := c.FormValue("reason")
	grantedBy := auth.GetUserID(c)

	if err := h.service.SetCampaignBypass(c.Request().Context(), campaignID, maxStorage, maxFiles, expiresAt, reason, grantedBy); err != nil {
		return err
	}

	slog.Info("campaign storage bypass set",
		slog.String("target_campaign", campaignID),
		slog.String("by", grantedBy),
		slog.Time("expires_at", expiresAt),
	)

	return middleware.HTMXRedirect(c, "/admin/storage")
}

// ClearCampaignBypass removes a temporary bypass from a campaign's storage limits
// (DELETE /admin/campaigns/:id/storage/bypass).
func (h *Handler) ClearCampaignBypass(c echo.Context) error {
	campaignID := c.Param("id")
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}

	if err := h.service.ClearCampaignBypass(c.Request().Context(), campaignID); err != nil {
		return err
	}

	slog.Info("campaign storage bypass cleared",
		slog.String("target_campaign", campaignID),
		slog.String("by", auth.GetUserID(c)),
	)

	return middleware.HTMXRedirect(c, "/admin/storage")
}

// GetCORSOrigins returns the current CORS origin whitelist as JSON
// (GET /admin/api/cors).
func (h *Handler) GetCORSOrigins(c echo.Context) error {
	origins, err := h.service.GetCORSOrigins(c.Request().Context())
	if err != nil {
		return err
	}
	if origins == nil {
		origins = []string{}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"origins": origins,
	})
}

// UpdateCORSOrigins replaces the CORS origin whitelist
// (POST /admin/api/cors).
func (h *Handler) UpdateCORSOrigins(c echo.Context) error {
	ctx := c.Request().Context()

	// Accept both JSON and form data.
	var origins []string
	ct := c.Request().Header.Get("Content-Type")
	if ct == "application/json" || ct == "application/json; charset=utf-8" {
		var body struct {
			Origins []string `json:"origins"`
		}
		if err := c.Bind(&body); err != nil {
			return apperror.NewBadRequest("invalid request body")
		}
		origins = body.Origins
	} else {
		// Form submission: origins as comma-separated textarea value.
		raw := c.FormValue("cors_origins")
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				origins = append(origins, line)
			}
		}
	}

	if err := h.service.UpdateCORSOrigins(ctx, origins); err != nil {
		return err
	}

	slog.Info("CORS origins updated",
		slog.String("by", auth.GetUserID(c)),
		slog.Int("count", len(origins)),
	)

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/api")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// parseDuration converts a duration string (e.g., "1h", "6h", "24h", "7d", "30d")
// into a future expiration time.
func parseDuration(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("duration is required")
	}

	durations := map[string]time.Duration{
		"1h":  1 * time.Hour,
		"6h":  6 * time.Hour,
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}

	d, ok := durations[s]
	if !ok {
		return time.Time{}, fmt.Errorf("unsupported duration %q", s)
	}
	return time.Now().Add(d), nil
}

// parseFloatField parses a form field as float64. Empty strings return 0 (no limit).
// Non-empty strings that fail to parse return an error, preventing accidental
// removal of limits via malformed input.
func parseFloatField(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("value must not be negative")
	}
	return v, nil
}

// parseIntField parses a form field as int. Empty strings return 0 (no limit).
// Non-empty strings that fail to parse return an error.
func parseIntField(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("value must not be negative")
	}
	return v, nil
}
