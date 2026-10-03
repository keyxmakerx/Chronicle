package syncapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// apiKeyContextKey is the Echo context key for the authenticated API key.
const apiKeyContextKey = "api_key"

// synthKeySessionID is the sentinel KeyID used for synthetic APIKeys that
// represent a session-authed caller on /api/v1/*. A real api_keys row has
// an AUTO_INCREMENT id >= 1, so ID == 0 unambiguously flags a synthetic
// identity and lets downstream middleware (RateLimit, logging) skip work
// that only makes sense for real keys.
const synthKeySessionID = 0

// SyncAPIAddonSlug is the campaign_addons slug behind the campaign's
// "Sync API" toggle (seeded by db/migrations/000001_baseline.up.sql).
// Exported because the reconciler, the service and the route wiring all
// name the same addon and a second literal is how the two drift apart.
const SyncAPIAddonSlug = "sync-api"

// GetAPIKey retrieves the authenticated API key from the request context.
// Under the multi-auth path (RequireAuthOrAPIKey), a session-authed caller
// gets a synthetic APIKey with ID == 0; callers that need to distinguish
// real keys from session callers should check for that sentinel.
func GetAPIKey(c echo.Context) *APIKey {
	key, _ := c.Get(apiKeyContextKey).(*APIKey)
	return key
}

// permissionsForCampaignRole maps a user's campaign membership role onto the
// API-key permission model so session-authed callers on /api/v1/* get the
// same downstream authorization as an equivalent Bearer request. The mapping
// is the natural least-surprising one: Owner grants everything; Scribe can
// read + write content but not configure sync endpoints; Player is read-only.
// Called by RequireAuthOrAPIKey when synthesising an APIKey from a session.
func permissionsForCampaignRole(role campaigns.Role) []APIKeyPermission {
	switch role {
	case campaigns.RoleOwner:
		return []APIKeyPermission{PermRead, PermWrite, PermSync}
	case campaigns.RoleScribe:
		return []APIKeyPermission{PermRead, PermWrite}
	case campaigns.RolePlayer:
		return []APIKeyPermission{PermRead}
	default:
		return nil
	}
}

// RequireAuthOrAPIKey authenticates /api/v1/* requests by EITHER a session
// cookie OR an Authorization: Bearer API key. A valid chronicle_session
// cookie synthesises an APIKey from the caller's campaign membership
// (CampaignID, UserID, Permissions) under apiKeyContextKey so downstream
// middleware works uniformly; otherwise it delegates to RequireAPIKey.
//
// Security notes:
//   - The session cookie is SameSite=Lax (auth.setSessionCookie), so a
//     cross-origin POST won't include it — that's the CSRF defense here,
//     no separate token needed.
//   - Synthesised keys carry ID == synthKeySessionID so the rate limiter
//     and LogRequest can tell session callers from real API keys.
//   - IP blocklist/allowlist and device-fingerprint checks run only on the
//     Bearer path; session auth trusts the upstream auth service.
//
// Must be wired in place of RequireAPIKey on the /api/v1 group.
func RequireAuthOrAPIKey(authSvc auth.AuthService, campaignSvc campaigns.CampaignService, syncSvc SyncAPIService) echo.MiddlewareFunc {
	apiKey := RequireAPIKey(syncSvc)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		apiKeyChain := apiKey(next)
		return func(c echo.Context) error {
			if key, ok := tryAuthFromSession(c, authSvc, campaignSvc); ok {
				c.Set(apiKeyContextKey, key)
				return next(c)
			}
			return apiKeyChain(c)
		}
	}
}

