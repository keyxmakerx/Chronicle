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
//   - Owner endpoints: pin / rotate / install-url / owner-tab fragment
//   - Public endpoints: per-campaign manifest + download
//   - Error mapping: foundry_vtt.Error → categorized JSON response
type Handler struct {
	svc             Service
	presenceLookup  PresenceLookup
}

// PresenceLookup is the narrow contract the presence-pill fragment
// handler needs from the WebSocket hub. Mirrors maps.FoundryPresenceLookup
// (same shape; intentionally duplicated to keep maps decoupled from
// foundry_vtt). Wired in via SetPresenceLookup from app/routes.go after
// the WS hub is constructed.
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
// true when we have no record of any Foundry-module connection for
// this campaign (the pill renders "never" in that case); otherwise
// LastSeen is set to the most recent activity timestamp.
//
// NW-2.3: relocated from the campaigns plugin where the endpoint
// originally lived. URL, response shape, and auth chain are preserved
// byte-for-byte to keep any operator bookmarks / external monitoring
// working — the plugin boundary moved, not the wire contract.
type FoundryPresenceResponse struct {
	Connected bool       `json:"connected"`
	NeverSeen bool       `json:"never_seen"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
}

// GetFoundryPresenceAPI returns the Foundry-module presence status
// for the campaign. Any campaign member can read — presence is
// operator diagnostic info, not sensitive state. Member access is
// enforced by the parent group's RequireCampaignAccess middleware
// (see app/routes.go's fvttCampaignAuthed group).
//
// GET /campaigns/:id/foundry-presence
//
// NW-2.3: moved from campaigns.Handler.GetFoundryPresenceAPI. The
// implementation reuses the existing foundry_vtt.PresenceLookup
// (already wired by app/routes.go via SetPresenceLookup), so the
// move shed the duplicate campaigns.FoundryPresenceLookup interface
// + handler field + setter without changing the wiring's behavior.
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

// OwnerTabFragmentHandler serves the per-campaign settings tab as
// an HTMX fragment. Called by the campaigns settings.templ's
// VTT Setup Guides → Foundry VTT disclosure section via hx-get.
//
// GET /campaigns/:id/foundry-vtt/settings-tab
//
// C-FMC-9 (Bug 3): error paths now render an inline error state
// INSIDE the swap target instead of returning apperror.NewMissingContext
// or 4xx/5xx. HTMX wouldn't swap a 4xx/5xx response by default, so
// owners hit a stuck-on-spinner state — visually "blank settings
// page". This handler now ALWAYS returns 200 with a rendered
// fragment; errors are surfaced via OwnerTabErrorState within the
// same container.
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

// CampaignSettingsFoundryGuideHandler serves the foundry-VTT-labeled
// disclosure block inside the per-campaign Settings -> Integrations
// tab's "VTT Setup Guides" section. Lazy-loaded by
// campaigns/settings.templ via HTMX. Owner-gated by the route's
// requireOwner middleware so a non-owner can't fetch the install-URL
// inner-fragment-loader markup.
//
// GET /campaigns/:id/foundry-vtt/setup-guide-fragment
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md (the
// fragment-lazy-load convention) + NW-2.2 Chunk D.
func (h *Handler) CampaignSettingsFoundryGuideHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	isOwner := cc.MemberRole >= campaigns.RoleOwner
	return middleware.Render(c, http.StatusOK, CampaignSettingsFoundryGuide(cc.Campaign.ID, isOwner))
}

// DashboardSyncBlockHandler serves the per-campaign dashboard "Foundry
// VTT Sync" block. Lazy-loaded by campaigns/dashboard_blocks.templ
// when the campaign's dashboard layout includes a sync_status block.
// Campaign-member access; the inner /sync-status hx-get is owner-
// gated by syncapi (preserves prior UX where non-owners see the
// outer chrome but the inner status fails).
//
// GET /campaigns/:id/foundry-vtt/dashboard-sync-block
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md +
// NW-2.2 Chunk D.
func (h *Handler) DashboardSyncBlockHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return middleware.Render(c, http.StatusOK, DashboardSyncBlock(cc.Campaign.ID))
}

// CampaignShowPresencePillHandler serves the "Connected to Foundry"
// status chip displayed next to the map title. Lazy-loaded by
// maps/maps.templ. Campaign-member access (non-sensitive status data).
//
// GET /campaigns/:id/foundry-vtt/presence-pill-fragment
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md +
// NW-2.2 Chunk D.
func (h *Handler) CampaignShowPresencePillHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	view := h.resolvePresence(cc.Campaign.ID)
	return middleware.Render(c, http.StatusOK, CampaignShowPresencePill(view))
}

// resolvePresence converts the WS hub's (lastSeen, connected) tuple into
// the templ's PresenceView. Mirrors maps.Handler.resolveFoundryPresence;
// the duplication is deliberate (each plugin owns its own renderable
// view of the same underlying data).
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

// CampaignShowBannerHandler serves the "newer Foundry module version
// available" banner displayed at the top of the campaign show page.
// Lazy-loaded by campaigns/show.templ. Owner-gated by the route's
// requireOwner middleware — matches the prior in-handler role gate.
//
// Returns either the rendered banner (if HasUpdate) or an empty body
// (if no update or banner not applicable). The templ itself renders
// nothing when HasUpdate is false, so the lazy-load slot replaces
// itself with empty content — graceful invisible state.
//
// GET /campaigns/:id/foundry-vtt/show-banner-fragment
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md +
// NW-2.2 Chunk D.
func (h *Handler) CampaignShowBannerHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	status, err := h.svc.GetBannerStatus(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		// Banner is supplementary; never fail the page-load chain over
		// a banner-read issue. Empty body lets the slot resolve to
		// invisible state.
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, CampaignShowFoundryBanner(cc.Campaign.ID, status))
}

// --- owner: pin / rotate / install-url ---

// SetPinAPI updates the calling campaign's FoundryModulePin.
// PUT /campaigns/:id/foundry-vtt/pin   Body: { "version": "v0.1.5" }
//
// Empty version clears the pin (latest-tracking mode).
func (h *Handler) SetPinAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req struct {
		Version string `json:"version"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if err := h.svc.SetPinnedVersion(c.Request().Context(), cc.Campaign.ID, req.Version); err != nil {
		return h.respondError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

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

// PublicManifestAPI is the endpoint Foundry hits on install + every
// update check. Token-gated; no campaign middleware (the per-
// campaign signed token is the only access control).
//
// GET /api/v1/campaigns/:cid/foundry-vtt/module.json?token=...
//
// Error responses are JSON-shaped per the operator's contract so
// Foundry's FM-CSU-DIAG can parse them and surface the message
// inline. See errors.go for the categorized formats.
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

// PublicDownloadAPI streams the per-campaign rewritten zip. The
// per-campaign token is the only access control; same shape as
// the manifest endpoint.
//
// GET /api/v1/campaigns/:cid/foundry-vtt/module.zip?token=...
//
// C-FMC-7 architectural correction: this endpoint now does per-
// campaign zip REWRITING at download time. Earlier (C-FMC-5b/5c)
// the endpoint streamed the install dir as-is, on the assumption
// that serve-time manifest rewriting at the manifest endpoint was
// sufficient. That was wrong — Foundry's update checks AFTER
// install read the extracted on-disk module.json (from the zip
// Chronicle served), and that file carried the upstream GitHub
// URLs. So update checks reverted to GitHub even though the
// install URL was Chronicle's.
//
// The fix: each download walks the install dir, copies every file
// byte-for-byte EXCEPT the module.json at the descriptor-declared
// path, which gets replaced with the same rewritten bytes the
// manifest endpoint serves. Foundry extracts a zip whose embedded
// module.json points at Chronicle URLs; update checks stay on
// Chronicle forever.
//
// Two different campaigns get different rewritten zips (different
// per-campaign signed manifest URLs in their module.json) even
// though both reference the same on-disk install dir. No caching —
// signatures must be freshly computed per request since token
// rotation invalidates earlier signatures.
//
// chronicle-package.json is excluded from the zip output (it's
// Chronicle-side metadata, not part of the module Foundry installs).
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

// respondError converts an error to the right HTTP shape. For
// foundry_vtt typed errors, returns the categorized JSON body
// Foundry's FM-CSU-DIAG knows how to parse. For other errors,
// re-returns them so Echo's apperror middleware handles them.
//
// Logs a structured breadcrumb for every typed error response so
// server-side operators can root-cause without depending on
// Foundry's client-side error rendering — Foundry core code emits
// generic "is forbidden" on 403 regardless of the structured body
// Chronicle sends, so the server log is the operator's only
// reliable diagnostic for cases like cordinator Issue #17.
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

// zipDirToWriterWithRewrite walks installDir and writes a zip
// stream to w. The file at moduleJSONPath (relative to installDir,
// e.g. "module.json" or "dist/module.json") is REPLACED with
// rewrittenManifest — the per-campaign Chronicle-URL-rewritten
// bytes the service produced. Every other file is copied byte-for-
// byte.
//
// chronicle-package.json is excluded — the descriptor is Chronicle-
// side metadata, not part of the module Foundry installs. Including
// it would leak the descriptor contract into the client's filesystem
// and confuse Foundry's manifest reader.
//
// C-FMC-7: this is the load-bearing piece of the URL-rewriting fix.
// Foundry's "Check for Update" reads the on-disk module.json that
// came from the installed zip; if THAT file's manifest field points
// at GitHub, update checks bypass Chronicle forever. Replacing
// module.json inside the streamed zip is the only place where we
// can guarantee Foundry's extracted file carries Chronicle URLs.
//
// Path comparison uses filepath.Clean on both sides — descriptors
// may declare the path as "module.json" or "./module.json" or
// "dist/module.json"; filepath.Walk produces forward-slash relative
// paths on Linux but the comparison normalizes either form.
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
