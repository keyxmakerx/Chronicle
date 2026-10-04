package foundry_vtt

import (
	"archive/zip"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler is the HTTP boundary. Echo handlers stay thin per the
// project conventions — bind, validate, call service, render.
//
// Three responsibilities:
//   - Owner endpoints: update screens / rotate / install-url / owner-tab fragment
//   - Public endpoints: per-campaign manifest + download
//   - Error mapping: foundry_vtt.Error → categorized JSON response
type Handler struct {
	svc            Service
	presenceLookup PresenceLookup
	activity       ActivityRecorder
	npcResolver    NPCResolver
	spotlight      SpotlightPublisher
	owner          OwnerUpdates
}

// PresenceLookup is the narrow contract the presence-pill fragment
// handler needs from the WebSocket hub. Mirrors maps.FoundryPresenceLookup
// (duplicated deliberately to keep maps decoupled from foundry_vtt).
type PresenceLookup interface {
	FoundryPresence(campaignID string) (lastSeen *time.Time, connected bool)
}

// SetPresenceLookup injects the WS hub's foundry-presence accessor for
// the per-campaign presence-pill fragment endpoint. Optional — if nil,
// the fragment endpoint returns the "never connected" state defensively.
func (h *Handler) SetPresenceLookup(p PresenceLookup) {
	h.presenceLookup = p
}

// NewHandler constructs the Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// FoundryPresenceResponse is the JSON shape returned by the
// /campaigns/:id/foundry-presence diagnostic endpoint. NeverSeen is
// true when there's no record of any Foundry-module connection for
// this campaign (the pill renders "never" in that case); otherwise
// LastSeen holds the most recent activity timestamp.
type FoundryPresenceResponse struct {
	Connected bool       `json:"connected"`
	NeverSeen bool       `json:"never_seen"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
}

// GetFoundryPresenceAPI returns the Foundry-module presence status for
// the campaign. Any campaign member can read — presence is operator
// diagnostic info, not sensitive state. Member access is enforced by
// the parent group's RequireCampaignAccess middleware (see
// app/routes.go's fvttCampaignAuthed group).
//
// GET /campaigns/:id/foundry-presence
func (h *Handler) GetFoundryPresenceAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.presenceLookup == nil {
		// Hub wasn't wired (test fixture or WS disabled). Treat as
		// never-seen so the pill / diagnostic renders consistently.
		return c.JSON(http.StatusOK, FoundryPresenceResponse{NeverSeen: true})
	}
	lastSeen, connected := h.presenceLookup.FoundryPresence(cc.Campaign.ID)
	return c.JSON(http.StatusOK, FoundryPresenceResponse{
		Connected: connected,
		NeverSeen: lastSeen == nil,
		LastSeen:  lastSeen,
	})
}

// --- owner: tab fragment ---

// OwnerTabFragmentHandler serves the per-campaign settings tab as an
// HTMX fragment, called by campaigns settings.templ's VTT Setup Guides
// disclosure section via hx-get.
//
// GET /campaigns/:id/foundry-vtt/settings-tab
//
// Always returns 200 with a rendered fragment, even on error: HTMX
// won't swap a 4xx/5xx response by default, so a raw error status here
// would leave owners stuck on a spinner. Errors render inline via
// OwnerTabErrorState within the same swap target instead.
func (h *Handler) OwnerTabFragmentHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		// Defensive — middleware should set this. If we get here,
		// either the route group's RequireCampaignAccess wasn't
		// applied, or campaign loading failed silently. Either way,
		// render an actionable message rather than a blank tab.
		return middleware.Render(c, http.StatusOK, OwnerTabErrorState(
			"Chronicle couldn't load the campaign for this Foundry VTT settings page.",
			"Reload the campaign settings page. If this persists, the campaign URL "+
				"may be malformed — contact a site admin.",
		))
	}
	data, err := h.svc.OwnerTabData(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		// Surface the typed error's actionable message inline. The
		// foundry_vtt typed errors already follow the four-clause
		// format; pass the full Message + an empty action (the
		// message already includes the next step).
		if fe := AsError(err); fe != nil {
			return middleware.Render(c, http.StatusOK, OwnerTabErrorState(fe.Message, ""))
		}
		// Untyped error: generic fallback that points at the admin
		// so the operator's logs are the recovery path.
		return middleware.Render(c, http.StatusOK, OwnerTabErrorState(
			"Chronicle hit an internal error preparing the Foundry VTT settings.",
			"Check the Chronicle server logs around this request timestamp; if the error persists, contact a site admin.",
		))
	}
	data.CSRFToken = middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK, OwnerTabFragment(data))
}

// DashboardSyncBlockHandler serves the per-campaign dashboard "Foundry
// VTT Sync" block. Lazy-loaded by campaigns/dashboard_blocks.templ when
// the dashboard layout includes a sync_status block. Campaign-member
// access; the inner /sync-status hx-get is owner-gated by syncapi, so
// non-owners see the outer chrome but the inner status fails.
//
// GET /campaigns/:id/foundry-vtt/dashboard-sync-block
func (h *Handler) DashboardSyncBlockHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return middleware.Render(c, http.StatusOK, DashboardSyncBlock(cc.Campaign.ID))
}

// CampaignShowPresencePillHandler serves the "Connected to Foundry"
// status chip next to the map title. Lazy-loaded by maps/maps.templ.
// Campaign-member access (non-sensitive status data).
//
// GET /campaigns/:id/foundry-vtt/presence-pill-fragment
func (h *Handler) CampaignShowPresencePillHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	view := h.resolvePresence(cc.Campaign.ID)
	return middleware.Render(c, http.StatusOK, CampaignShowPresencePill(view))
}

// resolvePresence converts the WS hub's (lastSeen, connected) tuple into
// the templ's PresenceView. Mirrors maps.Handler.resolveFoundryPresence
// deliberately — each plugin owns its own view of the same data.
func (h *Handler) resolvePresence(campaignID string) PresenceView {
	if h.presenceLookup == nil {
		// Defensive — SetPresenceLookup not called. Treat as never-
		// connected so the pill renders consistently.
		return PresenceView{NeverSeen: true}
	}
	last, connected := h.presenceLookup.FoundryPresence(campaignID)
	if last == nil {
		return PresenceView{NeverSeen: true}
	}
	return PresenceView{Connected: connected, LastSeen: last}
}

// --- owner: rotate / install-url ---

// RotateTokenAPI bumps the per-campaign signing version and
// returns the freshly-minted install URL.
// POST /campaigns/:id/foundry-vtt/token/rotate
func (h *Handler) RotateTokenAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	url, err := h.svc.RotateCampaignToken(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"install_url": url})
}

// InstallURLAPI returns the campaign's current install URL.
// GET /campaigns/:id/foundry-vtt/install-url
func (h *Handler) InstallURLAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	url, err := h.svc.BuildInstallURL(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"install_url": url})
}

// --- public: manifest + download (token-gated, no campaign middleware) ---

// PublicManifestAPI is the endpoint Foundry hits on install and every
// update check. Token-gated; no campaign middleware — the per-campaign
// signed token is the only access control. Error responses are
// JSON-shaped per errors.go's categorized formats so Foundry can parse
// and surface the message inline.
//
// GET /api/v1/campaigns/:cid/foundry-vtt/module.json?token=...
func (h *Handler) PublicManifestAPI(c echo.Context) error {
	cid := c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return h.respondError(c, ErrInvalidToken(nil))
	}
	if err := h.svc.VerifyManifestToken(c.Request().Context(), cid, token); err != nil {
		return h.respondError(c, err)
	}
	manifest, _, err := h.svc.BuildManifestForCampaign(c.Request().Context(), cid)
	if err != nil {
		return h.respondError(c, err)
	}
	return c.JSONBlob(http.StatusOK, manifest)
}

// PublicDownloadAPI streams the per-campaign rewritten zip: install dir
// copied byte-for-byte except module.json, which is replaced with the
// per-campaign Chronicle-URL-rewritten bytes so Foundry's later update
// checks stay on Chronicle instead of reverting to the upstream URLs
// baked into the on-disk file. Same token-only access control as the
// manifest endpoint. No caching — signatures must be fresh per request
// since token rotation invalidates earlier ones.
//
// GET /api/v1/campaigns/:cid/foundry-vtt/module.zip?token=...
func (h *Handler) PublicDownloadAPI(c echo.Context) error {
	cid := c.Param("cid")
	token := c.QueryParam("token")
	if token == "" {
		return h.respondError(c, ErrInvalidToken(nil))
	}
	if err := h.svc.VerifyManifestToken(c.Request().Context(), cid, token); err != nil {
		return h.respondError(c, err)
	}
	params, err := h.svc.BuildDownloadParams(c.Request().Context(), cid)
	if err != nil {
		return h.respondError(c, err)
	}
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="chronicle-foundry-module.zip"`)
	c.Response().WriteHeader(http.StatusOK)
	if err := zipDirToWriterWithRewrite(params.InstallDir, params.ModuleJSONPath, params.RewrittenManifest, c.Response()); err != nil {
		// Headers already sent; can't return a JSON error. Log
		// path via the framework's request logger.
		return err
	}
	return nil
}