// tryAuthFromSession attempts to resolve the caller from a session cookie.
// Returns (key, true) when a valid session maps to a campaign membership
// the URL allows, where `key` is a synthetic APIKey with permissions
// derived from the membership role. Returns (nil, false) on any negative
// outcome — "no cookie", "invalid cookie", "not a member" — so the caller
// can cleanly fall through to the Bearer path. A user who is signed in but
// isn't a member of the campaign in the URL must return false here rather
// than 403, otherwise a real API key for the same campaign would be
// blocked by the session-membership failure.
func tryAuthFromSession(c echo.Context, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) (*APIKey, bool) {
	token := auth.GetSessionTokenFromCookie(c)
	if token == "" {
		return nil, false
	}
	session, err := authSvc.ValidateSession(c.Request().Context(), token)
	if err != nil || session == nil {
		return nil, false
	}

	campaignID := c.Param("id")
	if campaignID == "" {
		// Routes that don't carry :id can still authenticate by session
		// (e.g. a future /api/v1/me endpoint); synthesise a minimal key.
		auth.SetSession(c, session)
		return &APIKey{
			ID:       synthKeySessionID,
			UserID:   session.UserID,
			IsActive: true,
		}, true
	}

	member, err := campaignSvc.GetMember(c.Request().Context(), campaignID, session.UserID)
	if err != nil || member == nil {
		// Session is valid but the user isn't a member of this campaign.
		// Fall through to the Bearer path; a site admin still lands on
		// 403 when the Bearer path also fails, which is the correct
		// outcome.
		return nil, false
	}

	auth.SetSession(c, session)
	return &APIKey{
		ID:          synthKeySessionID,
		CampaignID:  campaignID,
		UserID:      session.UserID,
		Permissions: permissionsForCampaignRole(member.Role),
		IsActive:    true,
	}, true
}

// moduleVersionHeader carries the Chronicle Sync module's own version on
// every REST request so the owner can see which build last connected.
const moduleVersionHeader = "X-Chronicle-Module-Version"

// moduleVersionPattern bounds what a client can make us store: the value is
// self-reported and later rendered, so anything beyond a plain version string
// (letters, digits, dot, plus, hyphen; at most 32) is dropped rather than
// escaped.
var moduleVersionPattern = regexp.MustCompile(`^[0-9A-Za-z.+\-]{1,32}$`)

// moduleVersionFromHeader returns the reported module version, or "" when the
// header is absent or malformed. "" means "leave the stored value alone",
// never "clear it".
func moduleVersionFromHeader(h http.Header) string {
	v := h.Get(moduleVersionHeader)
	if !moduleVersionPattern.MatchString(v) {
		return ""
	}
	return v
}

// recordKeyUsage stamps last-used time/IP and the reported module version on
// a stored key. The session door's synthetic key has no api_keys row, so it
// is never written.
func recordKeyUsage(ctx context.Context, service SyncAPIService, key *APIKey, ip, moduleVersion string) error {
	if key == nil || key.ID == synthKeySessionID {
		return nil
	}
	return service.UpdateKeyLastUsed(ctx, key.ID, ip, moduleVersion)
}

