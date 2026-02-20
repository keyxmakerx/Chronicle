package syncapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles sync API HTTP requests for both the management UI
// (key management, dashboards) and the actual sync API endpoints.
type Handler struct {
	service SyncAPIService
}

// NewHandler creates a new sync API handler.
func NewHandler(service SyncAPIService) *Handler {
	return &Handler{service: service}
}

// --- Campaign Owner: API Key Management ---

// KeysPage renders the API keys management page for a campaign (GET /campaigns/:id/api-keys).
func (h *Handler) KeysPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "campaign context required")
	}

	keys, err := h.service.ListKeysByCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	since := time.Now().Add(-24 * time.Hour)
	stats, _ := h.service.GetCampaignStats(c.Request().Context(), cc.Campaign.ID, since)

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, OwnerKeysPageTempl(cc.Campaign.ID, keys, stats, csrfToken))
}

// CreateKey handles POST /campaigns/:id/api-keys to create a new API key.
func (h *Handler) CreateKey(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "campaign context required")
	}

	userID := auth.GetUserID(c)

	// Parse permissions from form checkboxes.
	var perms []APIKeyPermission
	for _, p := range c.Request().Form["permissions"] {
		perms = append(perms, APIKeyPermission(p))
	}
	if len(perms) == 0 {
		// Fallback: check individual values.
		if c.FormValue("perm_read") == "on" {
			perms = append(perms, PermRead)
		}
		if c.FormValue("perm_write") == "on" {
			perms = append(perms, PermWrite)
		}
		if c.FormValue("perm_sync") == "on" {
			perms = append(perms, PermSync)
		}
	}

	rateLimit := 60
	if rl := c.FormValue("rate_limit"); rl != "" {
		if v, err := strconv.Atoi(rl); err == nil {
			rateLimit = v
		}
	}

	var ipAllowlist []string
	if ips := strings.TrimSpace(c.FormValue("ip_allowlist")); ips != "" {
		for _, ip := range strings.Split(ips, "\n") {
			if trimmed := strings.TrimSpace(ip); trimmed != "" {
				ipAllowlist = append(ipAllowlist, trimmed)
			}
		}
	}

	var expiresAt *time.Time
	if exp := c.FormValue("expires_at"); exp != "" {
		if t, err := time.Parse("2006-01-02", exp); err == nil {
			expiresAt = &t
		}
	}

	input := CreateAPIKeyInput{
		Name:        c.FormValue("name"),
		CampaignID:  cc.Campaign.ID,
		Permissions: perms,
		IPAllowlist: ipAllowlist,
		RateLimit:   rateLimit,
		ExpiresAt:   expiresAt,
	}

	result, err := h.service.CreateKey(c.Request().Context(), userID, input)
	if err != nil {
		return err
	}

	// Render the key creation result showing the plaintext key once.
	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, KeyCreatedTempl(cc.Campaign.ID, result, csrfToken))
}

// ToggleKey handles PUT /campaigns/:id/api-keys/:keyID/toggle.
func (h *Handler) ToggleKey(c echo.Context) error {
	keyID, err := strconv.Atoi(c.Param("keyID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid key ID")
	}

	action := c.FormValue("action")
	ctx := c.Request().Context()

	if action == "activate" {
		if err := h.service.ActivateKey(ctx, keyID); err != nil {
			return err
		}
	} else {
		if err := h.service.DeactivateKey(ctx, keyID); err != nil {
			return err
		}
	}

	cc := campaigns.GetCampaignContext(c)
	if middleware.IsHTMX(c) && cc != nil {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/api-keys")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/api-keys")
}