// --- error mapping ---

// respondError converts an error to the right HTTP shape. Foundry_vtt
// typed errors get the categorized JSON body; other errors re-return
// so Echo's apperror middleware handles them. Logs a structured
// breadcrumb for every typed error, since Foundry core's client-side
// error rendering collapses a 403 to a generic "is forbidden" and the
// server log is the operator's only reliable diagnostic.
func (h *Handler) respondError(c echo.Context, err error) error {
	fe := AsError(err)
	if fe == nil {
		return err
	}
	slog.Warn("foundry_vtt error response",
		slog.String("path", c.Request().URL.Path),
		slog.String("category", string(fe.Category)),
		slog.String("code", fe.Code),
		slog.Int("http_status", fe.HTTPStatus()),
		slog.Any("cause", fe.Cause),
	)
	body := map[string]any{
		"error":    fe.Code,
		"message":  fe.Message,
		"category": string(fe.Category),
	}
	return c.JSON(fe.HTTPStatus(), body)
}

// --- zip helpers ---

// zipDirToWriterWithRewrite walks installDir and writes a zip stream to
// w. The file at moduleJSONPath is replaced with rewrittenManifest (the
// per-campaign Chronicle-URL-rewritten bytes); every other file is
// copied byte-for-byte. This is the only place the extracted module.json
// can be guaranteed to carry Chronicle URLs, since Foundry's "Check for
// Update" reads that on-disk file, not the manifest endpoint.
// chronicle-package.json is excluded — it's Chronicle-side metadata, not
// part of the module Foundry installs. Path comparison uses
// filepath.Clean on both sides so "module.json" and "./module.json"
// compare equal.
func zipDirToWriterWithRewrite(installDir, moduleJSONPath string, rewrittenManifest []byte, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()

	// Normalize the target path so descriptor variants (with or
	// without "./" prefix) compare equal to filepath.Rel's output.
	wantRel := filepath.Clean(moduleJSONPath)

	return filepath.Walk(installDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(installDir, path)
		if err != nil {
			return err
		}
		// Exclude Chronicle-side descriptor — see comment above.
		if rel == descriptorFilename {
			return nil
		}
		// Replace the manifest entry with the rewritten bytes.
		if filepath.Clean(rel) == wantRel {
			entry, err := zw.Create(rel)
			if err != nil {
				return err
			}
			_, err = entry.Write(rewrittenManifest)
			return err
		}
		// Copy every other file byte-for-byte.
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		entry, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, f)
		return err
	})
}