// RequireAPIKey returns middleware that authenticates requests via API key.
// Extracts the key from the Authorization header, validates it with bcrypt,
// checks the IP blocklist, verifies IP allowlist, and records the request.
func RequireAPIKey(service SyncAPIService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ip := c.RealIP()

			// Check IP blocklist first — reject before any key processing.
			blocked, err := service.IsIPBlocked(ctx, ip)
			if err != nil {
				slog.Warn("ip blocklist check failed", slog.Any("error", err))
			}
			if blocked {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventIPBlocked,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
				})
				return apperror.NewForbidden("ip address blocked")
			}

			// Extract API key from Authorization header.
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventAuthFailure,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": "missing authorization header"},
				})
				return apperror.NewUnauthorized("api key required")
			}

			rawKey := strings.TrimPrefix(authHeader, "Bearer ")
			if rawKey == authHeader {
				// No "Bearer " prefix found.
				return apperror.NewUnauthorized("invalid authorization format, use: Bearer <key>")
			}

			// Authenticate the key (prefix lookup + bcrypt verify).
			key, err := service.AuthenticateKey(ctx, rawKey)
			if err != nil {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventAuthFailure,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": err.Error()},
				})
				return apperror.NewUnauthorized("invalid api key")
			}

			// Verify IP allowlist if configured.
			if len(key.IPAllowlist) > 0 && !isIPAllowed(ip, key.IPAllowlist) {
				_ = service.LogSecurityEvent(ctx, &SecurityEvent{
					EventType: EventIPBlocked,
					APIKeyID:  &key.ID,
					IPAddress: ip,
					UserAgent: strPtr(c.Request().UserAgent()),
					Details:   map[string]any{"reason": "ip not in allowlist"},
				})
				return apperror.NewForbidden("ip address not allowed for this key")
			}

			// Device fingerprint enforcement: if the client sends X-Device-Fingerprint,
			// auto-bind on first use; reject mismatches on subsequent requests.
			// This ensures a key can only be used by a single registered device.
			// Binding uses a conditional UPDATE (WHERE fingerprint IS NULL) so
			// concurrent first requests race safely — only one wins the bind.
			deviceFP := c.Request().Header.Get("X-Device-Fingerprint")
			if deviceFP != "" {
				if key.DeviceFingerprint == nil {
					// First use — bind the device synchronously.
					if bindErr := service.BindDevice(ctx, key.ID, deviceFP); bindErr != nil {
						slog.Warn("device fingerprint binding failed",
							slog.Int("key_id", key.ID),
							slog.Any("error", bindErr),
						)
					}
				} else if *key.DeviceFingerprint != deviceFP {
					// Device mismatch — reject.
					_ = service.LogSecurityEvent(ctx, &SecurityEvent{
						EventType:  EventSuspicious,
						APIKeyID:   &key.ID,
						CampaignID: &key.CampaignID,
						IPAddress:  ip,
						UserAgent:  strPtr(c.Request().UserAgent()),
						Details:    map[string]any{"reason": "device fingerprint mismatch"},
					})
					return apperror.NewForbidden("device not authorized for this key")
				}
			}

			// Store the key in context for downstream handlers.
			c.Set(apiKeyContextKey, key)

			// Update last-used timestamp (fire-and-forget).
			// Use background context since the request context may be cancelled
			// before the goroutine completes.
			moduleVersion := moduleVersionFromHeader(c.Request().Header)
			go func() {
				_ = recordKeyUsage(context.Background(), service, key, ip, moduleVersion)
			}()

			// Execute the handler and log the request.
			start := time.Now()
			err = next(c)
			duration := time.Since(start)

			statusCode := c.Response().Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					statusCode = he.Code
				} else {
					statusCode = http.StatusInternalServerError
				}
			}

			// Log the request (fire-and-forget).
			var errMsg *string
			if err != nil {
				msg := err.Error()
				errMsg = &msg
			}
			go func() {
				_ = service.LogRequest(context.Background(), &APIRequestLog{
					APIKeyID:     key.ID,
					CampaignID:   key.CampaignID,
					UserID:       key.UserID,
					Method:       c.Request().Method,
					Path:         c.Request().URL.Path,
					StatusCode:   statusCode,
					IPAddress:    ip,
					UserAgent:    strPtr(c.Request().UserAgent()),
					RequestSize:  int(c.Request().ContentLength),
					ResponseSize: int(c.Response().Size),
					DurationMs:   int(duration.Milliseconds()),
					ErrorMessage: errMsg,
				})
			}()

			return err
		}
	}
}

// RequirePermission returns middleware that checks the API key has a specific permission.
func RequirePermission(perm APIKeyPermission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return apperror.NewUnauthorized("api key required")
			}
			if !key.HasPermission(perm) {
				return apperror.NewForbidden("insufficient permissions: requires " + string(perm))
			}
			return next(c)
		}
	}
}

