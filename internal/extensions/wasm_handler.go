package extensions

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// WASMHandler serves HTTP endpoints for WASM plugin management and invocation.
type WASMHandler struct {
	manager    *PluginManager
	dispatcher *HookDispatcher
	extSvc     ExtensionService
}

// NewWASMHandler creates a new WASM handler.
func NewWASMHandler(manager *PluginManager, dispatcher *HookDispatcher, extSvc ExtensionService) *WASMHandler {
	return &WASMHandler{
		manager:    manager,
		dispatcher: dispatcher,
		extSvc:     extSvc,
	}
}

// ListWASMPlugins returns info about all loaded WASM plugins.
// GET /admin/extensions/wasm/plugins
func (h *WASMHandler) ListWASMPlugins(c echo.Context) error {
	plugins := h.manager.ListPlugins()
	if plugins == nil {
		plugins = []WASMPluginInfo{}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"plugins": plugins,
		"count":   len(plugins),
	})
}

// GetWASMPlugin returns info about a specific WASM plugin.
// GET /admin/extensions/wasm/plugins/:extID/:slug
func (h *WASMHandler) GetWASMPlugin(c echo.Context) error {
	extID := c.Param("extID")
	slug := c.Param("slug")

	info, found := h.manager.GetPlugin(extID, slug)
	if !found {
		return apperror.NewNotFound("WASM plugin not found")
	}

	return c.JSON(http.StatusOK, info)
}

// ReloadWASMPlugin reloads a WASM plugin from disk.
// POST /admin/extensions/wasm/plugins/:extID/:slug/reload
func (h *WASMHandler) ReloadWASMPlugin(c echo.Context) error {
	extID := c.Param("extID")
	slug := c.Param("slug")

	if err := h.manager.Reload(c.Request().Context(), extID, slug); err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "reloaded"})
}

// StopWASMPlugin unloads a WASM plugin.
// POST /admin/extensions/wasm/plugins/:extID/:slug/stop
func (h *WASMHandler) StopWASMPlugin(c echo.Context) error {
	extID := c.Param("extID")
	slug := c.Param("slug")

	if err := h.manager.Unload(c.Request().Context(), extID, slug); err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "stopped"})
}

// CallWASMPlugin invokes a function on a WASM plugin in the context of a campaign.
// POST /campaigns/:id/extensions/wasm/:extID/:slug/call
func (h *WASMHandler) CallWASMPlugin(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	extID := c.Param("extID")
	slug := c.Param("slug")

	var req WASMCallRequest
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if req.Function == "" {
		return apperror.NewBadRequest("function name is required")
	}

	// Verify the extension is enabled for this campaign.
	exts, err := h.extSvc.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	enabled := false
	for _, ext := range exts {
		if ext.ExtID == extID && ext.Enabled {
			enabled = true
			break
		}
	}
	if !enabled {
		return apperror.NewForbidden("extension not enabled for this campaign")
	}

	// Set campaign context for host functions.
	ctx := WithCampaignID(c.Request().Context(), cc.Campaign.ID)
	ctx = WithExtensionID(ctx, extID)

	resp, err := h.manager.Call(ctx, extID, slug, req.Function, req.Input)
	if err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, resp)
}

// ListCampaignWASMPlugins returns WASM plugins available for a campaign
// (from enabled extensions that have WASM contributions).
// GET /campaigns/:id/extensions/wasm/plugins
func (h *WASMHandler) ListCampaignWASMPlugins(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	exts, err := h.extSvc.ListForCampaign(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}

	var campaignPlugins []WASMPluginInfo
	for _, ext := range exts {
		if !ext.Enabled {
			continue
		}

		// Get WASM plugins from this extension's manifest.
		dbExt, err := h.extSvc.GetByExtID(c.Request().Context(), ext.ExtID)
		if err != nil {
			continue
		}

		var manifest ExtensionManifest
		if err := json.Unmarshal(dbExt.Manifest, &manifest); err != nil {
			continue
		}

		if manifest.Contributes == nil || len(manifest.Contributes.WASMPlugins) == 0 {
			continue
		}

		for _, wp := range manifest.Contributes.WASMPlugins {
			info, found := h.manager.GetPlugin(ext.ExtID, wp.Slug)
			if found {
				campaignPlugins = append(campaignPlugins, *info)
			} else {
				// Plugin exists in manifest but isn't loaded.
				campaignPlugins = append(campaignPlugins, WASMPluginInfo{
					ExtID:        ext.ExtID,
					Slug:         wp.Slug,
					Name:         wp.Name,
					Status:       WASMStatusStopped,
					Capabilities: wp.Capabilities,
					Hooks:        wp.Hooks,
				})
			}
		}
	}

	if campaignPlugins == nil {
		campaignPlugins = []WASMPluginInfo{}
	}

	return c.JSON(http.StatusOK, map[string]any{"plugins": campaignPlugins})
}