// RevokeKey handles DELETE /campaigns/:id/api-keys/:keyID.
func (h *Handler) RevokeKey(c echo.Context) error {
	keyID, err := strconv.Atoi(c.Param("keyID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid key ID")
	}

	if err := h.service.RevokeKey(c.Request().Context(), keyID); err != nil {
		return err
	}

	cc := campaigns.GetCampaignContext(c)
	if middleware.IsHTMX(c) && cc != nil {
		c.Response().Header().Set("HX-Redirect", "/campaigns/"+cc.Campaign.ID+"/api-keys")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/campaigns/"+cc.Campaign.ID+"/api-keys")
}

// --- Admin: API Monitoring Dashboard ---

// AdminDashboard renders the admin API monitoring page (GET /admin/api).
func (h *Handler) AdminDashboard(c echo.Context) error {
	ctx := c.Request().Context()
	since := time.Now().Add(-24 * time.Hour)

	stats, _ := h.service.GetStats(ctx, since)
	requestSeries, _ := h.service.GetRequestTimeSeries(ctx, since, "hour")
	securitySeries, _ := h.service.GetSecurityTimeSeries(ctx, since)
	topIPs, _ := h.service.GetTopIPs(ctx, since, 10)
	topPaths, _ := h.service.GetTopPaths(ctx, since, 10)
	topKeys, _ := h.service.GetTopKeys(ctx, since, 10)

	secFilter := SecurityEventFilter{Limit: 20}
	secEvents, _, _ := h.service.ListSecurityEvents(ctx, secFilter)

	ipBlocks, _ := h.service.ListIPBlocks(ctx)

	keys, totalKeys, _ := h.service.ListAllKeys(ctx, 20, 0)

	csrfToken := middleware.GetCSRFToken(c)

	data := AdminDashboardData{
		Stats:           stats,
		RequestSeries:   requestSeries,
		SecuritySeries:  securitySeries,
		TopIPs:          topIPs,
		TopPaths:        topPaths,
		TopKeys:         topKeys,
		SecurityEvents:  secEvents,
		IPBlocks:        ipBlocks,
		APIKeys:         keys,
		TotalKeys:       totalKeys,
		CSRFToken:       csrfToken,
	}

	return middleware.Render(c, http.StatusOK, AdminAPIDashboardTempl(data))
}

// AdminRequestLogs returns paginated request logs (GET /admin/api/logs).
func (h *Handler) AdminRequestLogs(c echo.Context) error {
	ctx := c.Request().Context()

	filter := RequestLogFilter{Limit: 50}
	if v := c.QueryParam("ip"); v != "" {
		filter.IPAddress = &v
	}
	if v := c.QueryParam("key_id"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			filter.APIKeyID = &id
		}
	}
	if v := c.QueryParam("offset"); v != "" {
		if off, err := strconv.Atoi(v); err == nil {
			filter.Offset = off
		}
	}

	logs, total, err := h.service.ListRequestLogs(ctx, filter)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"logs":  logs,
		"total": total,
	})
}

// AdminSecurityEvents returns paginated security events (GET /admin/api/security).
func (h *Handler) AdminSecurityEvents(c echo.Context) error {
	ctx := c.Request().Context()

	filter := SecurityEventFilter{Limit: 50}
	if v := c.QueryParam("type"); v != "" {
		et := SecurityEventType(v)
		filter.EventType = &et
	}
	if v := c.QueryParam("resolved"); v != "" {
		resolved := v == "true"
		filter.Resolved = &resolved
	}
	if v := c.QueryParam("offset"); v != "" {
		if off, err := strconv.Atoi(v); err == nil {
			filter.Offset = off
		}
	}

	events, total, err := h.service.ListSecurityEvents(ctx, filter)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"events": events,
		"total":  total,
	})
}

// ResolveEvent handles PUT /admin/api/security/:eventID/resolve.
func (h *Handler) ResolveEvent(c echo.Context) error {
	eventID, err := strconv.ParseInt(c.Param("eventID"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event ID")
	}

	adminID := auth.GetUserID(c)
	if err := h.service.ResolveSecurityEvent(c.Request().Context(), eventID, adminID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/api")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/api")
}

// BlockIP handles POST /admin/api/ip-blocks to add an IP block.
func (h *Handler) BlockIP(c echo.Context) error {
	adminID := auth.GetUserID(c)

	var expiresAt *time.Time
	if exp := c.FormValue("expires_at"); exp != "" {
		if t, err := time.Parse("2006-01-02", exp); err == nil {
			expiresAt = &t
		}
	}

	_, err := h.service.BlockIP(c.Request().Context(),
		c.FormValue("ip_address"),
		c.FormValue("reason"),
		adminID,
		expiresAt,
	)
	if err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/api")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/api")
}

// UnblockIP handles DELETE /admin/api/ip-blocks/:blockID.
func (h *Handler) UnblockIP(c echo.Context) error {
	blockID, err := strconv.Atoi(c.Param("blockID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid block ID")
	}

	if err := h.service.UnblockIP(c.Request().Context(), blockID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/api")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/api")
}

// AdminToggleKey handles PUT /admin/api/keys/:keyID/toggle — admin can activate/deactivate any key.
func (h *Handler) AdminToggleKey(c echo.Context) error {
	keyID, err := strconv.Atoi(c.Param("keyID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid key ID")
	}

	action := c.FormValue("action")
	ctx := c.Request().Context()

	if action == "activate" {
		if err := h.service.ActivateKey(ctx, keyID); err != nil {
			return err
		}
	} else {
		if err := h.service.DeactivateKey(ctx, keyID); err != nil {
			return err
		}
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/api")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/api")
}

// AdminRevokeKey handles DELETE /admin/api/keys/:keyID — admin can revoke any key.
func (h *Handler) AdminRevokeKey(c echo.Context) error {
	keyID, err := strconv.Atoi(c.Param("keyID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid key ID")
	}

	if err := h.service.RevokeKey(c.Request().Context(), keyID); err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/admin/api")
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/api")
}

// --- Dashboard data struct ---

// AdminDashboardData holds all data for the admin API monitoring dashboard.
type AdminDashboardData struct {
	Stats          *APIStats
	RequestSeries  []TimeSeriesPoint
	SecuritySeries []TimeSeriesPoint
	TopIPs         []TopEntry
	TopPaths       []TopEntry
	TopKeys        []TopEntry
	SecurityEvents []SecurityEvent
	IPBlocks       []IPBlock
	APIKeys        []APIKey
	TotalKeys      int
	CSRFToken      string
}