// RequireCampaignMatch returns middleware that verifies the API key's campaign
// matches the :id parameter in the URL, and blocks state-changing methods
// (POST/PUT/PATCH/DELETE) against an archived campaign. Bearer-key requests
// never pass through campaigns.RequireCampaignAccess (they carry no session,
// so that web middleware never runs), so this is the sync API's one
// group-level place to enforce the same "archived is read-only" rule the web
// app enforces by default — every route mounted on the /api/v1/campaigns/:id
// group (including the parallel v1Multipart media-upload group, which also
// calls this) inherits it. Prevents using a key scoped to one campaign to
// access another.
func RequireCampaignMatch(campaignSvc campaigns.CampaignService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return apperror.NewUnauthorized("api key required")
			}
			campaignID := c.Param("id")
			if campaignID != key.CampaignID {
				return apperror.NewForbidden("api key not authorized for this campaign")
			}

			switch c.Request().Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				campaign, err := campaignSvc.GetByID(c.Request().Context(), campaignID)
				if err != nil {
					return err
				}
				if campaign.IsArchived() {
					return apperror.NewForbidden("campaign is archived and read-only")
				}
			}

			return next(c)
		}
	}
}

// RequireJSONContentType returns middleware that rejects state-changing
// (POST / PUT / PATCH) requests whose Content-Type is not application/json.
// Safe methods (GET / HEAD / DELETE / OPTIONS) pass through unchanged
// because they carry no request body.
//
// The check accepts any media type whose top-level type+subtype is
// application/json, including parameters like
// "application/json; charset=utf-8". Anything else returns 415
// Unsupported Media Type with a structured AppError body so a client
// (or proxy) substituting a non-JSON body into a JSON-expecting
// handler fails loudly rather than triggering Echo's silent
// json.Unmarshal-of-zero behavior.
//
// Mounts on the /api/v1/* JSON group only. The single multipart
// endpoint (POST /api/v1/campaigns/:id/media) is registered on a
// parallel sub-group that omits this middleware — see RegisterAPIRoutes.
func RequireJSONContentType() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			switch c.Request().Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
			default:
				return next(c)
			}
			ct := c.Request().Header.Get("Content-Type")
			// Strip any media-type parameters (";charset=utf-8" etc).
			if i := strings.IndexByte(ct, ';'); i >= 0 {
				ct = ct[:i]
			}
			ct = strings.TrimSpace(strings.ToLower(ct))
			if ct != "application/json" {
				return &apperror.AppError{
					Code:    http.StatusUnsupportedMediaType,
					Type:    "unsupported_media_type",
					Message: "Content-Type must be application/json",
				}
			}
			return next(c)
		}
	}
}

// --- Rate Limiting ---

// rateLimiter tracks per-key request counts using a sliding window.
type rateLimiter struct {
	mu      sync.Mutex
	windows map[int]*rateLimitWindow // Keyed by API key ID.
}

// rateLimitWindow tracks requests in the current minute.
type rateLimitWindow struct {
	count   int
	resetAt time.Time
}

// globalRateLimiter is the singleton rate limiter instance.
var globalRateLimiter = &rateLimiter{
	windows: make(map[int]*rateLimitWindow),
}

// RateLimit returns middleware that enforces per-key request rate limits.
// Uses a simple fixed-window counter per minute.
//
// Synthetic session keys (ID == synthKeySessionID) skip this limiter —
// they represent an authenticated browser user, not an external client,
// and would otherwise all share the ID == 0 bucket. Browser-paced users
// are naturally rate-limited by human speed; abusive session behavior is
// the auth service's responsibility, not the API-key limiter's.
func RateLimit(service SyncAPIService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return next(c)
			}
			if key.ID == synthKeySessionID {
				return next(c)
			}

			globalRateLimiter.mu.Lock()
			window, exists := globalRateLimiter.windows[key.ID]
			now := time.Now()

			if !exists || now.After(window.resetAt) {
				// New window.
				window = &rateLimitWindow{
					count:   0,
					resetAt: now.Add(time.Minute),
				}
				globalRateLimiter.windows[key.ID] = window
			}

			window.count++
			remaining := key.RateLimit - window.count
			globalRateLimiter.mu.Unlock()

			// Set rate limit headers.
			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(key.RateLimit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(remaining, 0)))

			if remaining < 0 {
				_ = service.LogSecurityEvent(c.Request().Context(), &SecurityEvent{
					EventType:  EventRateLimit,
					APIKeyID:   &key.ID,
					CampaignID: &key.CampaignID,
					IPAddress:  c.RealIP(),
					UserAgent:  strPtr(c.Request().UserAgent()),
				})
				c.Response().Header().Set("Retry-After", "60")
				return &apperror.AppError{Code: http.StatusTooManyRequests, Type: "rate_limit_exceeded", Message: "rate limit exceeded"}
			}

			return next(c)
		}
	}
}

// --- Helpers ---

// strPtr returns a pointer to a string (nil if empty).
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isIPAllowed checks if an IP is in the allowlist.
// Supports exact match and proper CIDR notation (e.g., "192.168.1.0/24").
func isIPAllowed(ip string, allowlist []string) bool {
	parsedIP := net.ParseIP(ip)
	for _, allowed := range allowlist {
		// Try exact match first.
		if allowed == ip {
			return true
		}
		// Try proper CIDR matching using the standard library.
		if strings.Contains(allowed, "/") {
			_, network, err := net.ParseCIDR(allowed)
			if err == nil && parsedIP != nil && network.Contains(parsedIP) {
				return true
			}
		}
	}
	return false
}

// RequireAddonAPI returns middleware that gates API endpoints behind addon
// enabled checks. Returns 404 JSON response when the addon is disabled,
// matching the behavior of the web RequireAddon middleware but for API context.
func RequireAddonAPI(addonChecker AddonChecker, slug string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			campaignID := c.Param("id")
			if campaignID == "" {
				return apperror.NewBadRequest("campaign ID required")
			}

			enabled, err := addonChecker.IsEnabledForCampaign(c.Request().Context(), campaignID, slug)
			if err != nil {
				// Fail closed — block access when addon status cannot be verified.
				slog.Error("addon check failed",
					slog.String("campaign_id", campaignID),
					slog.String("slug", slug),
					slog.Any("error", err),
				)
				return &apperror.AppError{Code: http.StatusServiceUnavailable, Type: "service_unavailable", Message: "temporarily unable to verify addon status"}
			}
			if !enabled {
				return apperror.NewNotFound(slug + " addon is not enabled for this campaign")
			}
			return next(c)
		}
	}
}

// AddonChecker defines the interface for checking addon enabled state.
// Implemented by the addons plugin's service.
type AddonChecker interface {
	IsEnabledForCampaign(ctx context.Context, campaignID, slug string) (bool, error)
}

// RequireSyncAPIAddon enforces the campaign's "Sync API" toggle against
// EXTERNAL Bearer clients on /api/v1/*.
//
// Passes synthetic session identities (ID == synthKeySessionID) through
// untouched: the toggle is an INTEGRATION control, so disabling it revokes
// outside access, not Chronicle's own UI. Unlike RequireAddonAPI, this
// answers 403 ("sync_api_disabled"), not 404 — a 404 would send the Foundry
// module down its version-compatibility path and hide the real reason.
//
// Mounted on /api/v1 (not the campaign sub-group) so a later route can't
// land outside the gate, and reads the campaign from key.CampaignID rather
// than c.Param("id") so a route without an :id segment is still covered.
// Fails closed on any uncertainty: no key, no campaign, unreadable state.
func RequireSyncAPIAddon(addonChecker AddonChecker) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				// Auth middleware must have run before this one; reaching
				// here means the chain is mis-wired. Refuse rather than
				// pass an unidentified caller through.
				return apperror.NewUnauthorized("api key required")
			}

			// Trap 1: first-party browser widgets. A session-authed caller
			// carries the synthetic sentinel ID and is not an integration.
			if key.ID == synthKeySessionID {
				return next(c)
			}

			if key.CampaignID == "" {
				slog.Error("sync-api addon gate: bearer key has no campaign",
					slog.Int("key_id", key.ID),
				)
				return syncAPIDisabledError()
			}

			enabled, err := addonChecker.IsEnabledForCampaign(
				c.Request().Context(), key.CampaignID, SyncAPIAddonSlug)
			if err != nil {
				slog.Error("sync-api addon check failed",
					slog.String("campaign_id", key.CampaignID),
					slog.Int("key_id", key.ID),
					slog.Any("error", err),
				)
				return &apperror.AppError{
					Code:    http.StatusServiceUnavailable,
					Type:    "service_unavailable",
					Message: "temporarily unable to verify addon status",
				}
			}
			if !enabled {
				return syncAPIDisabledError()
			}
			return next(c)
		}
	}
}

// RequireKeyOwnerStillOwner enforces that a stored Bearer key never outlives
// its creator's Owner access: once the creator (key.UserID) is no longer an
// Owner of the key's campaign — demoted or removed — every request on this
// key is refused with a clear, structured error instead of continuing at
// Owner-level access indefinitely.
//
// Skips synthetic session identities (ID == synthKeySessionID): those
// already resolve against the caller's LIVE membership role on every
// request via RequireAuthOrAPIKey, so there is nothing to decouple.
//
// Mounted right after RequireAuthOrAPIKey, ahead of the sync-api addon gate
// and the rate limiter — the same placement reasoning as RequireSyncAPIAddon:
// a dead key should not spend rate-limit budget being refused, and "this
// key's creator lost access" is the more fundamental reason to report first.
func RequireKeyOwnerStillOwner(campaignSvc campaigns.CampaignService, syncSvc SyncAPIService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := GetAPIKey(c)
			if key == nil {
				return apperror.NewUnauthorized("api key required")
			}
			if key.ID == synthKeySessionID {
				return next(c)
			}

			member, err := campaignSvc.GetMember(c.Request().Context(), key.CampaignID, key.UserID)
			if err == nil && member.Role >= campaigns.RoleOwner {
				return next(c)
			}

			reason := "owner_demoted"
			if err != nil {
				reason = "owner_removed"
			}
			if shouldEmitDegradeSignal(key.ID) {
				keyID, campaignID := key.ID, key.CampaignID
				_ = syncSvc.LogSecurityEvent(c.Request().Context(), &SecurityEvent{
					EventType:  EventKeyOwnerDegraded,
					APIKeyID:   &keyID,
					CampaignID: &campaignID,
					IPAddress:  c.RealIP(),
					UserAgent:  strPtr(c.Request().UserAgent()),
					Details:    map[string]any{"reason": reason, "key_user_id": key.UserID},
				})
			}
			return keyOwnerLostAccessError()
		}
	}
}

// keyOwnerLostAccessError is the single response shape for "this key's
// creator is no longer a campaign Owner". Shared by the REST middleware
// above and the WebSocket refusal (service.go AuthenticateKeyForWS) so a
// client sees one condition, not two — same pattern as syncAPIDisabledError.
func keyOwnerLostAccessError() *apperror.AppError {
	return &apperror.AppError{
		Code: http.StatusForbidden,
		Type: "key_owner_lost_access",
		Message: "this API key's creator is no longer a campaign owner; " +
			"a current campaign owner must issue a new key",
	}
}

// syncAPIDisabledError is the single response shape for "this campaign has
// the Sync API switched off". Shared by the REST gate and named by the
// WebSocket refusal so a client sees one condition, not two.
func syncAPIDisabledError() *apperror.AppError {
	return &apperror.AppError{
		Code: http.StatusForbidden,
		Type: "sync_api_disabled",
		Message: "the Sync API integration is switched off for this campaign; " +
			"a campaign owner can re-enable it on the campaign's Extensions page (sidebar → Extensions)",
	}
}
